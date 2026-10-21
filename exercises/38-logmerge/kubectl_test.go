package logmerge_test

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	logmerge "github.com/yornik/go-practice/exercises/38-logmerge"
)

// result is one pair that a ParseKubectl sequence yields.
type result struct {
	entry logmerge.Entry
	err   error
}

// collect2 ranges over a ParseKubectl sequence and returns every pair.
func collect2(t *testing.T, what string, seq func() iter.Seq2[logmerge.Entry, error]) []result {
	t.Helper()
	var got []result
	within(t, what, func() {
		for e, err := range seq() {
			got = append(got, result{e, err})
		}
	})
	return got
}

// ts parses an RFC 3339 timestamp for the expected entries.
func ts(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("test bug: %v", err)
	}
	return tm
}

// wantOK fails the test unless r is the entry want with a nil error.
func wantOK(t *testing.T, i int, r result, want logmerge.Entry) {
	t.Helper()
	if r.err != nil {
		t.Errorf("pair %d: error %v, want entry %s", i, r.err, show([]logmerge.Entry{want}))
		return
	}
	if !equalEntries([]logmerge.Entry{r.entry}, []logmerge.Entry{want}) {
		t.Errorf("pair %d: entry {%v %s %q}, want {%v %s %q}",
			i, r.entry.Time, r.entry.Source, r.entry.Line, want.Time, want.Source, want.Line)
	}
}

// wantParseError fails the test unless r holds a *ParseError for the given
// source and line number.
func wantParseError(t *testing.T, i int, r result, source string, line int) {
	t.Helper()
	var pe *logmerge.ParseError
	switch {
	case r.err == nil:
		t.Errorf("pair %d: entry %s with a nil error, want a *ParseError for line %d",
			i, show([]logmerge.Entry{r.entry}), line)
	case !errors.As(r.err, &pe):
		t.Errorf("pair %d: error %v (%T), want a *ParseError for line %d", i, r.err, r.err, line)
	case pe.Source != source || pe.Line != line || pe.Err == nil:
		t.Errorf("pair %d: ParseError{Source: %q, Line: %d, Err: %v}, want Source %q, Line %d and a non-nil Err",
			i, pe.Source, pe.Line, pe.Err, source, line)
	}
}

func TestParseKubectlLines(t *testing.T) {
	input := "2026-10-16T12:00:00.123456789Z GET /healthz 200\n" +
		"2026-10-16T12:00:00.5Z level=error msg=\"db timeout\" latency_ms=532\n" +
		// A stack trace line: the message starts with spaces, keep them.
		"2026-10-16T12:00:01Z     at com.example.Checkout.pay(Checkout.java:42)\n" +
		// An empty log line: the timestamp, the space, nothing.
		"2026-10-16T12:00:02.000000000Z \n" +
		// The same after an editor stripped the trailing space.
		"2026-10-16T12:00:02.5Z\n" +
		"2026-10-16T14:00:03+02:00 an offset instead of Z is fine too\n" +
		"2026-10-16T12:00:04.000001Z the last line has no newline"
	want := []logmerge.Entry{
		{Time: ts(t, "2026-10-16T12:00:00.123456789Z"), Source: "api-7d9f", Line: "GET /healthz 200"},
		{Time: ts(t, "2026-10-16T12:00:00.5Z"), Source: "api-7d9f", Line: `level=error msg="db timeout" latency_ms=532`},
		{Time: ts(t, "2026-10-16T12:00:01Z"), Source: "api-7d9f", Line: "    at com.example.Checkout.pay(Checkout.java:42)"},
		{Time: ts(t, "2026-10-16T12:00:02Z"), Source: "api-7d9f", Line: ""},
		{Time: ts(t, "2026-10-16T12:00:02.5Z"), Source: "api-7d9f", Line: ""},
		{Time: ts(t, "2026-10-16T12:00:03Z"), Source: "api-7d9f", Line: "an offset instead of Z is fine too"},
		{Time: ts(t, "2026-10-16T12:00:04.000001Z"), Source: "api-7d9f", Line: "the last line has no newline"},
	}
	got := collect2(t, "ParseKubectl", func() iter.Seq2[logmerge.Entry, error] {
		return logmerge.ParseKubectl("api-7d9f", strings.NewReader(input))
	})
	if len(got) != len(want) {
		t.Fatalf("ParseKubectl yielded %d pairs, want %d", len(got), len(want))
	}
	for i := range want {
		wantOK(t, i, got[i], want[i])
	}
}

func TestParseKubectlBadLines(t *testing.T) {
	// Line 2 is empty: skipped, but counted. Lines 3 and 4 have a bad
	// timestamp, line 6 has none at all.
	input := `2026-10-16T12:00:00Z first

not-a-timestamp hello
2026-10-16 12:00:01 a space instead of T
2026-10-16T12:00:02Z after the bad lines
panic: runtime error: index out of range
2026-10-16T12:00:03Z parsing goes on to EOF
`
	got := collect2(t, "ParseKubectl", func() iter.Seq2[logmerge.Entry, error] {
		return logmerge.ParseKubectl("db-0", strings.NewReader(input))
	})
	if len(got) != 6 {
		t.Fatalf("ParseKubectl yielded %d pairs %v, want 6: 3 entries and 3 errors (the empty line yields nothing)", len(got), got)
	}
	wantOK(t, 0, got[0], logmerge.Entry{Time: ts(t, "2026-10-16T12:00:00Z"), Source: "db-0", Line: "first"})
	wantParseError(t, 1, got[1], "db-0", 3)
	wantParseError(t, 2, got[2], "db-0", 4)
	wantOK(t, 3, got[3], logmerge.Entry{Time: ts(t, "2026-10-16T12:00:02Z"), Source: "db-0", Line: "after the bad lines"})
	wantParseError(t, 4, got[4], "db-0", 6)
	wantOK(t, 5, got[5], logmerge.Entry{Time: ts(t, "2026-10-16T12:00:03Z"), Source: "db-0", Line: "parsing goes on to EOF"})
}

