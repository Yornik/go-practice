package workerpool_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workerpool "github.com/yornik/go-practice/exercises/37-workerpool"
)

// callGo calls g.Go(f). It fails the test if Go panics or blocks for longer
// than the timeout; use it only where the limit is not reached.
func callGo(t *testing.T, g *workerpool.Group, f func() error) {
	t.Helper()
	within(t, "g.Go (the limit is not reached, so it must not block)", func() { g.Go(f) })
}

// tryGo calls g.TryGo(f). It fails the test if TryGo panics or blocks.
func tryGo(t *testing.T, g *workerpool.Group, f func() error) bool {
	t.Helper()
	var started bool
	within(t, "g.TryGo (it must never block)", func() { started = g.TryGo(f) })
	return started
}

// callWait calls g.Wait(). It fails the test if Wait panics or hangs.
func callWait(t *testing.T, g *workerpool.Group) error {
	t.Helper()
	var err error
	within(t, "g.Wait", func() { err = g.Wait() })
	return err
}

// startWait calls g.Wait in a new goroutine. The channel receives its result,
// or an error that describes a panic in Wait.
func startWait(g *workerpool.Group) <-chan error {
	ch := make(chan error, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				ch <- fmt.Errorf("g.Wait panicked: %v", v)
			}
		}()
		ch <- g.Wait()
	}()
	return ch
}

// blockUntil returns a function for Go or TryGo that blocks until gate is
// closed, and then returns nil.
func blockUntil(gate <-chan struct{}) func() error {
	return func() error {
		select {
		case <-gate:
			return nil
		case <-time.After(timeout):
			return errors.New("the test never opened the gate")
		}
	}
}

func TestGroupZeroValue(t *testing.T) {
	t.Run("Wait without any function returns nil", func(t *testing.T) {
		var g workerpool.Group
		if err := callWait(t, &g); err != nil {
			t.Errorf("Wait() = %v, want nil", err)
		}
	})

	t.Run("Wait waits for every function", func(t *testing.T) {
		var g workerpool.Group
		gate := make(chan struct{})
		openGate := sync.OnceFunc(func() { close(gate) })
		defer openGate()

		var finished atomic.Int32
		for range 10 {
			callGo(t, &g, func() error {
				err := blockUntil(gate)()
				finished.Add(1)
				return err
			})
		}
		waitDone := startWait(&g)
		select {
		case err := <-waitDone:
			t.Fatalf("Wait returned (%v) while 10 functions were still running: Wait must wait for all of them", err)
		case <-time.After(window):
		}

		openGate()
		select {
		case err := <-waitDone:
			if err != nil {
				t.Errorf("Wait() = %v, want nil", err)
			}
		case <-time.After(timeout):
			t.Fatalf("Wait did not return within %v after every function returned", timeout)
		}
		if got := finished.Load(); got != 10 {
			t.Errorf("%d of 10 functions had finished when Wait returned, want 10", got)
		}
	})
}

func TestGroupFirstError(t *testing.T) {
	defer catchPanic(t)
	var g workerpool.Group
	g.SetLimit(1) // one at a time, so "first" is well defined
	errA := errors.New("probe a failed")
	errB := errors.New("probe b failed")
	var ran atomic.Int32
	callGo(t, &g, func() error { ran.Add(1); return errA })
	callGo(t, &g, func() error { ran.Add(1); return errB })
	callGo(t, &g, func() error { ran.Add(1); return nil })

	err := callWait(t, &g)
	if !errors.Is(err, errA) {
		t.Errorf("Wait() = %v, want the first error %v", err, errA)
	}
	if got := ran.Load(); got != 3 {
		t.Errorf("%d of 3 functions ran, want 3: Go starts f even after an earlier function failed", got)
	}
}

func TestGroupConcurrentErrors(t *testing.T) {
	// 5 goroutines start 10 failing functions each, all at the same time.
	// Run this with -race: the group's bookkeeping must be safe for concurrent use.
	var g workerpool.Group
	errs := make([]error, 50)
	for i := range errs {
		errs[i] = fmt.Errorf("probe %d failed", i)
	}
	var ran atomic.Int32
	panics := make(chan any, 5)
	within(t, "calling g.Go from 5 goroutines at once", func() {
		var callers sync.WaitGroup
		for c := range 5 {
			callers.Add(1)
			go func() {
				defer callers.Done()
				defer func() {
					if v := recover(); v != nil {
						panics <- v
					}
				}()
				for _, err := range errs[c*10 : c*10+10] {
					g.Go(func() error { ran.Add(1); return err })
				}
			}()
		}
		callers.Wait()
	})
	if len(panics) > 0 {
		t.Fatalf("g.Go panicked: %v", <-panics)
	}

	err := callWait(t, &g)
	if !slices.Contains(errs, err) {
		t.Errorf("Wait() = %v, want one of the 50 errors the functions returned", err)
	}
	if got := ran.Load(); got != 50 {
		t.Errorf("%d of 50 functions ran, want 50", got)
	}
}

