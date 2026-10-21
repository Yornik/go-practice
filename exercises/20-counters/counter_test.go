package counters_test

import (
	"fmt"
	"testing"
	"time"
)

// Step 1: Counter, Add and Value.

func TestCounterBasic(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	c := counter(t, r, "uploads_eu")
	wantValue(t, c, "uploads_eu", 0, "Counter(\"uploads_eu\")")

	steps := []struct {
		add  int64
		want int64
	}{
		{0, 0},
		{1, 1},
		{41, 42},
		{0, 42},
		{1 << 40, 1<<40 + 42},
	}
	for _, st := range steps {
		add(t, c, st.add)
		wantValue(t, c, "uploads_eu", st.want, fmt.Sprintf("Add(%d)", st.add))
	}
}

func TestCounterNamesAreIndependent(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	eu := counter(t, r, "uploads_eu")
	us := counter(t, r, "uploads_us")
	empty := counter(t, r, "")
	add(t, eu, 3)
	add(t, us, 5)
	add(t, empty, 7)
	wantValue(t, eu, "uploads_eu", 3, "Add(3) on uploads_eu, Add(5) on uploads_us, Add(7) on \"\"")
	wantValue(t, us, "uploads_us", 5, "Add(3) on uploads_eu, Add(5) on uploads_us, Add(7) on \"\"")
	wantValue(t, empty, "", 7, "Add(3) on uploads_eu, Add(5) on uploads_us, Add(7) on \"\"")
}

func TestCounterPanics(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	c := counter(t, r, "uploads_eu")
	add(t, c, 10)

	for _, n := range []int64{-1, -10, -1 << 62} {
		if v := panicValue(t, fmt.Sprintf("Add(%d)", n), func() { c.Add(n) }); v == nil {
			t.Errorf("Add(%d) did not panic, want a panic: a counter only goes up", n)
		}
		wantValue(t, c, "uploads_eu", 10, fmt.Sprintf("Add(10), Add(%d)", n))
	}

	// The counter keeps working after the panic. A lock that the panic left
	// locked would block here.
	add(t, c, 1)
	wantValue(t, c, "uploads_eu", 11, "Add(10), 3 negative Adds that panicked, Add(1)")
}

// panicValue calls f in a new goroutine and returns what f panicked with,
// or nil. It fails the test if f does not return within the timeout.
func panicValue(t *testing.T, what string, f func()) any {
	t.Helper()
	select {
	case v := <-async(f):
		return v
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v (is a lock held, or never released?)", what, timeout)
		return nil
	}
}