func TestParseKubectlLongLine(t *testing.T) {
	// One JSON log line of 300 KiB, more than bufio.Scanner's default limit
	// of 64 KiB, between two short lines.
	long := `{"msg":"dump","data":"` + strings.Repeat("x", 300*1024) + `"}`
	input := "2026-10-16T12:00:00Z before\n" +
		"2026-10-16T12:00:01Z " + long + "\n" +
		"2026-10-16T12:00:02Z after\n"
	got := collect2(t, "ParseKubectl", func() iter.Seq2[logmerge.Entry, error] {
		return logmerge.ParseKubectl("api", strings.NewReader(input))
	})
	if len(got) != 3 {
		t.Fatalf("ParseKubectl yielded %d pairs, want 3 (errors: %v)", len(got), got)
	}
	wantOK(t, 0, got[0], logmerge.Entry{Time: ts(t, "2026-10-16T12:00:00Z"), Source: "api", Line: "before"})
	if got[1].err != nil || len(got[1].entry.Line) != len(long) || got[1].entry.Line != long {
		t.Errorf("pair 1: entry with a %d-byte line and error %v, want the %d-byte line and a nil error",
			len(got[1].entry.Line), got[1].err, len(long))
	}
	wantOK(t, 2, got[2], logmerge.Entry{Time: ts(t, "2026-10-16T12:00:02Z"), Source: "api", Line: "after"})
}

func TestParseKubectlReadError(t *testing.T) {
	// The reader delivers 2 lines and then fails, like a broken connection
	// in the middle of kubectl logs -f.
	errConn := errors.New("unexpected EOF from the API server")
	r := io.MultiReader(
		strings.NewReader("2026-10-16T12:00:00Z one\n2026-10-16T12:00:01Z two\n"),
		iotest.ErrReader(errConn),
	)
	got := collect2(t, "ParseKubectl", func() iter.Seq2[logmerge.Entry, error] {
		return logmerge.ParseKubectl("api", r)
	})
	if len(got) != 3 {
		t.Fatalf("ParseKubectl yielded %d pairs %v, want 3: 2 entries, then 1 error, then the end", len(got), got)
	}
	wantOK(t, 0, got[0], logmerge.Entry{Time: ts(t, "2026-10-16T12:00:00Z"), Source: "api", Line: "one"})
	wantOK(t, 1, got[1], logmerge.Entry{Time: ts(t, "2026-10-16T12:00:01Z"), Source: "api", Line: "two"})
	if !errors.Is(got[2].err, errConn) {
		t.Errorf("pair 2: error %v, want an error that wraps the read error (errors.Is)", got[2].err)
	}
}

// followReader is like the output of kubectl logs -f: it never ends. Every
// Read returns one line, at t0 + n seconds. As a safety net it fails after
// endlessLimit lines, so a solution that reads everything first stops there.
type followReader struct {
	mu      sync.Mutex
	pending []byte
	lines   int
	reads   atomic.Int32
}

func (r *followReader) Read(p []byte) (int, error) {
	r.reads.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 {
		if r.lines == endlessLimit {
			return 0, errors.New("followReader: read too far")
		}
		r.pending = fmt.Appendf(nil, "%s line %d\n", at(float64(r.lines)).Format(time.RFC3339Nano), r.lines)
		r.lines++
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func TestParseKubectlIsLazy(t *testing.T) {
	r := &followReader{}
	var got []string
	within(t, "ranging over ParseKubectl of an endless reader", func() {
		seq := logmerge.ParseKubectl("api", r)
		if n := r.reads.Load(); n != 0 {
			t.Errorf("calling ParseKubectl read r %d times; nothing may happen before you range over the result", n)
		}
		for e, err := range seq {
			if err != nil {
				got = append(got, "error: "+err.Error())
				break
			}
			got = append(got, e.Line)
			if len(got) == 3 {
				break
			}
		}
	})
	want := []string{"line 0", "line 1", "line 2"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("first 3 pairs = %q, want %q", got, want)
	}
	// The reader gives one line per Read, so 3 entries need about 3 reads.
	// Buffering is fine; reading everything first is not.
	if n := r.reads.Load(); n > 100 {
		t.Errorf("ParseKubectl called Read %d times to yield 3 entries; it must read only as far as it needs", n)
	}
}

func TestParseKubectlBreak(t *testing.T) {
	// Breaking out of the loop must stop ParseKubectl, on an entry and on an
	// error. Ignoring yield's result makes the range loop panic.
	input := "2026-10-16T12:00:00Z ok\nbad line\n2026-10-16T12:00:01Z ok again\n"
	t.Run("break on the first entry", func(t *testing.T) {
		var n int
		within(t, "ranging over ParseKubectl", func() {
			for range logmerge.ParseKubectl("api", strings.NewReader(input)) {
				n++
				break
			}
		})
		if n != 1 {
			t.Errorf("the loop ran %d times, want 1", n)
		}
	})
	t.Run("break on the first error", func(t *testing.T) {
		var seen []result
		within(t, "ranging over ParseKubectl", func() {
			for e, err := range logmerge.ParseKubectl("api", strings.NewReader(input)) {
				seen = append(seen, result{e, err})
				if err != nil {
					break
				}
			}
		})
		if len(seen) != 2 || seen[1].err == nil {
			t.Errorf("the loop saw %v, want an entry and then an error", seen)
		}
	})
}
