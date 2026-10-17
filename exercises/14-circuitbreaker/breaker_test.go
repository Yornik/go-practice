package circuitbreaker_test

import (
	"strings"
	"testing"
	"time"

	circuitbreaker "github.com/yornik/go-practice/exercises/14-circuitbreaker"
)

// Step 1: New, Closed, Counts, IsFailure.

func TestNewRejectsInvalidSettings(t *testing.T) {
	valid := circuitbreaker.Settings{
		FailureThreshold: 3,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
	}
	tests := []struct {
		name  string
		edit  func(s *circuitbreaker.Settings)
		field string // the error message must contain it
	}{
		{"FailureThreshold 0", func(s *circuitbreaker.Settings) { s.FailureThreshold = 0 }, "FailureThreshold"},
		{"FailureThreshold -1", func(s *circuitbreaker.Settings) { s.FailureThreshold = -1 }, "FailureThreshold"},
		{"OpenTimeout 0", func(s *circuitbreaker.Settings) { s.OpenTimeout = 0 }, "OpenTimeout"},
		{"OpenTimeout -1s", func(s *circuitbreaker.Settings) { s.OpenTimeout = -time.Second }, "OpenTimeout"},
		{"HalfOpenMaxCalls 0", func(s *circuitbreaker.Settings) { s.HalfOpenMaxCalls = 0 }, "HalfOpenMaxCalls"},
		{"SuccessThreshold 0", func(s *circuitbreaker.Settings) { s.SuccessThreshold = 0 }, "SuccessThreshold"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := valid
			tt.edit(&s)
			var b *circuitbreaker.Breaker
			var err error
			within(t, "New", func() { b, err = circuitbreaker.New(s) })
			if err == nil {
				t.Fatalf("New with %s = %v, nil; want an error", tt.name, b)
			}
			if b != nil {
				t.Errorf("New with %s returned a breaker and an error; want a nil *Breaker", tt.name)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Errorf("New with %s: error %q does not name the field %s", tt.name, err, tt.field)
			}
		})
	}

	t.Run("the smallest valid values are accepted", func(t *testing.T) {
		newBreaker(t, circuitbreaker.Settings{
			FailureThreshold: 1,
			OpenTimeout:      time.Nanosecond,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
		})
	})
}

func TestNewBreaker(t *testing.T) {
	t.Run("starts closed with zero counts", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, circuitbreaker.Settings{
			FailureThreshold: 3,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
			Now:              clock.Now,
		})
		wantState(t, b, circuitbreaker.Closed)
		wantCounts(t, b, circuitbreaker.Counts{})
	})

	t.Run("IsFailure, Now and OnStateChange may be nil", func(t *testing.T) {
		b := newBreaker(t, circuitbreaker.Settings{
			FailureThreshold: 1,
			OpenTimeout:      time.Hour,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
		})
		executeN(t, b, 1, nil)
		executeN(t, b, 1, errUnavailable) // opens; there is no OnStateChange to call
		wantState(t, b, circuitbreaker.Open)
	})
}

func TestClosedOpensAfterConsecutiveFailures(t *testing.T) {
	settings := func(clock *fakeClock, threshold int) circuitbreaker.Settings {
		return circuitbreaker.Settings{
			FailureThreshold: threshold,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
			Now:              clock.Now,
		}
	}

	t.Run("opens on the 3rd consecutive failure, not before", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock, 3))
		executeN(t, b, 2, errUnavailable)
		wantState(t, b, circuitbreaker.Closed)
		executeN(t, b, 1, errUnavailable)
		wantState(t, b, circuitbreaker.Open)
	})

	t.Run("a success resets the run of failures", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock, 3))
		executeN(t, b, 2, errUnavailable)
		executeN(t, b, 1, nil)
		executeN(t, b, 2, errUnavailable)
		wantState(t, b, circuitbreaker.Closed) // 4 failures, but at most 2 in a row
		executeN(t, b, 1, errUnavailable)
		wantState(t, b, circuitbreaker.Open)
	})

	t.Run("FailureThreshold 1 opens on the first failure", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock, 1))
		executeN(t, b, 1, errUnavailable)
		wantState(t, b, circuitbreaker.Open)
	})
}

