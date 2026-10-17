package circuitbreaker_test

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	circuitbreaker "github.com/yornik/go-practice/exercises/14-circuitbreaker"
)

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// window is how long a test watches for something that must NOT happen. A
// correct solution never does it, so the window only decides how likely it
// is that a wrong solution gets caught. It never makes a correct one fail.
const window = 100 * time.Millisecond

var (
	errUnavailable = errors.New("503 service unavailable") // a failure
	errNotFound    = errors.New("404 not found")           // a failure, unless IsFailure says otherwise
)

// ignoreNotFound is an IsFailure func: a 404 means the dependency is up and
// answering, so it is not a failure.
func ignoreNotFound(err error) bool { return !errors.Is(err, errNotFound) }

// fakeClock is a clock that moves only when the test says so. Pass its Now
// method as Settings.Now. Safe for concurrent use.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// change is one call of OnStateChange.
type change struct{ From, To circuitbreaker.State }

func (c change) String() string { return fmt.Sprintf("(%v, %v)", c.From, c.To) }

var (
	closedToOpen     = change{circuitbreaker.Closed, circuitbreaker.Open}
	openToHalfOpen   = change{circuitbreaker.Open, circuitbreaker.HalfOpen}
	halfOpenToOpen   = change{circuitbreaker.HalfOpen, circuitbreaker.Open}
	halfOpenToClosed = change{circuitbreaker.HalfOpen, circuitbreaker.Closed}
)

// recorder records the calls of OnStateChange. Pass its OnStateChange
// method as Settings.OnStateChange. It also notices if two calls overlap.
// Safe for concurrent use.
type recorder struct {
	mu         sync.Mutex
	changes    []change
	running    atomic.Int32
	overlapped atomic.Bool
}

func (r *recorder) OnStateChange(from, to circuitbreaker.State) {
	if r.running.Add(1) > 1 {
		r.overlapped.Store(true)
	}
	defer r.running.Add(-1)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changes = append(r.changes, change{from, to})
}

func (r *recorder) Changes() []change {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.changes)
}

// wantChanges fails the test if the recorded changes are not want.
func wantChanges(t *testing.T, r *recorder, want ...change) {
	t.Helper()
	if got := r.Changes(); !slices.Equal(got, want) {
		t.Errorf("OnStateChange calls = %v, want %v", got, want)
	}
}

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics or does not return within the timeout. Every call into the breaker
// goes through it, so a forgotten Unlock or a "TODO: implement" panic shows
// up as a test failure, not as a hang or a crash of the whole test binary.
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
		t.Fatalf("%s did not return within %v (is a lock held, or not released?)", what, timeout)
	}
}

// newBreaker calls circuitbreaker.New and fails the test if it returns an
// error.
func newBreaker(t *testing.T, s circuitbreaker.Settings) *circuitbreaker.Breaker {
	t.Helper()
	var b *circuitbreaker.Breaker
	var err error
	within(t, "New", func() { b, err = circuitbreaker.New(s) })
	if err != nil || b == nil {
		t.Fatalf("New(FailureThreshold %d, OpenTimeout %v, HalfOpenMaxCalls %d, SuccessThreshold %d) = %v, %v; want a breaker and no error",
			s.FailureThreshold, s.OpenTimeout, s.HalfOpenMaxCalls, s.SuccessThreshold, b, err)
	}
	return b
}

// execute calls b.Execute(fn) and returns its error.
func execute(t *testing.T, b *circuitbreaker.Breaker, fn func() error) error {
	t.Helper()
	var err error
	within(t, "Execute", func() { err = b.Execute(fn) })
	return err
}

// executeN calls b.Execute n times with a fn that returns err. It fails the
// test unless every call is admitted (fn is called) and Execute returns err as
// is.
func executeN(t *testing.T, b *circuitbreaker.Breaker, n int, err error) {
	t.Helper()
	for i := range n {
		called := false
		got := execute(t, b, func() error { called = true; return err })
		if !called {
			t.Fatalf("call %d of %d: Execute did not call fn (it returned %v), want the call admitted", i+1, n, got)
		}
		if got != err {
			t.Fatalf("call %d of %d: Execute() = %v, want %v (fn's error, as is)", i+1, n, got, err)
		}
	}
}

// wantRejected calls b.Execute and fails the test unless it returns an error
// that matches want without calling fn.
func wantRejected(t *testing.T, b *circuitbreaker.Breaker, want error) {
	t.Helper()
	called := false
	err := execute(t, b, func() error { called = true; return nil })
	if called {
		t.Errorf("Execute called fn, want the call rejected with %v", want)
	}
	if !errors.Is(err, want) {
		t.Errorf("Execute() = %v, want %v", err, want)
	}
}

