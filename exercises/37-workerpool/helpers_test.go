package workerpool_test

import (
	"bytes"
	"context"
	"runtime"
	"runtime/pprof"
	"sync/atomic"
	"testing"
	"time"

	workerpool "github.com/yornik/go-practice/exercises/37-workerpool"
)

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// window is how long a test watches for something that must NOT happen, such
// as an extra call starting or Map returning too early. A correct solution
// never does it, so the window can only decide how likely it is that a wrong
// solution gets caught. It never makes a correct solution fail.
const window = 100 * time.Millisecond

// ints returns []int{0, 1, ..., n-1}. Item i is its own index, which makes it
// easy to check where each result ends up.
func ints(n int) []int {
	in := make([]int, n)
	for i := range in {
		in[i] = i
	}
	return in
}

// waitFor polls cond until it returns true. It fails the test if that takes
// longer than the timeout.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting until %s", timeout, what)
		}
		time.Sleep(time.Millisecond)
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

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics or does not return within the timeout.
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
		t.Fatalf("%s did not return within %v", what, timeout)
	}
}

// trackPeak raises peak to cur if cur is larger. Safe for concurrent use.
func trackPeak(peak *atomic.Int32, cur int32) {
	for {
		p := peak.Load()
		if cur <= p || peak.CompareAndSwap(p, cur) {
			return
		}
	}
}

// mapResult is what startMap delivers.
type mapResult[R any] struct {
	out      []R
	err      error
	panicked any // set if Map itself panicked in its caller's goroutine
}

// startMap calls workerpool.Map in a new goroutine. The returned channel
// receives the result once Map returns.
func startMap[T, R any](ctx context.Context, in []T, workers int, fn func(context.Context, T) (R, error)) <-chan mapResult[R] {
	ch := make(chan mapResult[R], 1)
	go func() {
		var res mapResult[R]
		defer func() {
			res.panicked = recover()
			ch <- res
		}()
		res.out, res.err = workerpool.Map(ctx, in, workers, fn)
	}()
	return ch
}

// await waits for the result of startMap. It fails the test if Map panicked
// or did not return within the timeout.
func await[R any](t *testing.T, ch <-chan mapResult[R]) ([]R, error) {
	t.Helper()
	select {
	case res := <-ch:
		if res.panicked != nil {
			t.Fatalf("Map panicked: %v", res.panicked)
		}
		return res.out, res.err
	case <-time.After(timeout):
		t.Fatalf("Map did not return within %v", timeout)
		return nil, nil
	}
}

// waitWhileMapRuns is waitFor for a Map started with startMap. If Map returns
// before cond holds, it fails at once and shows what Map returned.
func waitWhileMapRuns[R any](t *testing.T, ch <-chan mapResult[R], what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		select {
		case res := <-ch:
			if res.panicked != nil {
				t.Fatalf("Map panicked: %v", res.panicked)
			}
			t.Fatalf("Map returned (result %v, error %v) before %s", res.out, res.err, what)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting until %s", timeout, what)
		}
		time.Sleep(time.Millisecond)
	}
}

// runMap calls workerpool.Map and fails the test if it panics or hangs.
func runMap[T, R any](t *testing.T, ctx context.Context, in []T, workers int, fn func(context.Context, T) (R, error)) ([]R, error) {
	t.Helper()
	return await(t, startMap(ctx, in, workers, fn))
}

// expectNoLeak records how many goroutines exist now. The returned function
// fails the test if that number does not come back down within the timeout.
// Goroutines that already finished their work may need a moment to exit,
// which is why it polls. On failure it prints every goroutine's stack, so you
// can see where the leaked one is stuck.
func expectNoLeak(t *testing.T) func() {
	t.Helper()
	before := runtime.NumGoroutine()
	return func() {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for runtime.NumGoroutine() > before {
			if time.Now().After(deadline) {
				var stacks bytes.Buffer
				_ = pprof.Lookup("goroutine").WriteTo(&stacks, 1)
				t.Fatalf("%d goroutines are still running %v after Map returned, but only %d existed before Map was called: "+
					"a goroutine started by Map is stuck. All goroutines:\n%s",
					runtime.NumGoroutine(), timeout, before, stacks.String())
			}
			time.Sleep(time.Millisecond)
		}
	}
}
