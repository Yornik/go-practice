package circuitbreaker_test

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	circuitbreaker "github.com/yornik/go-practice/exercises/39-circuitbreaker"
)

// Step 5: concurrency, stale results, panics. Run these with -race.

func TestConcurrentCallsDoNotBlock(t *testing.T) {
	// One call's fn blocks. Other goroutines must not wait for it.
	clock := newClock()
	b := newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: 3,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Now:              clock.Now,
	})

	slow := startSlowCall(t, b)
	wantCounts(t, b, circuitbreaker.Counts{Requests: 1}) // counted when admitted
	wantState(t, b, circuitbreaker.Closed)
	executeN(t, b, 1, nil)
	executeN(t, b, 1, errUnavailable)
	wantCounts(t, b, circuitbreaker.Counts{Requests: 3, Successes: 1, Failures: 1, ConsecutiveFailures: 1})

	slow.finishWant(t, nil)
	wantCounts(t, b, circuitbreaker.Counts{Requests: 3, Successes: 2, Failures: 1, ConsecutiveSuccesses: 1})
}

func TestConcurrentProbeAdmission(t *testing.T) {
	// 50 goroutines call Execute at the same moment on a HalfOpen breaker
	// with HalfOpenMaxCalls 3. The probes block, so exactly 3 calls may be
	// admitted. Checking for a free slot and taking it must be one step. A
	// gap between the two shows up only now and then, so the test repeats
	// the scenario 20 times, and its clock is slow: callers queue up on the
	// breaker's lock, which makes such a gap much easier to hit. Use -race,
	// it makes the gap easier to hit still.
	clock := newClock()
	s := circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 100, // the probes do not close the breaker
		Now: func() time.Time {
			time.Sleep(20 * time.Microsecond)
			return clock.Now()
		},
	}
	b := newBreaker(t, s)
	toHalfOpen(t, b, clock, s)
	for round := 1; round <= 20; round++ {
		if got := probeRush(t, b, 50); got != 3 {
			t.Fatalf("round %d: %d calls were admitted at the same time, want HalfOpenMaxCalls = 3", round, got)
		}
		wantCounts(t, b, circuitbreaker.Counts{Requests: 3, Successes: 3, ConsecutiveSuccesses: 3})

		// A failed probe opens the breaker. After OpenTimeout it is
		// HalfOpen again, ready for the next round.
		executeN(t, b, 1, errUnavailable)
		clock.Advance(s.OpenTimeout)
		if got := stateOf(t, b); got != circuitbreaker.HalfOpen {
			t.Fatalf("round %d: State() = %v after a failed probe and OpenTimeout, want half-open", round, got)
		}
	}
}

// probeRush starts callers goroutines that call b.Execute at the same moment
// with a fn that blocks. Once every caller is either inside fn or rejected
// with ErrTooManyCalls, it lets the blocked calls return and waits for them.
// It returns how many calls were admitted.
func probeRush(t *testing.T, b *circuitbreaker.Breaker, callers int) int {
	t.Helper()
	gate := make(chan struct{})
	openGate := sync.OnceFunc(func() { close(gate) })
	defer openGate()

	var admitted, rejected atomic.Int32
	unexpected := make(chan error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					unexpected <- fmt.Errorf("Execute panicked: %v", v)
				}
			}()
			<-start
			err := b.Execute(func() error {
				admitted.Add(1)
				<-gate
				return nil
			})
			switch {
			case errors.Is(err, circuitbreaker.ErrTooManyCalls):
				rejected.Add(1)
			case err != nil:
				unexpected <- fmt.Errorf("Execute() = %v, want nil or ErrTooManyCalls", err)
			}
		}()
	}
	close(start)

	deadline := time.Now().Add(timeout)
	for int(admitted.Load()+rejected.Load()) < callers {
		if len(unexpected) > 0 {
			t.Fatal(<-unexpected)
		}
		if time.Now().After(deadline) {
			t.Fatalf("after %v: %d calls admitted and %d rejected with ErrTooManyCalls, want %d in all",
				timeout, admitted.Load(), rejected.Load(), callers)
		}
		time.Sleep(time.Millisecond)
	}
	openGate()
	within(t, "the admitted calls", wg.Wait)
	close(unexpected)
	for err := range unexpected {
		t.Fatal(err)
	}
	return int(admitted.Load())
}

