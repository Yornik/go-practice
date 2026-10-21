package singleflight_test

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoReturnsFnResult(t *testing.T) {
	t.Run("value", func(t *testing.T) {
		var g group // the zero value is ready to use
		q := newQuick("profile of user 42")
		want(t, `Do("user:42")`, do(t, &g, "user:42", q.fn), "profile of user 42", false)
		if got := q.calls.Load(); got != 1 {
			t.Errorf("fn was called %d times, want 1", got)
		}
	})

	t.Run("error and value are passed on as they are", func(t *testing.T) {
		// Some functions return a partial result together with an error.
		var g group
		o := do(t, &g, "user:42", func() (string, error) { return "partial", errBackend })
		if o.panicked != nil {
			t.Fatalf("Do panicked: %v", o.panicked)
		}
		if o.val != "partial" || !errors.Is(o.err, errBackend) || o.shared {
			t.Errorf(`Do("user:42") = %q, %v, %v; want "partial", %v, false`, o.val, o.err, o.shared, errBackend)
		}
	})
}

func TestDoDoesNotCache(t *testing.T) {
	// Every call that starts after the previous execution has finished runs
	// fn again, also after an error. Caching is the cache's job, not the
	// group's: the group only collapses calls that overlap in time.
	var g group
	var calls atomic.Int32
	fn := func() (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "", errBackend
		}
		return fmt.Sprintf("load %d", n), nil
	}

	o := do(t, &g, "user:42", fn)
	if o.panicked != nil || !errors.Is(o.err, errBackend) {
		t.Fatalf(`1st Do("user:42") = %q, %v (panic: %v), want the error %v`, o.val, o.err, o.panicked, errBackend)
	}
	want(t, `2nd Do("user:42")`, do(t, &g, "user:42", fn), "load 2", false)
	want(t, `3rd Do("user:42")`, do(t, &g, "user:42", fn), "load 3", false)
}

func TestDoCoalescesConcurrentCalls(t *testing.T) {
	// The cache stampede: a hot key expires and many requests miss at the
	// same moment. The first call runs fn; 10 more calls with the same key
	// arrive while it runs. They must wait for it and share its result.
	untilJoined(t, func(window time.Duration) int {
		var g group
		db := newLoader(t, "profile of user 42")
		first := startDo(&g, "user:42", db.fn)
		db.waitStartedBy(t, "Do", first)

		late := newQuick("from a second execution")
		var others [10]<-chan outcome
		for i := range others {
			others[i] = startDo(&g, "user:42", late.fn)
		}
		time.Sleep(window) // give the 10 callers time to reach Do

		if n := late.calls.Load(); n > 0 {
			t.Fatalf("%d of 10 callers ran their own fn while the first execution for the same key was "+
				"still running; they must wait for it and share its result", n)
		}

		db.release()
		f := await(t, "the first Do", first)
		joined := 0
		for i, ch := range others {
			o := await(t, fmt.Sprintf("Do number %d", i+2), ch)
			switch {
			case o.panicked != nil:
				t.Errorf("Do number %d panicked: %v", i+2, o.panicked)
			case o.val == db.val:
				joined++
				if o.err != nil || !o.shared {
					t.Errorf("Do number %d = %q, %v, %v; want %q, <nil>, true: it got the result of an "+
						"execution with more than one caller", i+2, o.val, o.err, o.shared, db.val)
				}
			case o.val == late.val:
				// It reached Do only after the first execution had finished,
				// so it rightly started a new one.
			default:
				t.Errorf("Do number %d = %q, %v, %v; want %q", i+2, o.val, o.err, o.shared, db.val)
			}
		}
		want(t, "the first Do", f, db.val, joined > 0)
		if got := db.calls.Load(); got != 1 {
			t.Errorf("the first caller's fn ran %d times, want 1", got)
		}
		return joined
	})
}

func TestDoKeysAreIndependent(t *testing.T) {
	// A slow load of one key must not delay any other key. If this test
	// times out, Do probably holds the group's lock while fn runs.
	var g group
	slow := newLoader(t, "profile of user 42")
	first := startDo(&g, "user:42", slow.fn)
	slow.waitStartedBy(t, "Do", first)

	other := newQuick("profile of user 7")
	want(t, `Do("user:7") while "user:42" is in flight`, do(t, &g, "user:7", other.fn), "profile of user 7", false)
	if got := other.calls.Load(); got != 1 {
		t.Errorf(`fn for "user:7" was called %d times, want 1`, got)
	}

	slow.release()
	want(t, `Do("user:42")`, await(t, `Do("user:42")`, first), "profile of user 42", false)
}
