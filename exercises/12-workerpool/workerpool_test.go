package workerpool_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workerpool "github.com/yornik/go-practice/exercises/12-workerpool"
)

// errProbe is what a failing health probe returns in these tests.
var errProbe = errors.New("probe failed: connection refused")

// traceIDKey is a context key, like the ones tracing libraries use to carry a
// request's trace ID.
type traceIDKey struct{}

// ----- PanicError -----

func TestPanicErrorError(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"string", "boom", "panic: boom"},
		{"int", 42, "panic: 42"},
		{"error", errors.New("db down"), "panic: db down"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			// The stack is set to show that Error must not print it.
			e := &workerpool.PanicError{Value: tc.value, Stack: []byte("goroutine 7 [running]:")}
			if got := e.Error(); got != tc.want {
				t.Errorf("(&PanicError{Value: %#v}).Error() = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestPanicErrorUnwrap(t *testing.T) {
	t.Run("value is an error", func(t *testing.T) {
		defer catchPanic(t)
		cause := errors.New("db down")
		e := &workerpool.PanicError{Value: cause}
		if got := e.Unwrap(); got != cause {
			t.Errorf("Unwrap() = %v, want the panic value %v", got, cause)
		}
		if !errors.Is(e, cause) {
			t.Errorf("errors.Is(panicErr, cause) = false, want true")
		}
	})
	t.Run("value is not an error", func(t *testing.T) {
		defer catchPanic(t)
		e := &workerpool.PanicError{Value: "boom"}
		if got := e.Unwrap(); got != nil {
			t.Errorf("Unwrap() with Value %q = %v, want nil", "boom", got)
		}
	})
}

// ----- Map: results -----

func TestMapResultsInInputOrder(t *testing.T) {
	const n = 500 // the scenario: 500 probes
	in := ints(n)
	for _, workers := range []int{1, 4, 16, n, 2 * n} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			var calls [n]atomic.Int32
			fn := func(_ context.Context, i int) (string, error) {
				calls[i].Add(1)
				runtime.Gosched() // let the calls overlap and finish in a mixed-up order
				return fmt.Sprintf("probe-%03d", i), nil
			}
			out, err := runMap(t, context.Background(), in, workers, fn)
			if err != nil {
				t.Fatalf("Map error = %v, want nil", err)
			}
			if len(out) != n {
				t.Fatalf("len(result) = %d, want %d", len(out), n)
			}
			for i, got := range out {
				if want := fmt.Sprintf("probe-%03d", i); got != want {
					t.Fatalf("result[%d] = %q, want %q: results must be in input order", i, got, want)
				}
			}
			for i := range calls {
				if got := calls[i].Load(); got != 1 {
					t.Fatalf("fn was called %d times for item %d, want exactly 1", got, i)
				}
			}
		})
	}
}

func TestMapResultsWhenCallsFinishInReverseOrder(t *testing.T) {
	// Item i can only finish after item i+1 has finished, so the calls finish
	// in exactly the reverse of the input order. With workers == len(in) all
	// calls run at the same time, so this cannot deadlock.
	const n = 8
	finished := make([]chan struct{}, n)
	for i := range finished {
		finished[i] = make(chan struct{})
	}
	fn := func(_ context.Context, i int) (int, error) {
		if i+1 < n {
			select {
			case <-finished[i+1]:
			case <-time.After(timeout):
				return 0, fmt.Errorf("item %d waited %v for item %d to finish: with workers == len(in), all calls must run at the same time", i, timeout, i+1)
			}
		}
		close(finished[i])
		return i * i, nil
	}
	out, err := runMap(t, context.Background(), ints(n), n, fn)
	if err != nil {
		t.Fatalf("Map error = %v, want nil", err)
	}
	want := []int{0, 1, 4, 9, 16, 25, 36, 49}
	if fmt.Sprint(out) != fmt.Sprint(want) {
		t.Errorf("Map result = %v, want %v: result[i] belongs to in[i], whatever order the calls finish in", out, want)
	}
}

