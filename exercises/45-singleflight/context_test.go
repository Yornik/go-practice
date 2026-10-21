package singleflight_test

import (
	"context"
	"errors"
	"testing"
	"time"
)

// traceKey is a context key, like the one a tracing library uses to carry
// the trace ID of a request.
type traceKey struct{}

// wantLeft checks that a DoContext caller left with err: zero value, err,
// shared = false.
func wantLeft(t *testing.T, what string, o outcome, err error) {
	t.Helper()
	if o.panicked != nil {
		t.Errorf("%s panicked: %v", what, o.panicked)
		return
	}
	if o.val != "" || !errors.Is(o.err, err) || o.shared {
		t.Errorf("%s = %q, %v, %v; want \"\", %v, false", what, o.val, o.err, o.shared, err)
	}
}

func TestContextReturnsFnResult(t *testing.T) {
	var g group
	ctx := context.WithValue(context.Background(), traceKey{}, "trace-7f3a")
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()

	var (
		fnCtx           context.Context
		traceID         any
		hasDeadline     bool
		errWhileRunning error
	)
	fn := func(ctx context.Context) (string, error) {
		fnCtx = ctx
		traceID = ctx.Value(traceKey{})
		_, hasDeadline = ctx.Deadline()
		errWhileRunning = ctx.Err()
		return "profile of user 42", nil
	}
	o := await(t, "DoContext", startDoContext(ctx, &g, "user:42", fn))
	want(t, `DoContext("user:42")`, o, "profile of user 42", false)
	if fnCtx == nil {
		t.Fatal("fn was not called")
	}
	if traceID != "trace-7f3a" {
		t.Errorf("fn's ctx.Value(traceKey{}) = %v, want %q: fn's ctx must keep the values of the caller's ctx", traceID, "trace-7f3a")
	}
	if hasDeadline {
		t.Errorf("fn's ctx has a deadline, want none: the deadline of the caller that came first must not end the work that other callers share")
	}
	if errWhileRunning != nil {
		t.Errorf("fn's ctx.Err() = %v while fn ran, want nil", errWhileRunning)
	}
	waitFor(t, "fn's ctx is cancelled after fn returned", func() bool { return fnCtx.Err() != nil })
}

func TestContextAlreadyDone(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancel2 := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel2()

	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"cancelled", cancelled, context.Canceled},
		{"deadline passed", expired, context.DeadlineExceeded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var g group
			db := newLoader(t, "profile of user 42")
			o := await(t, "DoContext", startDoContext(tc.ctx, &g, "user:42", db.fnCtx))
			wantLeft(t, "DoContext with a ctx that is already done", o, tc.want)
			// A wrong solution may start fn in a goroutine of its own, after
			// DoContext has returned, so watch for a moment.
			select {
			case <-db.started:
				t.Errorf("fn was called, want no call: the request is already over and nobody waits for the result")
			case <-time.After(window):
			}
		})
	}
}

func TestContextCallerLeaves(t *testing.T) {
	checkLeaks := expectNoLeak(t)
	var g group
	db := newLoader(t, "old profile")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := startDoContext(ctx, &g, "user:42", db.fnCtx)
	fnCtx := db.waitStartedBy(t, "DoContext", out)

	// The client hangs up. fn ignores its ctx and keeps running, but the
	// caller must return at once.
	cancel()
	wantLeft(t, "DoContext after its ctx was cancelled", await(t, "DoContext after its ctx was cancelled", out), context.Canceled)
	waitFor(t, "fn's ctx is cancelled because its only caller left", func() bool { return fnCtx.Err() != nil })

	// Nobody waits for the old execution any more, so it left the group: the
	// next call starts a new execution at once, while the old fn still runs.
	fresh := newQuick("fresh profile")
	o := await(t, "DoContext after the only caller left", startDoContext(context.Background(), &g, "user:42", fresh.fnCtx))
	want(t, "DoContext after the only caller left", o, "fresh profile", false)

	// When the old fn finally returns, its result is dropped, and it must not
	// take a newer execution out of the group.
	cur := newLoader(t, "new profile")
	curOut := startDoContext(context.Background(), &g, "user:42", cur.fnCtx)
	cur.waitStartedBy(t, "DoContext", curOut)
	db.release()
	waitFor(t, "the old fn has returned", func() bool { return db.returned.Load() == 1 })
	time.Sleep(window) // let the group clean up after the old execution
	late := newQuick("from a fourth execution")
	lateCh := doChan(t, &g, "user:42", late.fn)
	cur.release()
	want(t, "DoContext of the newest execution", await(t, "DoContext", curOut), "new profile", true)
	wantResult(t, "DoChan joined to the newest execution", receive(t, "DoChan", lateCh), "new profile", true)
	// The channel has delivered, so if late.fn ran, it has returned.
	if got := late.calls.Load(); got != 0 {
		t.Errorf("a DoChan caller ran its own fn while the newest execution was in flight; it must join it. " +
			"When the abandoned execution finished, did it remove the newest one from the group?")
	}
	checkLeaks()
}