func TestGroupWithContext(t *testing.T) {
	t.Run("cancelled by the first error, with that error as the cause", func(t *testing.T) {
		defer catchPanic(t)
		g, ctx := workerpool.WithContext(context.Background())
		errA := errors.New("probe a failed")
		callGo(t, g, func() error { return errA })
		callGo(t, g, func() error {
			select {
			case <-ctx.Done():
				return ctx.Err() // a later error: Wait must not return this one
			case <-time.After(timeout):
				return errors.New("the group's ctx was not cancelled after a function failed")
			}
		})
		err := callWait(t, g)
		if !errors.Is(err, errA) {
			t.Errorf("Wait() = %v, want the first error %v", err, errA)
		}
		if cause := context.Cause(ctx); !errors.Is(cause, errA) {
			t.Errorf("context.Cause(ctx) = %v, want %v", cause, errA)
		}
	})

	t.Run("not cancelled while nothing fails, cancelled when Wait returns", func(t *testing.T) {
		defer catchPanic(t)
		g, ctx := workerpool.WithContext(context.Background())
		var errWhileRunning error
		callGo(t, g, func() error {
			errWhileRunning = ctx.Err()
			return nil
		})
		if err := callWait(t, g); err != nil {
			t.Errorf("Wait() = %v, want nil", err)
		}
		if errWhileRunning != nil {
			t.Errorf("ctx.Err() inside a running function = %v, want nil: nothing had failed yet", errWhileRunning)
		}
		if ctx.Err() == nil {
			t.Error("ctx.Err() after Wait returned = nil, want context.Canceled: Wait must cancel the group's ctx")
		}
	})

	t.Run("derived from the parent", func(t *testing.T) {
		defer catchPanic(t)
		parent, cancel := context.WithCancel(context.WithValue(context.Background(), traceIDKey{}, "trace-42"))
		defer cancel()
		g, ctx := workerpool.WithContext(parent)
		if got := ctx.Value(traceIDKey{}); got != "trace-42" {
			t.Errorf("ctx.Value(traceIDKey{}) = %v, want %q", got, "trace-42")
		}
		cancel()
		if ctx.Err() == nil {
			t.Error("the group's ctx is not done after its parent was cancelled")
		}
		if err := callWait(t, g); err != nil {
			t.Errorf("Wait() = %v, want nil", err)
		}
	})
}

func TestGroupLimit(t *testing.T) {
	defer catchPanic(t)
	const limit, total = 3, 10
	var g workerpool.Group
	g.SetLimit(limit)

	gate := make(chan struct{})
	openGate := sync.OnceFunc(func() { close(gate) })
	defer openGate()

	var started, inFlight, peak atomic.Int32
	f := func() error {
		started.Add(1)
		cur := inFlight.Add(1)
		defer inFlight.Add(-1)
		trackPeak(&peak, cur)
		return blockUntil(gate)()
	}

	// Go blocks while the limit is reached, so the calls happen in another goroutine.
	allGoReturned := make(chan any, 1)
	go func() {
		defer func() { allGoReturned <- recover() }()
		for range total {
			g.Go(f)
		}
	}()

	waitFor(t, "3 functions run at the same time", func() bool { return inFlight.Load() == limit })
	select {
	case v := <-allGoReturned:
		if v != nil {
			t.Fatalf("g.Go panicked: %v", v)
		}
		t.Fatalf("all %d calls to Go returned while %d functions were blocked: with SetLimit(%d), Go must block", total, limit, limit)
	case <-time.After(window):
	}
	if got := started.Load(); got != limit {
		t.Fatalf("%d functions started with SetLimit(%d) while the running ones were blocked, want %d", got, limit, limit)
	}

	openGate()
	select {
	case v := <-allGoReturned:
		if v != nil {
			t.Fatalf("g.Go panicked: %v", v)
		}
	case <-time.After(timeout):
		t.Fatalf("Go still blocked %v after the running functions returned", timeout)
	}
	if err := callWait(t, &g); err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
	if got := started.Load(); got != total {
		t.Errorf("%d functions ran, want %d", got, total)
	}
	if got := peak.Load(); got != limit {
		t.Errorf("up to %d functions ran at the same time, want at most %d", got, limit)
	}
}

func TestGroupNoLimit(t *testing.T) {
	tests := []struct {
		name  string
		setup func(g *workerpool.Group)
	}{
		{"zero value", func(*workerpool.Group) {}},
		{"SetLimit(-1) removes a limit", func(g *workerpool.Group) { g.SetLimit(2); g.SetLimit(-1) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			const n = 50
			var g workerpool.Group
			tc.setup(&g)
			gate := make(chan struct{})
			openGate := sync.OnceFunc(func() { close(gate) })
			defer openGate()

			var arrived atomic.Int32
			for range n {
				callGo(t, &g, func() error {
					arrived.Add(1)
					return blockUntil(gate)()
				})
			}
			waitFor(t, "all 50 functions run at the same time", func() bool { return arrived.Load() == n })
			openGate()
			if err := callWait(t, &g); err != nil {
				t.Errorf("Wait() = %v, want nil", err)
			}
		})
	}
}