func TestMapEmptyInput(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name string
		ctx  context.Context
		in   []int
	}{
		{"nil slice", context.Background(), nil},
		{"empty slice", context.Background(), []int{}},
		// With no items, no item can be missing, so a done ctx is no error.
		{"empty slice, ctx already cancelled", cancelled, []int{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			fn := func(context.Context, int) (int, error) {
				calls.Add(1)
				return 0, nil
			}
			out, err := runMap(t, tc.ctx, tc.in, 4, fn)
			if err != nil {
				t.Errorf("Map error = %v, want nil", err)
			}
			if out == nil || len(out) != 0 {
				t.Errorf("Map result = %#v, want an empty, non-nil slice", out)
			}
			if got := calls.Load(); got != 0 {
				t.Errorf("fn was called %d times, want 0", got)
			}
		})
	}
}

func TestMapInvalidWorkers(t *testing.T) {
	tests := []struct {
		workers int
		in      []int
	}{
		{0, ints(3)},
		{-1, ints(3)},
		// A bad workers value is a bug in the caller. Report it even when
		// there happens to be nothing to do, so it shows up in the first test
		// run and not in the first real rollout.
		{0, nil},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("workers=%d,len(in)=%d", tc.workers, len(tc.in)), func(t *testing.T) {
			workers := tc.workers
			var calls atomic.Int32
			fn := func(_ context.Context, i int) (int, error) {
				calls.Add(1)
				return i, nil
			}
			out, err := runMap(t, context.Background(), tc.in, workers, fn)
			if err == nil {
				t.Errorf("Map with workers=%d, %d items: error = nil, want an error", workers, len(tc.in))
			}
			if out != nil {
				t.Errorf("Map with workers=%d, %d items: result = %v, want nil", workers, len(tc.in), out)
			}
			if got := calls.Load(); got != 0 {
				t.Errorf("fn was called %d times, want 0", got)
			}
		})
	}
}

// ----- Map: concurrency limit -----

func TestMapLimitsConcurrency(t *testing.T) {
	const items = 20
	for _, workers := range []int{1, 3, 8} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			gate := make(chan struct{})
			openGate := sync.OnceFunc(func() { close(gate) })
			defer openGate() // unblock the calls even if the test fails early

			var started, inFlight, peak atomic.Int32
			fn := func(_ context.Context, i int) (int, error) {
				started.Add(1)
				cur := inFlight.Add(1)
				defer inFlight.Add(-1)
				trackPeak(&peak, cur)
				select {
				case <-gate:
					return i, nil
				case <-time.After(timeout):
					return 0, errors.New("the test never opened the gate")
				}
			}
			ch := startMap(context.Background(), ints(items), workers, fn)

			// Map must use all its workers: that many calls block at the gate.
			waitWhileMapRuns(t, ch, fmt.Sprintf("%d calls of fn run at the same time (Map must run up to workers calls in parallel)", workers),
				func() bool { return inFlight.Load() == int32(workers) })
			// The running calls stay blocked. Give a wrong solution time to start more.
			time.Sleep(window)
			if got := started.Load(); got != int32(workers) {
				t.Fatalf("workers=%d: fn was called %d times while the running calls were blocked, want %d", workers, got, workers)
			}

			openGate()
			out, err := await(t, ch)
			if err != nil {
				t.Fatalf("Map error = %v, want nil", err)
			}
			for i, got := range out {
				if got != i {
					t.Fatalf("result[%d] = %d, want %d", i, got, i)
				}
			}
			if got := peak.Load(); got != int32(workers) {
				t.Errorf("up to %d calls of fn ran at the same time, want at most workers=%d", got, workers)
			}
		})
	}
}

// ----- Map: failures -----