func TestContextDeadline(t *testing.T) {
	// The usual way a request stops waiting: its deadline passes while fn
	// still runs. The caller gets ctx.Err(), which is DeadlineExceeded here,
	// not Canceled.
	var g group
	db := newLoader(t, "profile of user 42")
	ctx, cancel := context.WithTimeout(context.Background(), 2*window)
	defer cancel()
	o := await(t, "DoContext with a short deadline", startDoContext(ctx, &g, "user:42", db.fnCtx))
	wantLeft(t, "DoContext after its deadline passed", o, context.DeadlineExceeded)
}

func TestContextLeaveAfterForget(t *testing.T) {
	// Forget does not stop the old execution: its caller still waits for it,
	// so fn's ctx stays alive. After Forget, a newer execution owns the key.
	// When the last caller of the forgotten execution leaves, fn's ctx is
	// cancelled, and the newer execution must stay in the group.
	var g group
	old := newLoader(t, "old profile")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := startDoContext(ctx, &g, "user:42", old.fnCtx)
	oldCtx := old.waitStartedBy(t, "DoContext", out)
	forget(t, &g, "user:42")
	if doneWithin(oldCtx, window) {
		t.Errorf("fn's ctx was cancelled by Forget, although a caller still waited for that execution; " +
			"Forget only makes the next call start a new execution")
	}
	cur := newLoader(t, "new profile")
	curCh := doChan(t, &g, "user:42", cur.fn)
	cur.waitStarted(t)

	cancel() // the only caller of the forgotten execution leaves
	wantLeft(t, "DoContext after its ctx was cancelled", await(t, "DoContext after its ctx was cancelled", out), context.Canceled)
	waitFor(t, "fn's ctx of the forgotten execution is cancelled because its only caller left",
		func() bool { return oldCtx.Err() != nil })
	late := newQuick("from a third execution")
	lateCh := doChan(t, &g, "user:42", late.fn)
	cur.release()
	wantResult(t, "DoChan that started the newer execution", receive(t, "DoChan", curCh), "new profile", true)
	wantResult(t, "DoChan that joined the newer execution", receive(t, "DoChan", lateCh), "new profile", true)
	// The channel has delivered, so if late.fn ran, it has returned.
	if got := late.calls.Load(); got != 0 {
		t.Errorf("a DoChan caller ran its own fn while the newer execution was in flight; it must join it. " +
			"When the last caller of the forgotten execution left, did it remove the newer one from the group?")
	}
}