func TestGroupTryGo(t *testing.T) {
	t.Run("respects the limit and never blocks", func(t *testing.T) {
		defer catchPanic(t)
		var g workerpool.Group
		g.SetLimit(2)
		gate := make(chan struct{})
		openGate := sync.OnceFunc(func() { close(gate) })
		defer openGate()

		for i := range 2 {
			if !tryGo(t, &g, blockUntil(gate)) {
				t.Fatalf("TryGo with %d of 2 slots in use = false, want true", i)
			}
		}
		var extraRan atomic.Bool
		if tryGo(t, &g, func() error { extraRan.Store(true); return nil }) {
			t.Errorf("TryGo with 2 of 2 slots in use = true, want false")
		}

		openGate()
		// Once the blocked functions have returned, their slots are free again.
		waitFor(t, "TryGo returns true after the running functions returned", func() bool {
			return tryGo(t, &g, func() error { return nil })
		})
		if err := callWait(t, &g); err != nil {
			t.Errorf("Wait() = %v, want nil", err)
		}
		if extraRan.Load() {
			t.Error("TryGo returned false but ran the function anyway")
		}
	})

	t.Run("called from many goroutines at once", func(t *testing.T) {
		defer catchPanic(t)
		// Repeated, because a TryGo that first checks for a free slot and then
		// takes it in a separate step goes wrong only when another caller gets
		// in between the two steps.
		for attempt := range 100 {
			raceTryGo(t, attempt)
		}
	})

	t.Run("without a limit it always starts f", func(t *testing.T) {
		var g workerpool.Group
		gate := make(chan struct{})
		openGate := sync.OnceFunc(func() { close(gate) })
		defer openGate()

		for i := range 20 {
			if !tryGo(t, &g, blockUntil(gate)) {
				t.Fatalf("TryGo on a Group without a limit, %d functions running = false, want true", i)
			}
		}
		openGate()
		if err := callWait(t, &g); err != nil {
			t.Errorf("Wait() = %v, want nil", err)
		}
	})
}

// raceTryGo lets 20 goroutines call TryGo at the same moment on a Group with
// SetLimit(2) whose functions block. Exactly 2 calls must return true, and
// the 18 others must return false at once.
func raceTryGo(t *testing.T, attempt int) {
	t.Helper()
	const limit, callers = 2, 20
	var g workerpool.Group
	g.SetLimit(limit)
	gate := make(chan struct{})
	openGate := sync.OnceFunc(func() { close(gate) })
	defer openGate() // unblock everything, also if the test fails early

	startLine := make(chan struct{})
	results := make(chan any, callers) // true, false, or what TryGo panicked with
	for range callers {
		go func() {
			defer func() {
				if v := recover(); v != nil {
					results <- fmt.Sprintf("g.TryGo panicked: %v", v)
				}
			}()
			<-startLine
			results <- g.TryGo(func() error { <-gate; return nil })
		}()
	}
	close(startLine) // all callers start at once

	won := 0
	for returned := range callers {
		select {
		case res := <-results:
			switch res {
			case true:
				won++
			case false:
			default:
				t.Fatal(res)
			}
		case <-time.After(timeout):
			t.Fatalf("attempt %d: only %d of %d concurrent TryGo calls returned within %v while all %d slots were taken: "+
				"TryGo must never block, also when other goroutines call it at the same time", attempt, returned, callers, timeout, limit)
		}
	}
	if won != limit {
		t.Fatalf("attempt %d: %d of %d concurrent TryGo calls returned true with SetLimit(%d), want %d", attempt, won, callers, limit, limit)
	}
	openGate()
	if err := callWait(t, &g); err != nil {
		t.Fatalf("Wait() = %v, want nil", err)
	}
}

// crashingTask is a task with a bug. It is a named function so that the test
// can look for its name in the stack trace.
func crashingTask() error {
	panic("task crashed")
}

func TestGroupRecoversPanics(t *testing.T) {
	tests := []struct {
		name  string
		start func(t *testing.T, g *workerpool.Group)
	}{
		{"Go", func(t *testing.T, g *workerpool.Group) { callGo(t, g, crashingTask) }},
		{"TryGo", func(t *testing.T, g *workerpool.Group) {
			if !tryGo(t, g, crashingTask) {
				t.Fatal("TryGo on a Group without a limit = false, want true")
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			g, ctx := workerpool.WithContext(context.Background())
			tc.start(t, g)
			err := callWait(t, g)

			var pe *workerpool.PanicError
			if !errors.As(err, &pe) {
				t.Fatalf("Wait() = %v (%T), want a *workerpool.PanicError", err, err)
			}
			if pe.Value != "task crashed" {
				t.Errorf("PanicError.Value = %#v, want %q", pe.Value, "task crashed")
			}
			if !bytes.Contains(pe.Stack, []byte("crashingTask")) {
				t.Errorf("PanicError.Stack does not mention crashingTask, the function that panicked. Stack:\n%s", pe.Stack)
			}
			if cause := context.Cause(ctx); !errors.As(cause, &pe) {
				t.Errorf("context.Cause(ctx) = %v, want the *PanicError: a panic cancels the group's ctx like an error", cause)
			}
		})
	}
}