func TestMapStopsAtFirstError(t *testing.T) {
	t.Run("workers=1: no call after the failing one", func(t *testing.T) {
		defer catchPanic(t) // errors.Is calls Unwrap if Map returns a *PanicError
		var calls atomic.Int32
		fn := func(_ context.Context, i int) (int, error) {
			if calls.Add(1) == 4 {
				return 0, errProbe
			}
			return i, nil
		}
		out, err := runMap(t, context.Background(), ints(10), 1, fn)
		if !errors.Is(err, errProbe) {
			t.Errorf("Map error = %v, want %v", err, errProbe)
		}
		if out != nil {
			t.Errorf("Map result = %v, want nil when Map fails", out)
		}
		if got := calls.Load(); got != 4 {
			t.Errorf("fn was called %d times, want 4: the 4th call failed, so Map must not start a 5th", got)
		}
	})

	t.Run("workers=1: a panic stops Map like an error", func(t *testing.T) {
		defer catchPanic(t)
		var calls atomic.Int32
		fn := func(_ context.Context, i int) (int, error) {
			if calls.Add(1) == 4 {
				panic("probe crashed")
			}
			return i, nil
		}
		out, err := runMap(t, context.Background(), ints(10), 1, fn)
		var pe *workerpool.PanicError
		if !errors.As(err, &pe) {
			t.Errorf("Map error = %v (%T), want a *workerpool.PanicError", err, err)
		}
		if out != nil {
			t.Errorf("Map result = %v, want nil when Map fails", out)
		}
		if got := calls.Load(); got != 4 {
			t.Errorf("fn was called %d times, want 4: the 4th call panicked, so Map must not start a 5th", got)
		}
	})

	t.Run("workers=4: the first error wins and cancels the other calls", func(t *testing.T) {
		defer catchPanic(t)
		const workers = 4
		var started atomic.Int32
		causes := make(chan error, workers)
		fn := func(ctx context.Context, _ int) (int, error) {
			if started.Add(1) == workers {
				// The 3 calls that started before this one are blocked below.
				return 0, errProbe
			}
			select {
			case <-ctx.Done():
				causes <- context.Cause(ctx)
				return 0, ctx.Err() // a later error: Map must not return this one
			case <-time.After(timeout):
				return 0, errors.New("fn blocked for 5s: either the ctx passed to fn was not cancelled after another call failed, " +
					"or Map runs fewer than workers calls at once")
			}
		}
		out, err := runMap(t, context.Background(), ints(20), workers, fn)
		if !errors.Is(err, errProbe) {
			t.Errorf("Map error = %v, want the first error %v (not the context.Canceled that the cancelled calls returned after it)", err, errProbe)
		}
		if out != nil {
			t.Errorf("Map result = %v, want nil when Map fails", out)
		}
		if got := started.Load(); got != workers {
			t.Errorf("fn was called %d times, want %d: no new call may start after a call failed", got, workers)
		}
		for range workers - 1 {
			select {
			case cause := <-causes:
				if !errors.Is(cause, errProbe) {
					t.Errorf("context.Cause(ctx) inside a cancelled call = %v, want %v: cancel fn's ctx with the first error as the cause", cause, errProbe)
				}
			case <-time.After(timeout):
				t.Fatal("a blocked call never saw its ctx cancelled")
			}
		}
	})
}

