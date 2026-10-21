package nightlog_test

import (
	"fmt"
	"io"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	nightlog "github.com/yornik/go-practice/exercises/11-nightlog"
)

// timeout bounds every call into the package, so a loop that never ends
// fails the test with a message instead of hanging the test binary.
const timeout = 5 * time.Second

// text joins lines with "\n" and ends the result with "\n":
// text("a", "b") is "a\nb\n". text() is "".
func text(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// at returns 2026-10-16 at the given UTC time of day, for example
// at(3, 12, 9) is 2026-10-16T03:12:09Z.
func at(hour, minute, sec int) time.Time {
	return time.Date(2026, 10, 16, hour, minute, sec, 0, time.UTC)
}

// plus2 is the zone +02:00, for lines that carry an offset.
var plus2 = time.FixedZone("", 2*60*60)

// readers wrap the input so that it arrives in different ways. Summarize
// must give the same result for every one of them.
var readers = []struct {
	name string
	wrap func(io.Reader) io.Reader
}{
	{"plain", func(r io.Reader) io.Reader { return r }},
	{"OneByteReader", iotest.OneByteReader}, // 1 byte per Read
	{"HalfReader", iotest.HalfReader},       // half of the bytes asked for
	{"DataErrReader", iotest.DataErrReader}, // io.EOF together with the last data
}

// summarize runs nightlog.Summarize in a goroutine. It fails the test if
// Summarize panics (with the stack, so you can find the line) or does not
// return within the timeout.
func summarize(t *testing.T, r io.Reader, minGap time.Duration) (nightlog.Summary, error) {
	t.Helper()
	type result struct {
		s     nightlog.Summary
		err   error
		panic string
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- result{panic: fmt.Sprintf("%v\n%s", v, debug.Stack())}
			}
		}()
		s, err := nightlog.Summarize(r, minGap)
		done <- result{s: s, err: err}
	}()
	select {
	case res := <-done:
		if res.panic != "" {
			t.Fatalf("Summarize panicked: %s", res.panic)
		}
		return res.s, res.err
	case <-time.After(timeout):
		t.Fatalf("Summarize did not return within %v (does the loop stop at io.EOF?)", timeout)
		return nightlog.Summary{}, nil
	}
}

// write runs nightlog.Write in a goroutine, like summarize.
func write(t *testing.T, w io.Writer, s nightlog.Summary) error {
	t.Helper()
	type result struct {
		err   error
		panic string
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- result{panic: fmt.Sprintf("%v\n%s", v, debug.Stack())}
			}
		}()
		done <- result{err: nightlog.Write(w, s)}
	}()
	select {
	case res := <-done:
		if res.panic != "" {
			t.Fatalf("Write panicked: %s", res.panic)
		}
		return res.err
	case <-time.After(timeout):
		t.Fatalf("Write did not return within %v", timeout)
		return nil
	}
}

// show formats a time for failure messages and comparisons: RFC 3339 with
// fractional seconds and the offset, so a time converted to another zone
// does not look equal. The zero time is "-".
func show(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339Nano)
}

// describe formats every field of s on its own line, levels A to Z. Two
// summaries are equal for these tests when describe gives the same text: a
// nil and an empty Gaps are equal, a nil and an empty Levels are not.
func describe(s nightlog.Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  Lines:     %d\n", s.Lines)
	fmt.Fprintf(&b, "  Malformed: %d\n", s.Malformed)
	if s.Levels == nil {
		b.WriteString("  Levels:    nil\n")
	} else {
		var levels []string
		for name, n := range s.Levels {
			levels = append(levels, fmt.Sprintf("%q: %d", name, n))
		}
		slices.Sort(levels)
		fmt.Fprintf(&b, "  Levels:    {%s}\n", strings.Join(levels, ", "))
	}
	fmt.Fprintf(&b, "  First:     %s\n", show(s.First))
	fmt.Fprintf(&b, "  Last:      %s\n", show(s.Last))
	fmt.Fprintf(&b, "  Gaps:      %d\n", len(s.Gaps))
	for _, g := range s.Gaps {
		fmt.Fprintf(&b, "    %s to %s\n", show(g.From), show(g.To))
	}
	return b.String()
}

// checkSummary fails the test if got and want differ. call describes the
// Summarize call.
func checkSummary(t *testing.T, call string, got, want nightlog.Summary) {
	t.Helper()
	if g, w := describe(got), describe(want); g != w {
		t.Errorf("%s returned\n%s\nwant\n%s", call, g, w)
	}
}

// errAfter returns data and then err. With withLast, the last bytes of data
// come in the same Read call as err (n > 0 and err != nil), which the
// io.Reader contract allows: the caller must use those bytes before it
// looks at the error.
type errAfter struct {
	data     string
	err      error
	withLast bool
}

func (r *errAfter) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 && r.withLast {
		return n, r.err
	}
	return n, nil
}

// failWriter is a disk that fills up: it accepts room bytes, then fails
// every Write with err. failed counts the failed calls.
type failWriter struct {
	err    error
	room   int
	buf    strings.Builder
	failed int
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.failed > 0 || w.buf.Len()+len(p) > w.room {
		w.failed++
		return 0, w.err
	}
	return w.buf.Write(p)
}