func TestContextRefCount(t *testing.T) {
	t.Run("a caller that leaves does not stop the work for the others", func(t *testing.T) {
		untilJoined(t, func(window time.Duration) int {
			var g group
			db := newLoader(t, "profile of user 42")
			ctxA, cancelA := context.WithCancel(context.Background())
			defer cancelA()
			outA := startDoContext(ctxA, &g, "user:42", db.fnCtx)
			fnCtx := db.waitStartedBy(t, "DoContext", outA)

			own := newQuick("from a second execution")
			outB := startDoContext(context.Background(), &g, "user:42", own.fnCtx)
			time.Sleep(window) // give caller B time to join

			cancelA() // caller A, which started the execution, leaves
			wantLeft(t, "DoContext A", await(t, "DoContext A after its ctx was cancelled", outA), context.Canceled)
			cancelledEarly := doneWithin(fnCtx, window)

			db.release()
			oB := await(t, "DoContext B", outB)
			if oB.panicked == nil && oB.val == own.val {
				return 0 // B arrived after A had left and rightly started a new execution
			}
			if cancelledEarly {
				t.Errorf("fn's ctx was cancelled when caller A left, although caller B still waited for the result; " +
					"cancel it only when every caller has left")
			}
			want(t, "DoContext B, which stayed", oB, db.val, true)
			return 1
		})
	})

	t.Run("the last caller to leave cancels fn's ctx", func(t *testing.T) {
		// Whether fn's ctx survives A leaving is the subtest above. This one
		// checks that it is cancelled once B has left too, and that nothing
		// stays behind.
		untilJoined(t, func(window time.Duration) int {
			checkLeaks := expectNoLeak(t)
			var g group
			db := newLoader(t, "profile of user 42")
			ctxA, cancelA := context.WithCancel(context.Background())
			defer cancelA()
			ctxB, cancelB := context.WithCancel(context.Background())
			defer cancelB()
			outA := startDoContext(ctxA, &g, "user:42", db.fnCtx)
			fnCtx := db.waitStartedBy(t, "DoContext", outA)
			outB := startDoContext(ctxB, &g, "user:42", db.fnCtx)
			time.Sleep(window) // give caller B time to join

			cancelA()
			wantLeft(t, "DoContext A", await(t, "DoContext A after its ctx was cancelled", outA), context.Canceled)
			cancelB()
			wantLeft(t, "DoContext B", await(t, "DoContext B after its ctx was cancelled", outB), context.Canceled)
			waitFor(t, "fn's ctx is cancelled after both callers left", func() bool { return fnCtx.Err() != nil })

			db.release()
			checkLeaks() // now every fn has returned, so db.calls is final
			if db.calls.Load() != 1 {
				return 0 // B arrived after A had left and rightly started a new execution
			}
			return 1
		})
	})

	t.Run("a DoChan caller never leaves", func(t *testing.T) {
		var g group
		db := newLoader(t, "profile of user 42")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		out := startDoContext(ctx, &g, "user:42", db.fnCtx)
		fnCtx := db.waitStartedBy(t, "DoContext", out)
		late := newQuick("from a second execution")
		ch := doChan(t, &g, "user:42", late.fn) // joins before it returns

		cancel()
		wantLeft(t, "DoContext", await(t, "DoContext after its ctx was cancelled", out), context.Canceled)
		if doneWithin(fnCtx, window) {
			t.Errorf("fn's ctx was cancelled when the DoContext caller left, although a DoChan caller still " +
				"waited; Do and DoChan callers count as callers that never leave")
		}
		db.release()
		wantResult(t, "DoChan", receive(t, "DoChan", ch), db.val, true)
		if got := late.calls.Load(); got != 0 {
			t.Errorf("the DoChan caller's fn ran %d times, want 0", got)
		}
	})
}

func TestContextPanic(t *testing.T) {
	t.Run("waiting callers panic with a *PanicError", func(t *testing.T) {
		var g group
		db := newLoader(t, "")
		db.panicValue = crash
		out := startDoContext(context.Background(), &g, "user:42", db.fnCtx)
		db.waitStartedBy(t, "DoContext", out)
		db.release()
		pe := wantPanicError(t, "DoContext", await(t, "DoContext", out), crash)
		wantStackOfLoader(t, pe)

		fresh := newQuick("profile of user 42")
		want(t, "DoContext after the panic", await(t, "DoContext", startDoContext(context.Background(), &g, "user:42", fresh.fnCtx)), "profile of user 42", false)
	})

	t.Run("a panic after every caller left is dropped", func(t *testing.T) {
		// If the panic were not recovered, it would kill the test binary.
		checkLeaks := expectNoLeak(t)
		var g group
		db := newLoader(t, "")
		db.panicValue = crash
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		out := startDoContext(ctx, &g, "user:42", db.fnCtx)
		db.waitStartedBy(t, "DoContext", out)
		cancel()
		wantLeft(t, "DoContext after its ctx was cancelled", await(t, "DoContext", out), context.Canceled)

		db.release()
		waitFor(t, "the abandoned fn has returned", func() bool { return db.returned.Load() == 1 })
		checkLeaks()
		fresh := newQuick("profile of user 42")
		want(t, "DoContext after the panic", await(t, "DoContext", startDoContext(context.Background(), &g, "user:42", fresh.fnCtx)), "profile of user 42", false)
	})
}
