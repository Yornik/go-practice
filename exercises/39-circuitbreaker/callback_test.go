package circuitbreaker_test

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	circuitbreaker "github.com/yornik/go-practice/exercises/39-circuitbreaker"
)

// Step 4: OnStateChange.

func TestOnStateChangeFullCycle(t *testing.T) {
	// One goroutine drives the breaker around the state diagram. After each
	// step the callback has already run for every change the step made.
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

	executeN(t, b, 1, errUnavailable)
	wantChanges(t, rec) // no change yet

	executeN(t, b, 1, errUnavailable)
	wantChanges(t, rec, closedToOpen)

	wantRejected(t, b, circuitbreaker.ErrOpen)
	wantChanges(t, rec, closedToOpen)

	clock.Advance(10 * time.Second)
	wantState(t, b, circuitbreaker.HalfOpen) // State makes the change
	wantChanges(t, rec, closedToOpen, openToHalfOpen)

	wantState(t, b, circuitbreaker.HalfOpen) // no new change, no new call
	wantChanges(t, rec, closedToOpen, openToHalfOpen)

	executeN(t, b, 1, errUnavailable) // the probe fails
	wantChanges(t, rec, closedToOpen, openToHalfOpen, halfOpenToOpen)

	clock.Advance(10 * time.Second)
	executeN(t, b, 1, nil) // one Execute, two changes
	wantChanges(t, rec, closedToOpen, openToHalfOpen, halfOpenToOpen, openToHalfOpen, halfOpenToClosed)

	executeN(t, b, 1, nil)
	wantChanges(t, rec, closedToOpen, openToHalfOpen, halfOpenToOpen, openToHalfOpen, halfOpenToClosed)
}

func TestOnStateChangeCanUseBreaker(t *testing.T) {
	// The callback calls State and Counts. If the breaker held its lock
	// while it calls OnStateChange, this would deadlock.
	type seen struct {
		Change change
		State  circuitbreaker.State
		Counts circuitbreaker.Counts
	}
	var (
		mu  sync.Mutex
		got []seen
		b   *circuitbreaker.Breaker
	)
	clock := newClock()
	b = newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Now:              clock.Now,
		OnStateChange: func(from, to circuitbreaker.State) {
			s := seen{Change: change{from, to}, State: b.State(), Counts: b.Counts()}
			mu.Lock()
			defer mu.Unlock()
			got = append(got, s)
		},
	})

	executeN(t, b, 1, errUnavailable)
	clock.Advance(10 * time.Second)
	wantState(t, b, circuitbreaker.HalfOpen)
	executeN(t, b, 1, nil)

	want := []seen{
		{closedToOpen, circuitbreaker.Open, circuitbreaker.Counts{}},
		{openToHalfOpen, circuitbreaker.HalfOpen, circuitbreaker.Counts{}},
		{halfOpenToClosed, circuitbreaker.Closed, circuitbreaker.Counts{}},
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(got, want) {
		t.Errorf("inside OnStateChange, State and Counts returned\n%+v\nwant\n%+v", got, want)
	}
}

func TestOnStateChangeDoesNotBlockOthers(t *testing.T) {
	// The callback for closed → open blocks until the test releases it.
	// Meanwhile other goroutines must be able to use the breaker.
	entered := make(chan change, 4)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()

	clock := newClock()
	b := newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Now:              clock.Now,
		OnStateChange: func(from, to circuitbreaker.State) {
			entered <- change{from, to}
			<-release
		},
	})

	tripped := goExecute(b, func() error { return errUnavailable })
	waitForChange(t, entered, closedToOpen)

	// OnStateChange(closed, open) is running now.
	wantState(t, b, circuitbreaker.Open)
	wantCounts(t, b, circuitbreaker.Counts{})
	wantRejected(t, b, circuitbreaker.ErrOpen)

	releaseOnce()
	if err := awaitExecute(t, tripped); err != errUnavailable {
		t.Errorf("Execute() = %v, want %v", err, errUnavailable)
	}
}