func TestConcurrentStress(t *testing.T) {
	// 8 goroutines make 300 calls each, in bursts of 5 failures and 5
	// successes, while the clock moves on. Each fn reads State and Counts.
	// Afterwards the reported changes must form one path through the state
	// diagram, and the counts must add up. Run it with -race: it is meant
	// to find data races, such as reading a field without the lock.
	clock := newClock()
	rec := &recorder{}
	const failureThreshold, successThreshold = 3, 2
	b := newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: failureThreshold,
		OpenTimeout:      10 * time.Millisecond,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: successThreshold,
		Now:              clock.Now,
		OnStateChange:    rec.OnStateChange,
	})

	const goroutines, calls = 8, 300
	unexpected := make(chan error, goroutines)
	within(t, "8 goroutines calling Execute and State", func() {
		var wg sync.WaitGroup
		for g := range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if v := recover(); v != nil {
						unexpected <- fmt.Errorf("panic: %v", v)
					}
				}()
				for i := range calls {
					var want error
					if (g+i)%10 < 5 {
						want = errUnavailable
					}
					err := b.Execute(func() error {
						// While this call is in flight, read the breaker and
						// let other goroutines change it.
						b.State()
						b.Counts()
						runtime.Gosched()
						return want
					})
					if err != want && !errors.Is(err, circuitbreaker.ErrOpen) && !errors.Is(err, circuitbreaker.ErrTooManyCalls) {
						unexpected <- fmt.Errorf("Execute() = %v, want %v, ErrOpen or ErrTooManyCalls", err, want)
						return
					}
					if i%7 == 0 {
						clock.Advance(4 * time.Millisecond)
						b.State()
					}
				}
			}()
		}
		wg.Wait()
	})
	close(unexpected)
	for err := range unexpected {
		t.Fatal(err)
	}

	final := stateOf(t, b)
	changes := rec.Changes()
	if len(changes) == 0 {
		t.Fatalf("no state change was reported, but the bursts of 5 failures must open the breaker")
	}
	if rec.overlapped.Load() {
		t.Errorf("two calls of OnStateChange ran at the same time")
	}
	edges := map[change]bool{closedToOpen: true, openToHalfOpen: true, halfOpenToOpen: true, halfOpenToClosed: true}
	from := circuitbreaker.Closed
	for i, c := range changes {
		if c.From != from {
			t.Fatalf("change %d of %d is %v, but the change before it ended in %v: changes must be reported once each, in order",
				i+1, len(changes), c, from)
		}
		if !edges[c] {
			t.Fatalf("change %d of %d is %v, which is not an arrow of the state diagram", i+1, len(changes), c)
		}
		from = c.To
	}
	if final != from {
		t.Errorf("State() = %v, but the last reported change was to %v", final, from)
	}

	// All calls have returned, so every call admitted in the current state
	// has recorded its result.
	c := countsOf(t, b)
	if c.Requests != c.Successes+c.Failures {
		t.Errorf("Counts() = %+v after all calls returned, want Requests == Successes + Failures", c)
	}
	switch final {
	case circuitbreaker.Closed:
		if c.ConsecutiveFailures >= failureThreshold {
			t.Errorf("Counts() = %+v in state closed, want ConsecutiveFailures < FailureThreshold (%d)", c, failureThreshold)
		}
	case circuitbreaker.Open:
		if c != (circuitbreaker.Counts{}) {
			t.Errorf("Counts() = %+v in state open, want zero counts", c)
		}
	case circuitbreaker.HalfOpen:
		if c.Failures != 0 || c.ConsecutiveSuccesses >= successThreshold {
			t.Errorf("Counts() = %+v in state half-open, want no failures and ConsecutiveSuccesses < SuccessThreshold (%d)", c, successThreshold)
		}
	}
}