func TestClosedCounts(t *testing.T) {
	clock := newClock()
	b := newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: 3,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Now:              clock.Now,
	})
	steps := []struct {
		fnReturns error
		want      circuitbreaker.Counts
	}{
		{nil, circuitbreaker.Counts{Requests: 1, Successes: 1, ConsecutiveSuccesses: 1}},
		{nil, circuitbreaker.Counts{Requests: 2, Successes: 2, ConsecutiveSuccesses: 2}},
		{errUnavailable, circuitbreaker.Counts{Requests: 3, Successes: 2, Failures: 1, ConsecutiveFailures: 1}},
		{errUnavailable, circuitbreaker.Counts{Requests: 4, Successes: 2, Failures: 2, ConsecutiveFailures: 2}},
		{nil, circuitbreaker.Counts{Requests: 5, Successes: 3, Failures: 2, ConsecutiveSuccesses: 1}},
		{errUnavailable, circuitbreaker.Counts{Requests: 6, Successes: 3, Failures: 3, ConsecutiveFailures: 1}},
		{errUnavailable, circuitbreaker.Counts{Requests: 7, Successes: 3, Failures: 4, ConsecutiveFailures: 2}},
		// The 3rd failure in a row opens the breaker, and a state change
		// resets every count.
		{errUnavailable, circuitbreaker.Counts{}},
	}
	for i, step := range steps {
		executeN(t, b, 1, step.fnReturns)
		if got := countsOf(t, b); got != step.want {
			t.Fatalf("after call %d (fn returned %v): Counts() = %+v, want %+v", i+1, step.fnReturns, got, step.want)
		}
	}
	wantState(t, b, circuitbreaker.Open)
}

func TestClosedIsFailure(t *testing.T) {
	settings := func(clock *fakeClock, isFailure func(error) bool) circuitbreaker.Settings {
		return circuitbreaker.Settings{
			FailureThreshold: 2,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
			IsFailure:        isFailure,
			Now:              clock.Now,
		}
	}

	t.Run("an error that IsFailure rejects counts as a success", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock, ignoreNotFound))
		executeN(t, b, 5, errNotFound) // Execute still returns the 404 to the caller
		wantState(t, b, circuitbreaker.Closed)
		wantCounts(t, b, circuitbreaker.Counts{Requests: 5, Successes: 5, ConsecutiveSuccesses: 5})
	})

	t.Run("such an error resets the run of failures", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock, ignoreNotFound))
		executeN(t, b, 1, errUnavailable)
		executeN(t, b, 1, errNotFound)
		executeN(t, b, 1, errUnavailable)
		wantState(t, b, circuitbreaker.Closed)
		executeN(t, b, 1, errUnavailable)
		wantState(t, b, circuitbreaker.Open)
	})

	t.Run("nil is always a success; IsFailure is only asked about errors", func(t *testing.T) {
		clock := newClock()
		everything := func(error) bool { return true }
		b := newBreaker(t, settings(clock, everything))
		executeN(t, b, 3, nil)
		wantState(t, b, circuitbreaker.Closed)
		wantCounts(t, b, circuitbreaker.Counts{Requests: 3, Successes: 3, ConsecutiveSuccesses: 3})
	})

	t.Run("nil IsFailure: every error is a failure", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock, nil))
		executeN(t, b, 2, errNotFound)
		wantState(t, b, circuitbreaker.Open)
	})
}

// Step 2: Open and the open timeout.

func TestOpenRejectsCalls(t *testing.T) {
	clock := newClock()
	b := newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Now:              clock.Now,
	})
	executeN(t, b, 1, errUnavailable)
	wantState(t, b, circuitbreaker.Open)
	for range 3 {
		wantRejected(t, b, circuitbreaker.ErrOpen)
	}
	wantCounts(t, b, circuitbreaker.Counts{}) // rejected calls are not counted
	clock.Advance(5 * time.Second)
	wantRejected(t, b, circuitbreaker.ErrOpen)
	wantState(t, b, circuitbreaker.Open)
}