func TestOnStateChangeInOrder(t *testing.T) {
	// Goroutine A opens the breaker; its report of closed → open blocks.
	// Then the timeout passes and goroutine B calls State, which changes
	// the state to half-open. The report of open → half-open must wait
	// until the report of closed → open has returned: reports never
	// overlap and never overtake each other. Meanwhile the test itself
	// must still be able to use the breaker.
	entered := make(chan change, 4)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()

	clock := newClock()
	rec := &recorder{}
	// lookedAtClock tells the test that goroutine B's State has read the
	// clock, so B, not the test, makes the change to half-open.
	var timeoutPassed atomic.Bool
	lookedAtClock := make(chan struct{}, 1)
	b := newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Now: func() time.Time {
			if timeoutPassed.Load() {
				select {
				case lookedAtClock <- struct{}{}:
				default:
				}
			}
			return clock.Now()
		},
		OnStateChange: func(from, to circuitbreaker.State) {
			rec.OnStateChange(from, to)
			entered <- change{from, to}
			if to == circuitbreaker.Open {
				<-release
			}
		},
	})

	a := goExecute(b, func() error { return errUnavailable })
	waitForChange(t, entered, closedToOpen)

	clock.Advance(10 * time.Second)
	timeoutPassed.Store(true)
	bState := make(chan circuitbreaker.State, 1)
	go func() {
		defer func() { _ = recover() }() // a panic shows up as a missing result below
		bState <- b.State()
	}()
	select {
	case <-lookedAtClock:
	case <-time.After(timeout):
		t.Fatalf("goroutine B: State did not call Settings.Now within %v", timeout)
	}

	select {
	case c := <-entered:
		t.Fatalf("OnStateChange%v was called while OnStateChange%v was still running", c, closedToOpen)
	case <-time.After(window):
	}

	// B's change waits for its report, but the breaker is not blocked:
	// other goroutines can still use it.
	wantState(t, b, circuitbreaker.HalfOpen)
	wantCounts(t, b, circuitbreaker.Counts{})

	releaseOnce()
	if err := awaitExecute(t, a); err != errUnavailable {
		t.Errorf("goroutine A: Execute() = %v, want %v", err, errUnavailable)
	}
	select {
	case s := <-bState:
		if s != circuitbreaker.HalfOpen {
			t.Errorf("goroutine B: State() = %v, want %v", s, circuitbreaker.HalfOpen)
		}
	case <-time.After(timeout):
		t.Fatalf("goroutine B: State did not return within %v", timeout)
	}
	waitForChange(t, entered, openToHalfOpen)
	wantChanges(t, rec, closedToOpen, openToHalfOpen)
	if rec.overlapped.Load() {
		t.Errorf("two calls of OnStateChange ran at the same time")
	}
}

func TestOnStateChangeCallbackChangesState(t *testing.T) {
	// The callback for closed → open is slow: by the time it calls State,
	// the open timeout has passed, so State changes the state to half-open.
	// Then the callback calls Execute, whose probe succeeds and closes the
	// breaker. Those two changes cannot be reported while the callback
	// still runs (reports never overlap). They are reported right after it
	// returns, before the outer Execute returns. A breaker that makes the
	// callback wait for its own report deadlocks here.
	clock := newClock()
	rec := &recorder{}
	type inside struct {
		state circuitbreaker.State
		err   error
	}
	seen := make(chan inside, 1)
	var b *circuitbreaker.Breaker
	b = newBreaker(t, circuitbreaker.Settings{
		FailureThreshold: 1,
		OpenTimeout:      10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Now:              clock.Now,
		OnStateChange: func(from, to circuitbreaker.State) {
			rec.OnStateChange(from, to)
			if (change{from, to}) != closedToOpen {
				return
			}
			clock.Advance(10 * time.Second)
			var in inside
			in.state = b.State()
			in.err = b.Execute(func() error { return nil })
			seen <- in
		},
	})

	executeN(t, b, 1, errUnavailable)
	select {
	case in := <-seen:
		if in.state != circuitbreaker.HalfOpen {
			t.Errorf("inside OnStateChange%v: State() = %v, want half-open", closedToOpen, in.state)
		}
		if in.err != nil {
			t.Errorf("inside OnStateChange%v: Execute() = %v, want nil (admitted as a probe)", closedToOpen, in.err)
		}
	case <-time.After(timeout):
		t.Fatalf("OnStateChange%v was not called, or did not return, within %v", closedToOpen, timeout)
	}
	wantChanges(t, rec, closedToOpen, openToHalfOpen, halfOpenToClosed)
	if rec.overlapped.Load() {
		t.Errorf("two calls of OnStateChange ran at the same time")
	}
	wantState(t, b, circuitbreaker.Closed)
}

// goExecute calls b.Execute(fn) in a new goroutine. Read the result with
// awaitExecute.
func goExecute(b *circuitbreaker.Breaker, fn func() error) <-chan callResult {
	ch := make(chan callResult, 1)
	go func() {
		var res callResult
		defer func() {
			res.panicked = recover()
			ch <- res
		}()
		res.err = b.Execute(fn)
	}()
	return ch
}

func awaitExecute(t *testing.T, ch <-chan callResult) error {
	t.Helper()
	select {
	case res := <-ch:
		if res.panicked != nil {
			t.Fatalf("Execute panicked: %v", res.panicked)
		}
		return res.err
	case <-time.After(timeout):
		t.Fatalf("Execute did not return within %v", timeout)
		return nil
	}
}

// waitForChange waits until OnStateChange reports want on entered.
func waitForChange(t *testing.T, entered <-chan change, want change) {
	t.Helper()
	select {
	case got := <-entered:
		if got != want {
			t.Fatalf("OnStateChange%v was called, want OnStateChange%v", got, want)
		}
	case <-time.After(timeout):
		t.Fatalf("OnStateChange%v was not called within %v", want, timeout)
	}
}
