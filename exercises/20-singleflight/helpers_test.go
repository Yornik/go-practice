package singleflight_test

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	singleflight "github.com/yornik/go-practice/exercises/20-singleflight"
)

// group is the Group type all tests use: string keys, string values.
type group = singleflight.Group[string, string]

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// window is how long a test watches for something that must NOT happen, such
// as fn's ctx being cancelled while a caller still waits. A correct solution
// never does it, so the window only decides how likely it is that a wrong
// solution gets caught. It never makes a correct solution fail.
const window = 50 * time.Millisecond

// joinWindows are how long a test gives the callers it just started to reach
// Do (or DoContext) and join the execution in flight. That takes
// microseconds, but on a busy machine a new goroutine can wait a while before
// it runs. A caller that arrives after the execution has finished is
// allowed to start a new one, so such a test cannot fail a correct solution:
// it only learns nothing. Then it tries again with a longer window.
var joinWindows = []time.Duration{50 * time.Millisecond, 250 * time.Millisecond, time.Second}

// untilJoined runs attempt with each window of joinWindows until attempt
// reports that at least one caller it started late joined the execution in
// flight, or the test failed.
func untilJoined(t *testing.T, attempt func(window time.Duration) (joined int)) {
	t.Helper()
	for i, w := range joinWindows {
		if attempt(w) > 0 || t.Failed() {
			return
		}
		if i < len(joinWindows)-1 {
			t.Logf("no caller joined within %v; trying again with a longer window", w)
		}
	}
	t.Fatalf("none of the callers that the test started while fn was running joined that execution, "+
		"even with %v to reach the group. A caller with the same key must wait for the execution in "+
		"flight and share its result. Does a caller wait for a lock that is held while fn runs?",
		joinWindows[len(joinWindows)-1])
}

// loader is a fake slow backend, like the database query behind a cache.
// Every call of fn or fnCtx is one execution: it counts itself, announces on
// started that it is running (with the ctx it got), and then blocks until the
// test calls release. Then it returns val and err, or panics with panicValue
// if that is set. It ignores ctx on purpose, like a query that does not
// check for cancellation, so the tests see that callers can leave anyway.
type loader struct {
	val        string
	err        error
	panicValue any

	calls    atomic.Int32
	returned atomic.Int32
	started  chan context.Context
	gate     chan struct{}
	once     sync.Once
}

// newLoader returns a loader that returns val. It is released when the test
// ends, so no goroutine stays blocked in it.
func newLoader(t *testing.T, val string) *loader {
	t.Helper()
	l := &loader{val: val, started: make(chan context.Context, 100), gate: make(chan struct{})}
	t.Cleanup(l.release)
	return l
}

// fn is the loader for Do and DoChan.
func (l *loader) fn() (string, error) {
	return l.fnCtx(context.Background())
}

// fnCtx is the loader for DoContext.
func (l *loader) fnCtx(ctx context.Context) (string, error) {
	l.calls.Add(1)
	defer l.returned.Add(1)
	l.started <- ctx
	<-l.gate
	if l.panicValue != nil {
		panic(l.panicValue)
	}
	return l.val, l.err
}

// release lets every call of the loader return, now and in the future.
func (l *loader) release() {
	l.once.Do(func() { close(l.gate) })
}

// waitStarted waits until a call of the loader has started and returns the
// ctx that call got.
func (l *loader) waitStarted(t *testing.T) context.Context {
	t.Helper()
	select {
	case ctx := <-l.started:
		return ctx
	case <-time.After(timeout):
		t.Fatalf("fn was not called within %v", timeout)
		return nil
	}
}

// waitStartedBy is waitStarted for a call started with startDo or
// startDoContext. If that call returns before fn has started, for example
// because it panicked with "TODO: implement", it fails at once and shows what
// the call did, instead of waiting for the timeout.
func (l *loader) waitStartedBy(t *testing.T, what string, out <-chan outcome) context.Context {
	t.Helper()
	select {
	case ctx := <-l.started:
		return ctx
	case o := <-out:
		if o.panicked != nil {
			t.Fatalf("%s panicked before fn was called: %v", what, o.panicked)
		}
		t.Fatalf("%s returned %q, %v, %v before fn was called", what, o.val, o.err, o.shared)
	case <-time.After(timeout):
		t.Fatalf("fn was not called within %v", timeout)
	}
	return nil
}

// quick is a fn that returns val at once and counts its calls. Tests use it
// for callers that are expected to join an execution in flight: such a
// caller never calls its own fn.
type quick struct {
	val   string
	calls atomic.Int32
}

func newQuick(val string) *quick { return &quick{val: val} }

func (q *quick) fn() (string, error) {
	q.calls.Add(1)
	return q.val, nil
}

func (q *quick) fnCtx(context.Context) (string, error) { return q.fn() }

// outcome is what a call of Do or DoContext did: the values it returned, or
// the value it panicked with.
type outcome struct {
	val      string
	err      error
	shared   bool
	panicked any
}