func TestOpenTimeout(t *testing.T) {
	const openTimeout = 10 * time.Second
	settings := func(clock *fakeClock) circuitbreaker.Settings {
		return circuitbreaker.Settings{
			FailureThreshold: 1,
			OpenTimeout:      openTimeout,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
			Now:              clock.Now,
		}
	}

	t.Run("half-open once OpenTimeout has passed, not before", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock))
		executeN(t, b, 1, errUnavailable)
		clock.Advance(openTimeout - time.Nanosecond)
		wantState(t, b, circuitbreaker.Open)
		wantRejected(t, b, circuitbreaker.ErrOpen)
		clock.Advance(time.Nanosecond)
		wantState(t, b, circuitbreaker.HalfOpen)
		wantCounts(t, b, circuitbreaker.Counts{})
	})

	t.Run("the timeout starts when the failing call returns", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock))
		// fn takes 3 seconds and then fails: the breaker opens at the end.
		err := execute(t, b, func() error {
			clock.Advance(3 * time.Second)
			return errUnavailable
		})
		if err != errUnavailable {
			t.Fatalf("Execute() = %v, want %v", err, errUnavailable)
		}
		clock.Advance(openTimeout - time.Nanosecond)
		wantState(t, b, circuitbreaker.Open)
		clock.Advance(time.Nanosecond)
		wantState(t, b, circuitbreaker.HalfOpen)
	})

	t.Run("rejected calls do not extend the open period", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock))
		executeN(t, b, 1, errUnavailable)
		clock.Advance(4 * time.Second)
		wantRejected(t, b, circuitbreaker.ErrOpen)
		clock.Advance(4 * time.Second)
		wantRejected(t, b, circuitbreaker.ErrOpen)
		clock.Advance(2 * time.Second)
		wantState(t, b, circuitbreaker.HalfOpen)
	})
}

func TestOpenExecuteAfterTimeout(t *testing.T) {
	// No State call in between: Execute itself sees that the timeout has
	// passed, changes to HalfOpen and admits the call as a probe.
	clock := newClock()
	b := newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 2,
		Now:              clock.Now,
	})
	executeN(t, b, 1, errUnavailable)
	clock.Advance(10 * time.Second)
	executeN(t, b, 1, nil)
	wantState(t, b, circuitbreaker.HalfOpen) // 1 of 2 successes
	wantCounts(t, b, circuitbreaker.Counts{Requests: 1, Successes: 1, ConsecutiveSuccesses: 1})
}

func TestOpenWithRealClock(t *testing.T) {
	// Now is nil, so the breaker uses time.Now.
	settings := func(openTimeout time.Duration) circuitbreaker.Settings {
		return circuitbreaker.Settings{
			FailureThreshold: 1,
			OpenTimeout:      openTimeout,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
		}
	}

	t.Run("stays open during a long timeout", func(t *testing.T) {
		b := newBreaker(t, settings(time.Hour))
		executeN(t, b, 1, errUnavailable)
		wantState(t, b, circuitbreaker.Open)
		wantRejected(t, b, circuitbreaker.ErrOpen)
	})

	t.Run("half-open after a short timeout", func(t *testing.T) {
		b := newBreaker(t, settings(time.Millisecond))
		executeN(t, b, 1, errUnavailable)
		deadline := time.Now().Add(timeout)
		for stateOf(t, b) != circuitbreaker.HalfOpen {
			if time.Now().After(deadline) {
				t.Fatalf("State() is still %v %v after the breaker opened with OpenTimeout 1ms and Now nil, want half-open",
					stateOf(t, b), timeout)
			}
			time.Sleep(time.Millisecond)
		}
	})
}

// Step 3: HalfOpen probes.

