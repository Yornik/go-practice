package ratelimit_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	ratelimit "github.com/yornik/go-practice/exercises/33-ratelimit"
)

const ms = time.Millisecond

// timeout bounds every wait for goroutines in these tests, so a wrong
// solution fails with a message instead of hanging.
const timeout = 5 * time.Second

// hangLimit is how long guard lets a single test run. A correct solution
// needs a few milliseconds per test.
const hangLimit = 10 * time.Second

// Tolerances: float rounding in your refill or wait formula must not decide
// whether a test passes.
const (
	tokenTolerance = 1e-9
	waitTolerance  = time.Microsecond
)

// t0 is where every fake clock starts.
var t0 = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// fakeClock is the clock the tests pass to New and NewKeyed. Time stands
// still until the test moves it. It is safe for concurrent use.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock { return &fakeClock{now: t0} }

// Now is the method value the tests pass as the now argument: clock.Now.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d (backwards if d is negative).
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// at describes the clock for failure messages: "at t0+250ms".
func (c *fakeClock) at() string {
	d := c.Now().Sub(t0)
	if d < 0 {
		return fmt.Sprintf("at t0-%v", -d)
	}
	return fmt.Sprintf("at t0+%v", d)
}

// guard turns the two ways a wrong solution can break the test binary into
// readable failures. Use it as the first line of a test: defer guard(t)().
//
//   - A panic in the test, such as the "TODO: implement" of a method you have
//     not written yet, becomes a test failure with the stack, and the other
//     tests still run.
//   - A call that never returns, for example because a method locks a
//     sync.Mutex that the same goroutine already holds, would hang until the
//     10-minute test timeout. After hangLimit, guard prints the stack of every
//     goroutine, so you can see where it is stuck, and stops the test binary.
func guard(t *testing.T) func() {
	t.Helper()
	name := t.Name()
	watchdog := time.AfterFunc(hangLimit, func() {
		var stacks bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
		fmt.Fprintf(os.Stderr, "\n%s is still running after %v: a call is blocked "+
			"(does a method lock a mutex that its goroutine already holds?).\n"+
			"Stacks of all goroutines:\n\n%s\n", name, hangLimit, stacks.String())
		os.Exit(2)
	})
	return func() {
		t.Helper()
		watchdog.Stop()
		if v := recover(); v != nil {
			t.Fatalf("panicked: %v\n%s", v, debug.Stack())
		}
	}
}

// panicValue calls f and returns what f panicked with, or nil.
func panicValue(f func()) (v any) {
	defer func() { v = recover() }()
	f()
	return nil
}

// parallel runs f(0), ..., f(n-1) in n goroutines, released at the same
// moment so that they really overlap, and waits for all of them. It fails
// the test if one of them panics or if they do not all return in time.
func parallel(t *testing.T, n int, f func(i int)) {
	t.Helper()
	start := make(chan struct{})
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		panics []any
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					mu.Lock()
					panics = append(panics, v)
					mu.Unlock()
				}
			}()
			<-start
			f(i)
		}()
	}
	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%d goroutines did not all return within %v: a call is blocked", n, timeout)
	}
	if len(panics) > 0 {
		t.Fatalf("%d of %d goroutines panicked, for example with: %v", len(panics), n, panics[0])
	}
}

// lim is a Limiter driven by a fake clock. Its methods call the Limiter and
// fail the test, with the clock in the message, if the result is wrong.
type lim struct {
	t     *testing.T
	clock *fakeClock
	l     *ratelimit.Limiter
}

func newLim(t *testing.T, rate float64, burst int) *lim {
	t.Helper()
	clock := newClock()
	return &lim{t: t, clock: clock, l: ratelimit.New(rate, burst, clock.Now)}
}

func (h *lim) allow(want bool) {
	h.t.Helper()
	if got := h.l.Allow(); got != want {
		h.t.Errorf("%s: Allow() = %v, want %v", h.clock.at(), got, want)
	}
}

func (h *lim) allowN(n int, want bool) {
	h.t.Helper()
	if got := h.l.AllowN(n); got != want {
		h.t.Errorf("%s: AllowN(%d) = %v, want %v", h.clock.at(), n, got, want)
	}
}

func (h *lim) tokens(want float64) {
	h.t.Helper()
	if got := h.l.Tokens(); math.Abs(got-want) > tokenTolerance {
		h.t.Errorf("%s: Tokens() = %v, want %v", h.clock.at(), got, want)
	}
}

// reserve calls Reserve(n) and expects the wait want and a nil error.
func (h *lim) reserve(n int, want time.Duration) {
	h.t.Helper()
	got, err := h.l.Reserve(n)
	if err != nil || !closeEnough(got, want) {
		h.t.Errorf("%s: Reserve(%d) = %v, %v; want %v, nil", h.clock.at(), n, got, err, want)
	}
}

// reserveErr calls Reserve(n) and expects 0 and an error that matches want.
func (h *lim) reserveErr(n int, want error) {
	h.t.Helper()
	got, err := h.l.Reserve(n)
	if !errors.Is(err, want) || got != 0 {
		h.t.Errorf("%s: Reserve(%d) = %v, %v; want 0 and an error that matches %q", h.clock.at(), n, got, err, want)
	}
}

func closeEnough(got, want time.Duration) bool {
	return (got - want).Abs() <= waitTolerance
}

// keyed is a Keyed driven by a fake clock, like lim.
type keyed struct {
	t     *testing.T
	clock *fakeClock
	k     *ratelimit.Keyed
}

func newKeyed(t *testing.T, rate float64, burst int, idle time.Duration) *keyed {
	t.Helper()
	clock := newClock()
	return &keyed{t: t, clock: clock, k: ratelimit.NewKeyed(rate, burst, idle, clock.Now)}
}

func (h *keyed) allow(key string, want bool) {
	h.t.Helper()
	if got := h.k.Allow(key); got != want {
		h.t.Errorf("%s: Allow(%q) = %v, want %v", h.clock.at(), key, got, want)
	}
}

func (h *keyed) sweep(want int) {
	h.t.Helper()
	if got := h.k.Sweep(); got != want {
		h.t.Errorf("%s: Sweep() = %d, want %d", h.clock.at(), got, want)
	}
}

func (h *keyed) length(want int) {
	h.t.Helper()
	if got := h.k.Len(); got != want {
		h.t.Errorf("%s: Len() = %d, want %d", h.clock.at(), got, want)
	}
}