// startDo calls g.Do in a new goroutine. The channel receives the outcome.
func startDo(g *group, key string, fn func() (string, error)) <-chan outcome {
	ch := make(chan outcome, 1)
	go func() {
		var o outcome
		defer func() {
			o.panicked = recover()
			ch <- o
		}()
		o.val, o.err, o.shared = g.Do(key, fn)
	}()
	return ch
}

// startDoContext calls g.DoContext in a new goroutine. The channel receives
// the outcome.
func startDoContext(ctx context.Context, g *group, key string, fn func(context.Context) (string, error)) <-chan outcome {
	ch := make(chan outcome, 1)
	go func() {
		var o outcome
		defer func() {
			o.panicked = recover()
			ch <- o
		}()
		o.val, o.err, o.shared = g.DoContext(ctx, key, fn)
	}()
	return ch
}

// await waits for an outcome. It fails the test if none arrives in time.
func await(t *testing.T, what string, ch <-chan outcome) outcome {
	t.Helper()
	select {
	case o := <-ch:
		return o
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v", what, timeout)
		return outcome{}
	}
}

// want checks that a call returned val, a nil error and shared, without
// panicking.
func want(t *testing.T, what string, o outcome, val string, shared bool) {
	t.Helper()
	if o.panicked != nil {
		t.Errorf("%s panicked: %v", what, o.panicked)
		return
	}
	if o.val != val || o.err != nil || o.shared != shared {
		t.Errorf("%s = %q, %v, %v; want %q, <nil>, %v", what, o.val, o.err, o.shared, val, shared)
	}
}

// wantPanicError checks that a call panicked with a *PanicError for value
// and returns it.
func wantPanicError(t *testing.T, what string, o outcome, value any) *singleflight.PanicError {
	t.Helper()
	if o.panicked == nil {
		t.Errorf("%s = %q, %v, %v; want a panic with a *singleflight.PanicError", what, o.val, o.err, o.shared)
		return nil
	}
	pe, ok := o.panicked.(*singleflight.PanicError)
	if !ok {
		t.Errorf("%s panicked with %v (type %T), want a *singleflight.PanicError", what, o.panicked, o.panicked)
		return nil
	}
	if pe.Value != value {
		t.Errorf("%s panicked with a PanicError with Value %v, want %v", what, pe.Value, value)
	}
	return pe
}

// do calls g.Do and waits for it.
func do(t *testing.T, g *group, key string, fn func() (string, error)) outcome {
	t.Helper()
	return await(t, "Do("+key+")", startDo(g, key, fn))
}

// doChan calls g.DoChan. It fails the test if DoChan panics or does not
// return at once: DoChan must not wait for fn.
func doChan(t *testing.T, g *group, key string, fn func() (string, error)) <-chan singleflight.Result[string] {
	t.Helper()
	type ret struct {
		ch       <-chan singleflight.Result[string]
		panicked any
	}
	done := make(chan ret, 1)
	go func() {
		var r ret
		defer func() {
			r.panicked = recover()
			done <- r
		}()
		r.ch = g.DoChan(key, fn)
	}()
	select {
	case r := <-done:
		if r.panicked != nil {
			t.Fatalf("DoChan(%q) panicked: %v", key, r.panicked)
		}
		if r.ch == nil {
			t.Fatalf("DoChan(%q) returned a nil channel", key)
		}
		return r.ch
	case <-time.After(timeout):
		t.Fatalf("DoChan(%q) did not return within %v: it must return at once and not wait for fn", key, timeout)
		return nil
	}
}

// receive waits for the Result on a channel from DoChan.
func receive(t *testing.T, what string, ch <-chan singleflight.Result[string]) singleflight.Result[string] {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(timeout):
		t.Fatalf("%s: no Result on the channel within %v", what, timeout)
		return singleflight.Result[string]{}
	}
}

// wantResult checks a Result from DoChan.
func wantResult(t *testing.T, what string, got singleflight.Result[string], val string, shared bool) {
	t.Helper()
	if got.Val != val || got.Err != nil || got.Shared != shared {
		t.Errorf("%s = %+v, want {Val:%s Err:<nil> Shared:%v}", what, got, val, shared)
	}
}

// forget calls g.Forget and fails the test if it panics or blocks.
func forget(t *testing.T, g *group, key string) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		g.Forget(key)
	}()
	select {
	case v := <-done:
		if v != nil {
			t.Fatalf("Forget(%q) panicked: %v", key, v)
		}
	case <-time.After(timeout):
		t.Fatalf("Forget(%q) did not return within %v", key, timeout)
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
		t.Fatalf("%s did not finish within %v", what, timeout)
	}
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

// doneWithin reports whether ctx is done within d.
func doneWithin(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(d):
		return false
	}
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
				t.Fatalf("%d goroutines are still running %v after every call returned and fn finished, "+
					"but only %d existed before: a goroutine of the group is stuck, probably sending a "+
					"result that nobody receives. All goroutines:\n%s",
					runtime.NumGoroutine(), timeout, before, stacks.String())
			}
			time.Sleep(time.Millisecond)
		}
	}
}

// errBackend is an error that the fake backends return.
var errBackend = errors.New("connection refused")