func TestMapWaitsForRunningCalls(t *testing.T) {
	// Two ways to stop a run: a call fails (the 3rd one here), or the caller
	// cancels ctx. Either way Map must wait for the calls that are running.
	tests := []struct {
		name       string
		thirdFails bool // if false, the test cancels ctx once 3 calls run
		want       error
	}{
		{"after a call failed", true, errProbe},
		{"after ctx was cancelled", false, context.Canceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const workers = 3
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			release := make(chan struct{})
			openRelease := sync.OnceFunc(func() { close(release) })
			defer openRelease()

			var started, running, cancelled atomic.Int32
			fn := func(ctx context.Context, _ int) (int, error) {
				running.Add(1)
				defer running.Add(-1)
				if started.Add(1) == workers && tc.thirdFails {
					return 0, errProbe
				}
				select {
				case <-ctx.Done():
				case <-time.After(timeout):
					return 0, errors.New("the ctx passed to fn was not cancelled after the run stopped")
				}
				cancelled.Add(1)
				// Like a real call that still has to close its connection after
				// the cancellation: it returns only when the test says so.
				select {
				case <-release:
				case <-time.After(timeout):
				}
				return 0, ctx.Err()
			}
			ch := startMap(ctx, ints(10), workers, fn)

			blocked := int32(workers - 1) // the calls besides the failing one
			if !tc.thirdFails {
				blocked = workers
				waitWhileMapRuns(t, ch, "3 calls of fn run at the same time", func() bool { return started.Load() == workers })
				cancel()
			}
			waitWhileMapRuns(t, ch, fmt.Sprintf("the %d running calls see their ctx cancelled", blocked), func() bool {
				// A call started after the stop would also block above and
				// make this wait time out, so name that bug instead.
				if got := started.Load(); got > workers {
					t.Fatalf("fn was called %d times, want %d: no new call may start after the run stopped", got, workers)
				}
				return cancelled.Load() == blocked
			})
			select {
			case res := <-ch:
				t.Fatalf("Map returned (error %v) while %d calls of fn were still running: Map must wait for them", res.err, running.Load())
			case <-time.After(window):
			}

			openRelease()
			_, err := await(t, ch)
			if !errors.Is(err, tc.want) {
				t.Errorf("Map error = %v, want %v", err, tc.want)
			}
			if got := running.Load(); got != 0 {
				t.Errorf("%d calls of fn were still running after Map returned, want 0", got)
			}
		})
	}
}

// crashOnSeven is a probe with a bug: it panics on item 7. It is a named
// function so that the test can look for its name in the stack trace.
func crashOnSeven(_ context.Context, i int) (int, error) {
	if i == 7 {
		panic("probe crashed on item 7")
	}
	return i, nil
}

// probeConfig is the per-target configuration in the runtime error test.
type probeConfig struct{ Port int }

func TestMapRecoversPanics(t *testing.T) {
	t.Run("value and stack", func(t *testing.T) {
		defer catchPanic(t)
		out, err := runMap(t, context.Background(), ints(20), 3, crashOnSeven)
		var pe *workerpool.PanicError
		if !errors.As(err, &pe) {
			t.Fatalf("Map error = %v (%T), want a *workerpool.PanicError", err, err)
		}
		if pe.Value != "probe crashed on item 7" {
			t.Errorf("PanicError.Value = %#v, want %q", pe.Value, "probe crashed on item 7")
		}
		if !bytes.Contains(pe.Stack, []byte("crashOnSeven")) {
			t.Errorf("PanicError.Stack does not mention crashOnSeven, the function that panicked. "+
				"Capture it with debug.Stack() inside the deferred function that recovers. Stack:\n%s", pe.Stack)
		}
		if out != nil {
			t.Errorf("Map result = %v, want nil when Map fails", out)
		}
	})

	t.Run("runtime error", func(t *testing.T) {
		defer catchPanic(t) // errors.As calls your Unwrap
		// A classic bug: no config for some targets, so a nil pointer is dereferenced.
		configs := map[int]*probeConfig{0: {Port: 443}}
		fn := func(_ context.Context, i int) (int, error) {
			return configs[i].Port, nil
		}
		_, err := runMap(t, context.Background(), ints(5), 2, fn)
		var pe *workerpool.PanicError
		if !errors.As(err, &pe) {
			t.Fatalf("Map error = %v (%T), want a *workerpool.PanicError", err, err)
		}
		var re runtime.Error
		if !errors.As(err, &re) {
			t.Errorf("errors.As(err, &runtimeErr) = false for a nil pointer dereference in fn; PanicError.Unwrap must return Value when it is an error")
		}
	})
}

