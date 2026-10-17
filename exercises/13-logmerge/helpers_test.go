package logmerge_test

import (
	"fmt"
	"iter"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	logmerge "github.com/yornik/go-practice/exercises/13-logmerge"
)

// timeout bounds every test step, so a wrong solution that loops forever
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// t0 is the base time of the test entries: 2026-10-16T12:00:00Z.
var t0 = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// at returns t0 plus sec seconds, rounded to the nanosecond.
func at(sec float64) time.Time {
	return t0.Add(time.Duration(math.Round(sec * float64(time.Second))))
}

// e builds an Entry at t0 + sec seconds.
func e(source string, sec float64, line string) logmerge.Entry {
	return logmerge.Entry{Time: at(sec), Source: source, Line: line}
}

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics (for example with "TODO: implement") or does not return within the
// timeout.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case v := <-done:
		if v != nil {
			t.Fatalf("%s panicked: %v", what, v)
		}
	case <-time.After(timeout):
		t.Fatalf("%s did not finish within %v", what, timeout)
	}
}

// collect ranges over seq and returns every entry it yields.
func collect(t *testing.T, what string, seq func() iter.Seq[logmerge.Entry]) []logmerge.Entry {
	t.Helper()
	var got []logmerge.Entry
	within(t, what, func() {
		for e := range seq() {
			got = append(got, e)
		}
	})
	return got
}

// show formats entries compactly for failure messages: the seconds after t0,
// the source and the line, for example [10s pod-a GET /].
func show(es []logmerge.Entry) string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = fmt.Sprintf("{%v %s %q}", e.Time.Sub(t0), e.Source, e.Line)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// equalEntries reports whether got and want hold the same entries in the same
// order. Times are compared with Time.Equal, as instants.
func equalEntries(got, want []logmerge.Entry) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !got[i].Time.Equal(want[i].Time) || got[i].Source != want[i].Source || got[i].Line != want[i].Line {
			return false
		}
	}
	return true
}

// checkEntries fails the test if got and want differ.
func checkEntries(t *testing.T, what string, got, want []logmerge.Entry) {
	t.Helper()
	if !equalEntries(got, want) {
		t.Errorf("%s yielded\n  %s\nwant\n  %s", what, show(got), show(want))
	}
}

// source is a test source that counts what happens to it. Its seq method
// returns an iter.Seq that yields the entries in order.
type source struct {
	name     string
	entries  []logmerge.Entry
	starts   atomic.Int32 // how often the sequence function was called
	pulled   atomic.Int32 // entries handed to yield so far (counted before the yield call)
	cleanups atomic.Int32 // how often the deferred cleanup of the sequence ran
}

// newSource makes a source whose entries are at t0 + secs, with the lines
// "<name>0", "<name>1", ...
func newSource(name string, secs ...float64) *source {
	s := &source{name: name}
	for i, sec := range secs {
		s.entries = append(s.entries, e(name, sec, fmt.Sprintf("%s%d", name, i)))
	}
	return s
}

func (s *source) seq() iter.Seq[logmerge.Entry] {
	return func(yield func(logmerge.Entry) bool) {
		s.starts.Add(1)
		// Like closing a file or an HTTP response body when the reader is done.
		defer s.cleanups.Add(1)
		for _, e := range s.entries {
			s.pulled.Add(1)
			if !yield(e) {
				return
			}
		}
	}
}

// endlessLimit is a safety net for endless sources: a wrong solution that
// tries to read everything first stops there instead of running forever.
const endlessLimit = 100_000

// endless is a source that never ends on its own, like kubectl logs -f. Entry
// i is at t0 + start + i*step seconds, with the line "<name><i>".
type endless struct {
	name        string
	start, step float64
	pulled      atomic.Int32
	cleanups    atomic.Int32
}

func (s *endless) seq() iter.Seq[logmerge.Entry] {
	return func(yield func(logmerge.Entry) bool) {
		defer s.cleanups.Add(1)
		for i := 0; i < endlessLimit; i++ {
			s.pulled.Add(1)
			if !yield(e(s.name, s.start+float64(i)*s.step, fmt.Sprintf("%s%d", s.name, i))) {
				return
			}
		}
	}
}
