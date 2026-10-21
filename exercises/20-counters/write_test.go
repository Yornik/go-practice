package counters_test

import (
	"bytes"
	"testing"
	"time"

	counters "github.com/yornik/go-practice/exercises/20-counters"
)

// Step 4: a slow scraper does not block the counters. Run with -race.

// stallingWriter is a scraper that stops in the middle of its first Write
// until another goroutine has created a counter and added to 2 counters.
// If those calls do not finish within the timeout, it gives up, records
// that they were blocked, and returns, so that the test ends instead of
// hanging.
type stallingWriter struct {
	r       *counters.Registry
	text    bytes.Buffer
	calls   int
	blocked bool // the other goroutine's calls did not return in time
	panic   any  // what the other goroutine's calls panicked with
}

func (w *stallingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == 1 {
		done := async(func() {
			w.r.Counter("uploads_new").Add(1)
			w.r.Counter("uploads_us").Add(1)
		})
		select {
		case w.panic = <-done:
		case <-time.After(timeout):
			w.blocked = true
		}
	}
	return w.text.Write(p)
}

func TestWriteDoesNotBlock(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	add(t, counter(t, r, "uploads_eu"), 3)
	add(t, counter(t, r, "uploads_us"), 2)

	w := &stallingWriter{r: r}
	var err error
	select {
	case v := <-async(func() { err = r.Write(w) }):
		if v != nil {
			t.Fatalf("Write(w) panicked: %v", v)
		}
	case <-time.After(3 * timeout):
		t.Fatalf("Write(w) did not return within %v. Stacks of all goroutines:\n%s", 3*timeout, stacks())
	}

	if w.blocked {
		t.Fatalf("Add and Counter blocked while Write was writing: Counter(%q).Add(1) and Counter(%q).Add(1) "+
			"in another goroutine did not return within %v while w.Write was running. "+
			"Write must not hold a lock while it calls w.Write", "uploads_new", "uploads_us", timeout)
	}
	if w.panic != nil {
		t.Fatalf("Counter(name).Add(1) in another goroutine panicked while Write was writing: %v", w.panic)
	}
	if w.calls == 0 {
		t.Fatalf("Write(w) did not call w.Write; want the text %q", "uploads_eu 3\nuploads_us 2\n")
	}
	if err != nil {
		t.Errorf("Write(w) returned the error %v, want nil", err)
	}
	if got, want := w.text.String(), "uploads_eu 3\nuploads_us 2\n"; got != want {
		t.Errorf("Write(w) wrote %q, want %q: the values of 1 snapshot taken when Write starts, "+
			"without the counter and the Add that another goroutine made while Write was writing", got, want)
	}

	wantSamples(t, "Snapshot() after Write", snapshot(t, r),
		s("uploads_eu", 3), s("uploads_new", 1), s("uploads_us", 3))
	if got, err := write(t, r); got != "uploads_eu 3\nuploads_new 1\nuploads_us 3\n" || err != nil {
		t.Errorf("the next Write(w) wrote %q and returned %v, want %q and nil",
			got, err, "uploads_eu 3\nuploads_new 1\nuploads_us 3\n")
	}
}