// ----- Map: context -----

func TestMapContextEnds(t *testing.T) {
	alreadyDone := []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{"already cancelled: fn is never called", func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, context.Canceled},
		{"deadline already passed: fn is never called", func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Minute))
		}, context.DeadlineExceeded},
	}
	for _, tc := range alreadyDone {
		t.Run(tc.name, func(t *testing.T) {
			// Repeated, because select picks a random case when several are
			// ready: a Map that relies on select alone to notice the done ctx
			// gets this right by luck most of the time.
			for attempt := range 20 {
				ctx, cancel := tc.ctx()
				var calls atomic.Int32
				fn := func(_ context.Context, i int) (int, error) {
					calls.Add(1)
					return i, nil
				}
				out, err := runMap(t, ctx, ints(100), 4, fn)
				cancel()
				if !errors.Is(err, tc.want) {
					t.Fatalf("attempt %d: Map error = %v, want %v", attempt, err, tc.want)
				}
				if out != nil {
					t.Fatalf("attempt %d: Map result = %v, want nil", attempt, out)
				}
				if got := calls.Load(); got != 0 {
					t.Fatalf("attempt %d: fn was called %d times although ctx was already done, want 0", attempt, got)
				}
			}
		})
	}

	t.Run("cancelled while calls run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var started atomic.Int32
		fn := func(ctx context.Context, _ int) (int, error) {
			started.Add(1)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(timeout):
				return 0, errors.New("the ctx passed to fn was not cancelled after the caller's ctx was")
			}
		}
		ch := startMap(ctx, ints(10), 2, fn)
		waitWhileMapRuns(t, ch, "2 calls of fn are running", func() bool { return started.Load() == 2 })
		cancel()
		out, err := await(t, ch)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Map error = %v, want context.Canceled", err)
		}
		if out != nil {
			t.Errorf("Map result = %v, want nil", out)
		}
		if got := started.Load(); got != 2 {
			t.Errorf("fn was called %d times, want 2: no new call may start after ctx was cancelled", got)
		}
	})

	t.Run("cancelled while calls run, and fn ignores ctx", func(t *testing.T) {
		// A sloppy fn that never looks at ctx and always succeeds. Map must
		// still notice that items are missing.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls atomic.Int32
		fn := func(_ context.Context, i int) (int, error) {
			if calls.Add(1) == 2 {
				cancel() // for example, the operator pressed Ctrl-C during the 2nd call
			}
			return i, nil
		}
		out, err := runMap(t, ctx, ints(5), 1, fn)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Map error = %v, want context.Canceled: only 2 of 5 items were processed, "+
				"and returning a full slice would hide 3 missing results behind zero values", err)
		}
		if out != nil {
			t.Errorf("Map result = %v, want nil", out)
		}
		if got := calls.Load(); got != 2 {
			t.Errorf("fn was called %d times, want 2: no new call may start after ctx was cancelled", got)
		}
	})

	t.Run("a call failed before ctx ended: the call's error wins", func(t *testing.T) {
		// A probe fails, and while Map waits for the slow calls to wind down,
		// the operator presses Ctrl-C (or the deadline passes). The report
		// must still name the real failure, not "context canceled".
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		release := make(chan struct{})
		openRelease := sync.OnceFunc(func() { close(release) })
		defer openRelease()

		sawFailure := make(chan struct{})
		var started atomic.Int32
		fn := func(ctx context.Context, _ int) (int, error) {
			switch started.Add(1) {
			case 1: // the slow call
				select {
				case <-ctx.Done():
					close(sawFailure)
				case <-time.After(timeout):
					return 0, errors.New("the ctx passed to fn was not cancelled after another call failed")
				}
				select {
				case <-release:
				case <-time.After(timeout):
				}
				return 0, ctx.Err()
			case 2:
				return 0, errProbe
			default: // the other tests check that no call starts after a failure
				return 0, nil
			}
		}
		ch := startMap(ctx, ints(10), 2, fn)
		waitWhileMapRuns(t, ch, "the slow call sees its ctx cancelled after the 2nd call failed", func() bool {
			select {
			case <-sawFailure:
				return true
			default:
				return false
			}
		})
		cancel() // the caller's ctx ends while Map waits for the slow call
		openRelease()
		out, err := await(t, ch)
		if !errors.Is(err, errProbe) {
			t.Errorf("Map error = %v, want %v: the first error of a failed call wins over a ctx that ended later", err, errProbe)
		}
		if out != nil {
			t.Errorf("Map result = %v, want nil", out)
		}
	})
}