// stateOf calls b.State.
func stateOf(t *testing.T, b *circuitbreaker.Breaker) circuitbreaker.State {
	t.Helper()
	var s circuitbreaker.State
	within(t, "State", func() { s = b.State() })
	return s
}

func wantState(t *testing.T, b *circuitbreaker.Breaker, want circuitbreaker.State) {
	t.Helper()
	if got := stateOf(t, b); got != want {
		t.Errorf("State() = %v, want %v", got, want)
	}
}

// countsOf calls b.Counts.
func countsOf(t *testing.T, b *circuitbreaker.Breaker) circuitbreaker.Counts {
	t.Helper()
	var c circuitbreaker.Counts
	within(t, "Counts", func() { c = b.Counts() })
	return c
}

func wantCounts(t *testing.T, b *circuitbreaker.Breaker, want circuitbreaker.Counts) {
	t.Helper()
	if got := countsOf(t, b); got != want {
		t.Errorf("Counts() = %+v, want %+v", got, want)
	}
}

// slowCall is a call of Execute whose fn blocks until the test lets it
// return. While it blocks, the call is admitted and running.
type slowCall struct {
	release  chan error // what fn returns
	returned chan callResult
}

type callResult struct {
	err      error
	panicked any
}

// startSlowCall calls b.Execute in a new goroutine with a fn that blocks. It
// returns once fn is running. It fails the test if Execute returns without
// calling fn (for example with ErrTooManyCalls).
func startSlowCall(t *testing.T, b *circuitbreaker.Breaker) *slowCall {
	t.Helper()
	c := &slowCall{release: make(chan error, 1), returned: make(chan callResult, 1)}
	// If the test fails before it finishes the call, let fn return anyway.
	t.Cleanup(func() {
		select {
		case c.release <- errors.New("test ended"):
		default:
		}
	})
	started := make(chan struct{})
	go func() {
		var res callResult
		defer func() {
			res.panicked = recover()
			c.returned <- res
		}()
		res.err = b.Execute(func() error {
			close(started)
			return <-c.release
		})
	}()
	select {
	case <-started:
		return c
	case res := <-c.returned:
		t.Fatalf("Execute returned without calling fn (error %v, panic %v), want the call admitted", res.err, res.panicked)
	case <-time.After(timeout):
		t.Fatalf("Execute did not call fn within %v (is a lock held while fn of another call runs?)", timeout)
	}
	return nil
}

// finish makes fn return err, waits until Execute returns and returns
// Execute's error.
func (c *slowCall) finish(t *testing.T, err error) error {
	t.Helper()
	c.release <- err
	select {
	case res := <-c.returned:
		if res.panicked != nil {
			t.Fatalf("Execute panicked: %v", res.panicked)
		}
		return res.err
	case <-time.After(timeout):
		t.Fatalf("Execute did not return within %v after fn returned", timeout)
		return nil
	}
}

// finishWant is finish, and fails the test unless Execute returns err as is.
func (c *slowCall) finishWant(t *testing.T, err error) {
	t.Helper()
	if got := c.finish(t, err); got != err {
		t.Errorf("Execute() = %v, want %v (fn's error, as is)", got, err)
	}
}

// executePanics calls b.Execute with a fn that panics with v, and returns
// what Execute panicked with. It fails the test if Execute returns normally.
func executePanics(t *testing.T, b *circuitbreaker.Breaker, v any) any {
	t.Helper()
	type outcome struct {
		err      error
		panicked any
		returned bool
	}
	done := make(chan outcome, 1)
	go func() {
		var o outcome
		defer func() {
			if !o.returned {
				o.panicked = recover()
			}
			done <- o
		}()
		o.err = b.Execute(func() error { panic(v) })
		o.returned = true
	}()
	select {
	case o := <-done:
		if o.returned {
			t.Fatalf("Execute returned %v, want it to panic with %v like fn did", o.err, v)
		}
		return o.panicked
	case <-time.After(timeout):
		t.Fatalf("Execute did not return or panic within %v", timeout)
		return nil
	}
}

// toHalfOpen opens a Closed breaker with s.FailureThreshold failures, moves
// the clock by s.OpenTimeout and checks that the breaker is HalfOpen.
func toHalfOpen(t *testing.T, b *circuitbreaker.Breaker, clock *fakeClock, s circuitbreaker.Settings) {
	t.Helper()
	executeN(t, b, s.FailureThreshold, errUnavailable)
	if got := stateOf(t, b); got != circuitbreaker.Open {
		t.Fatalf("after %d failures: State() = %v, want open", s.FailureThreshold, got)
	}
	clock.Advance(s.OpenTimeout)
	if got := stateOf(t, b); got != circuitbreaker.HalfOpen {
		t.Fatalf("after OpenTimeout: State() = %v, want half-open", got)
	}
}