func TestStaleResults(t *testing.T) {
	// A call that was admitted in one state and returns after the state has
	// changed is ignored: no counts, no state change, no freed probe slot.
	// Execute still returns fn's error to its caller.
	settings := func(clock *fakeClock, failureThreshold, halfOpenMaxCalls, successThreshold int) circuitbreaker.Settings {
		return circuitbreaker.Settings{
			FailureThreshold: failureThreshold,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: halfOpenMaxCalls,
			SuccessThreshold: successThreshold,
			Now:              clock.Now,
		}
	}

	t.Run("a success from closed returns while open", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, settings(clock, 1, 1, 1))
		slow := startSlowCall(t, b)
		executeN(t, b, 1, errUnavailable) // opens
		slow.finishWant(t, nil)
		wantState(t, b, circuitbreaker.Open)
		wantCounts(t, b, circuitbreaker.Counts{})
		wantRejected(t, b, circuitbreaker.ErrOpen)
	})

	t.Run("a failure from closed returns while half-open", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 1, 1, 1)
		b := newBreaker(t, s)
		slow := startSlowCall(t, b)
		toHalfOpen(t, b, clock, s)
		slow.finishWant(t, errUnavailable) // must not re-open the breaker
		wantState(t, b, circuitbreaker.HalfOpen)
		wantCounts(t, b, circuitbreaker.Counts{})
		executeN(t, b, 1, nil) // the probe slot is free; the probe closes the breaker
		wantState(t, b, circuitbreaker.Closed)
	})

	t.Run("a success from closed returns while half-open", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 1, 1, 1)
		b := newBreaker(t, s)
		slow := startSlowCall(t, b)
		toHalfOpen(t, b, clock, s)
		slow.finishWant(t, nil) // not a probe: must not close the breaker
		wantState(t, b, circuitbreaker.HalfOpen)
		wantCounts(t, b, circuitbreaker.Counts{})
	})

	t.Run("a probe from an earlier half-open returns in a new half-open", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 1, 2, 2)
		b := newBreaker(t, s)
		toHalfOpen(t, b, clock, s)
		old := startSlowCall(t, b)        // probe of the first half-open
		executeN(t, b, 1, errUnavailable) // another probe fails: open again
		clock.Advance(s.OpenTimeout)
		wantState(t, b, circuitbreaker.HalfOpen) // the second half-open

		// old is still running, but it holds no slot of the new half-open.
		p1 := startSlowCall(t, b)
		p2 := startSlowCall(t, b)

		old.finishWant(t, errUnavailable) // must not re-open the breaker
		wantState(t, b, circuitbreaker.HalfOpen)
		wantCounts(t, b, circuitbreaker.Counts{Requests: 2})
		wantRejected(t, b, circuitbreaker.ErrTooManyCalls) // and must not free a slot

		p1.finishWant(t, nil)
		p2.finishWant(t, nil)
		wantState(t, b, circuitbreaker.Closed)
	})

	t.Run("a probe failure returns after another probe closed the breaker", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 1, 2, 1)
		b := newBreaker(t, s)
		toHalfOpen(t, b, clock, s)
		slow := startSlowCall(t, b) // a probe that will time out
		executeN(t, b, 1, nil)      // the other probe succeeds: closed
		wantState(t, b, circuitbreaker.Closed)

		slow.finishWant(t, errUnavailable) // must not re-open the breaker
		wantState(t, b, circuitbreaker.Closed)
		wantCounts(t, b, circuitbreaker.Counts{})
	})

	t.Run("a failure from an earlier closed returns in a new closed", func(t *testing.T) {
		clock := newClock()
		s := settings(clock, 2, 1, 1)
		b := newBreaker(t, s)
		slow := startSlowCall(t, b)
		toHalfOpen(t, b, clock, s)
		executeN(t, b, 1, nil) // the probe closes the breaker again
		wantState(t, b, circuitbreaker.Closed)

		slow.finishWant(t, errUnavailable)
		wantCounts(t, b, circuitbreaker.Counts{})
		executeN(t, b, 1, errUnavailable) // 1 failure in this closed state, not 2
		wantState(t, b, circuitbreaker.Closed)
	})
}

func TestPanicInFn(t *testing.T) {
	const crash = "nil pointer in the payments client"

	t.Run("Execute panics with fn's value and counts a failure", func(t *testing.T) {
		clock := newClock()
		rec := &recorder{}
		b := newBreaker(t, circuitbreaker.Settings{
			FailureThreshold: 2,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
			Now:              clock.Now,
			OnStateChange:    rec.OnStateChange,
		})
		if v := executePanics(t, b, crash); v != crash {
			t.Errorf("Execute panicked with %v, want fn's panic value %q", v, crash)
		}
		wantCounts(t, b, circuitbreaker.Counts{Requests: 1, Failures: 1, ConsecutiveFailures: 1})
		wantState(t, b, circuitbreaker.Closed)

		executePanics(t, b, crash)        // the 2nd failure in a row: open
		wantChanges(t, rec, closedToOpen) // reported before Execute panicked
		wantState(t, b, circuitbreaker.Open)
	})

	t.Run("a panic is a failure even if IsFailure says no to everything", func(t *testing.T) {
		clock := newClock()
		b := newBreaker(t, circuitbreaker.Settings{
			FailureThreshold: 1,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
			IsFailure:        func(error) bool { return false },
			Now:              clock.Now,
		})
		executePanics(t, b, crash)
		wantState(t, b, circuitbreaker.Open)
	})

	t.Run("a panicking probe re-opens the breaker and frees its slot", func(t *testing.T) {
		clock := newClock()
		s := circuitbreaker.Settings{
			FailureThreshold: 1,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: 1,
			SuccessThreshold: 1,
			Now:              clock.Now,
		}
		b := newBreaker(t, s)
		toHalfOpen(t, b, clock, s)
		executePanics(t, b, crash)
		wantState(t, b, circuitbreaker.Open)
		clock.Advance(s.OpenTimeout)
		executeN(t, b, 1, nil)
		wantState(t, b, circuitbreaker.Closed)
	})
}