func TestMapFnContext(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	parent, cancel := context.WithDeadline(context.WithValue(context.Background(), traceIDKey{}, "trace-42"), deadline)
	defer cancel()

	var (
		mu       sync.Mutex
		seen     []context.Context
		problems []string
	)
	fn := func(ctx context.Context, i int) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, ctx)
		if got := ctx.Value(traceIDKey{}); got != "trace-42" {
			problems = append(problems, fmt.Sprintf("item %d: ctx.Value(traceIDKey{}) = %v, want %q: derive fn's ctx from the ctx passed to Map", i, got, "trace-42"))
		}
		if got, ok := ctx.Deadline(); !ok || !got.Equal(deadline) {
			problems = append(problems, fmt.Sprintf("item %d: ctx.Deadline() = %v, %v, want %v, true: derive fn's ctx from the ctx passed to Map", i, got, ok, deadline))
		}
		if err := ctx.Err(); err != nil {
			problems = append(problems, fmt.Sprintf("item %d: ctx.Err() = %v during the call, want nil: nothing has failed", i, err))
		}
		return i, nil
	}
	if _, err := runMap(t, parent, ints(10), 3, fn); err != nil {
		t.Fatalf("Map error = %v, want nil", err)
	}
	for _, p := range problems {
		t.Error(p)
	}
	for _, ctx := range seen {
		if ctx.Err() == nil {
			t.Error("the ctx passed to fn is not cancelled after Map returned; cancel it, so anything fn started with it stops too")
			break
		}
	}
}

// ----- Map: no leaks -----

func TestMapNoGoroutineLeak(t *testing.T) {
	check := func(t *testing.T, ctx context.Context, fn func(context.Context, int) (int, error)) {
		t.Helper()
		noLeak := expectNoLeak(t)
		_, _ = runMap(t, ctx, ints(100), 4, fn) // the other tests check the results
		noLeak()
	}

	t.Run("all calls succeed", func(t *testing.T) {
		check(t, context.Background(), func(_ context.Context, i int) (int, error) { return i, nil })
	})
	t.Run("the first item fails", func(t *testing.T) {
		check(t, context.Background(), func(_ context.Context, i int) (int, error) {
			if i == 0 {
				return 0, errProbe
			}
			return i, nil
		})
	})
	t.Run("an item panics", func(t *testing.T) {
		check(t, context.Background(), crashOnSeven)
	})
	t.Run("ctx already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		check(t, ctx, func(_ context.Context, i int) (int, error) { return i, nil })
	})
	t.Run("ctx cancelled while calls run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		check(t, ctx, func(_ context.Context, i int) (int, error) {
			if i == 10 {
				cancel()
			}
			return i, nil
		})
	})
}

// ----- Example -----

func ExampleMap() {
	services := []string{"api", "db", "cache"}
	addrs, err := workerpool.Map(context.Background(), services, 2,
		func(_ context.Context, name string) (string, error) {
			return name + ".prod.svc.cluster.local:443", nil
		})
	fmt.Println(addrs, err)
	// Output: [api.prod.svc.cluster.local:443 db.prod.svc.cluster.local:443 cache.prod.svc.cluster.local:443] <nil>
}