func TestHalfOpenClosesAfterSuccesses(t *testing.T) {
	// SuccessThreshold 3 with HalfOpenMaxCalls 1: the probes run one after
	// another, and the 3rd success in a row closes the breaker.
	clock := newClock()
	s := circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 3,
		Now:              clock.Now,
	}
	b := newBreaker(t, s)
	toHalfOpen(t, b, clock, s)

	executeN(t, b, 2, nil)
	wantState(t, b, circuitbreaker.HalfOpen)
	wantCounts(t, b, circuitbreaker.Counts{Requests: 2, Successes: 2, ConsecutiveSuccesses: 2})
	executeN(t, b, 1, nil)
	wantState(t, b, circuitbreaker.Closed)
	wantCounts(t, b, circuitbreaker.Counts{})

	// Closed again: calls go through and are counted from zero.
	executeN(t, b, 1, nil)
	wantCounts(t, b, circuitbreaker.Counts{Requests: 1, Successes: 1, ConsecutiveSuccesses: 1})
}

func TestHalfOpenFailureReopens(t *testing.T) {
	settings := func(clock *fakeClock, successThreshold int) circuitbreaker.Settings {
		return circuitbreaker.Settings{
			FailureThreshold: 1,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: successThreshold,
			IsFailure:        ignoreNotFound,
			Now:              clock.Now,
		}
	}

	t.Run("the first probe fails", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 1)
		b := newBreaker(t, s)
		toHalfOpen(t, b, clock, s)
		executeN(t, b, 1, errUnavailable)
		wantState(t, b, circuitbreaker.Open)
		wantCounts(t, b, circuitbreaker.Counts{})
		wantRejected(t, b, circuitbreaker.ErrOpen)
	})

	t.Run("a probe fails after some successes", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 3)
		b := newBreaker(t, s)
		toHalfOpen(t, b, clock, s)
		executeN(t, b, 2, nil)
		executeN(t, b, 1, errUnavailable)
		wantState(t, b, circuitbreaker.Open)
	})

	t.Run("the open timeout starts again at the failed probe", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 1)
		b := newBreaker(t, s)
		toHalfOpen(t, b, clock, s)
		clock.Advance(4 * time.Second)
		executeN(t, b, 1, errUnavailable)
		clock.Advance(s.OpenTimeout - time.Nanosecond)
		wantState(t, b, circuitbreaker.Open)
		clock.Advance(time.Nanosecond)
		wantState(t, b, circuitbreaker.HalfOpen)
	})

	t.Run("IsFailure applies to probes too", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 1)
		b := newBreaker(t, s)
		toHalfOpen(t, b, clock, s)
		executeN(t, b, 1, errNotFound) // the dependency answers: a success
		wantState(t, b, circuitbreaker.Closed)
	})
}

func TestHalfOpenProbeLimit(t *testing.T) {
	// These probes block until the test finishes them, so they run at the
	// same time. Execute must not hold a lock while fn runs.
	clock := newClock()
	s := circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 3,
		Now:              clock.Now,
	}
	b := newBreaker(t, s)
	toHalfOpen(t, b, clock, s)

	p1 := startSlowCall(t, b)
	p2 := startSlowCall(t, b)
	wantRejected(t, b, circuitbreaker.ErrTooManyCalls) // 2 probes are running
	wantCounts(t, b, circuitbreaker.Counts{Requests: 2})

	p1.finishWant(t, nil)
	wantState(t, b, circuitbreaker.HalfOpen)
	wantCounts(t, b, circuitbreaker.Counts{Requests: 2, Successes: 1, ConsecutiveSuccesses: 1})

	p3 := startSlowCall(t, b) // p1's slot is free again
	wantRejected(t, b, circuitbreaker.ErrTooManyCalls)

	p2.finishWant(t, nil)
	p3.finishWant(t, nil)
	wantState(t, b, circuitbreaker.Closed) // 3 successes in a row
}

// The tests for step 4 (OnStateChange) are in callback_test.go, and the tests
// for step 5 (concurrency, stale results, panics) in concurrency_test.go.
