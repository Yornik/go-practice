package retry_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	retry "github.com/yornik/go-practice/exercises/07-retry"
)

// timeout bounds every call of Do and DoValue in these tests, so a wrong
// solution (for example one that really sleeps for an hour) fails with a
// message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

const ms = time.Millisecond

// Errors that the fake operations return.
var (
	errRefused    = errors.New("connection refused")
	errUnavail    = errors.New("503 Service Unavailable")
	errBadRequest = errors.New("400 Bad Request")
)

// basePolicy is the policy most tests use: 5 attempts, a backoff of 100ms
// that doubles up to 1s, no jitter. The backoffs are 100ms, 200ms, 400ms,
// 800ms, 1s, 1s, ... Tests set Sleep (and sometimes Rand) on a copy.
var basePolicy = retry.Policy{
	MaxAttempts: 5,
	Initial:     100 * ms,
	Max:         time.Second,
	Multiplier:  2,
}

// sleeper is a fake Policy.Sleep. It returns at once and records the delays
// it was asked to wait, so the tests run in microseconds and can check every
// delay exactly. If hook is set, it is called on every sleep (n is 1 for the
// first sleep) and its result is what Sleep returns; tests use it to cancel
// the context "during" a sleep. sleeper is safe for concurrent use.
type sleeper struct {
	hook func(ctx context.Context, n int) error

	mu     sync.Mutex
	delays []time.Duration
}

func (s *sleeper) Sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.delays = append(s.delays, d)
	n := len(s.delays)
	s.mu.Unlock()
	if s.hook != nil {
		return s.hook(ctx, n)
	}
	return nil
}

// Delays returns a copy of the recorded delays, in call order.
func (s *sleeper) Delays() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.delays)
}

// fakeOp is a scripted operation. Call number i (0-based) returns results[i].
// A call past the end of the script returns errNotScripted, so a solution
// that calls op too often fails with a clear message. fakeOp is safe for
// concurrent use.
type fakeOp struct {
	results []error
	// before, if set, runs at the start of call number n (1-based), with the
	// ctx that Do passed to op. Tests use it to cancel ctx during an attempt.
	before func(ctx context.Context, n int)

	mu    sync.Mutex
	calls int
}

var errNotScripted = errors.New("fakeOp: op was called more often than the test expects")

func (f *fakeOp) op(ctx context.Context) error {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	if f.before != nil {
		f.before(ctx, n)
	}
	if n > len(f.results) {
		return errNotScripted
	}
	return f.results[n-1]
}

// Calls returns how many times op was called.
func (f *fakeOp) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// failing returns n distinct errors that all wrap errRefused:
// "attempt 1: connection refused", "attempt 2: ...". Distinct errors let a
// test check that Do reports the error of the last attempt, not the first.
func failing(n int) []error {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = fmt.Errorf("attempt %d: %w", i+1, errRefused)
	}
	return errs
}

// runDo calls retry.Do in a new goroutine and waits for it. It fails the test
// if Do panics (for example with "TODO: implement") or does not return
// within the timeout.
func runDo(t *testing.T, ctx context.Context, p retry.Policy, op func(context.Context) error) error {
	t.Helper()
	_, err := within(t, "Do", func() (struct{}, error) {
		return struct{}{}, retry.Do(ctx, p, op)
	})
	return err
}

// runDoValue is runDo for retry.DoValue.
func runDoValue[T any](t *testing.T, ctx context.Context, p retry.Policy, op func(context.Context) (T, error)) (T, error) {
	t.Helper()
	return within(t, "DoValue", func() (T, error) {
		return retry.DoValue(ctx, p, op)
	})
}

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics or does not return within the timeout.
func within[T any](t *testing.T, what string, f func() (T, error)) (T, error) {
	t.Helper()
	type result struct {
		v        T
		err      error
		panicked any
	}
	ch := make(chan result, 1)
	go func() {
		var r result
		defer func() {
			r.panicked = recover()
			ch <- r
		}()
		r.v, r.err = f()
	}()
	select {
	case r := <-ch:
		if r.panicked != nil {
			t.Fatalf("%s panicked: %v", what, r.panicked)
		}
		return r.v, r.err
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v", what, timeout)
		var zero T
		return zero, nil
	}
}

// catchPanic turns a panic in the test's own goroutine, such as the
// "TODO: implement" of a function you have not written yet, into a test
// failure, so the other tests still run. Use it as: defer catchPanic(t).
func catchPanic(t *testing.T) {
	t.Helper()
	if v := recover(); v != nil {
		t.Fatalf("panicked: %v", v)
	}
}

// checkDelays fails the test if Do did not sleep exactly the expected delays.
func checkDelays(t *testing.T, s *sleeper, want ...time.Duration) {
	t.Helper()
	if got := s.Delays(); !slices.Equal(got, want) {
		t.Errorf("Do slept %v, want %v", got, want)
	}
}

// checkCalls fails the test if op was not called exactly want times.
func checkCalls(t *testing.T, f *fakeOp, want int) {
	t.Helper()
	if got := f.Calls(); got != want {
		t.Errorf("op was called %d times, want %d", got, want)
	}
}

// expiringContext is a context whose deadline passes when the test calls
// expire. It behaves like one made with context.WithTimeout, but the test
// decides when the deadline passes: after attempt 1, never before it, however
// slow the machine is. Err returns context.DeadlineExceeded once Done is
// closed.
type expiringContext struct {
	context.Context // context.Background(): no values, no deadline
	done            chan struct{}
	once            sync.Once
}

func newExpiringContext() *expiringContext {
	return &expiringContext{Context: context.Background(), done: make(chan struct{})}
}

func (c *expiringContext) Done() <-chan struct{} { return c.done }

func (c *expiringContext) Err() error {
	select {
	case <-c.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func (c *expiringContext) expire() { c.once.Do(func() { close(c.done) }) }
