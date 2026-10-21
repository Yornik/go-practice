package singleflight_test

import (
	"errors"
	"testing"
)

func TestChanReturnsAtOnce(t *testing.T) {
	t.Run("value", func(t *testing.T) {
		var g group
		db := newLoader(t, "profile of user 42")
		ch := doChan(t, &g, "user:42", db.fn) // fails if DoChan waits for fn
		db.waitStarted(t)
		select {
		case r := <-ch:
			t.Fatalf("the channel delivered %+v while fn was still running", r)
		default:
		}

		db.release()
		wantResult(t, "the DoChan Result", receive(t, "DoChan", ch), "profile of user 42", false)
	})

	t.Run("error and value are passed on as they are", func(t *testing.T) {
		var g group
		ch := doChan(t, &g, "user:42", func() (string, error) { return "partial", errBackend })
		r := receive(t, "DoChan", ch)
		if r.Val != "partial" || !errors.Is(r.Err, errBackend) || r.Shared {
			t.Errorf("the DoChan Result = %+v, want {Val:partial Err:%v Shared:false}", r, errBackend)
		}
	})
}

func TestChanCoalesces(t *testing.T) {
	t.Run("DoChan callers share one execution", func(t *testing.T) {
		// DoChan joins before it returns, so these calls are known to
		// overlap with the first execution.
		var g group
		db := newLoader(t, "profile of user 42")
		first := doChan(t, &g, "user:42", db.fn)
		db.waitStarted(t)
		late := newQuick("from a second execution")
		second := doChan(t, &g, "user:42", late.fn)
		third := doChan(t, &g, "user:42", late.fn)

		db.release()
		wantResult(t, "1st DoChan", receive(t, "1st DoChan", first), db.val, true)
		wantResult(t, "2nd DoChan", receive(t, "2nd DoChan", second), db.val, true)
		wantResult(t, "3rd DoChan", receive(t, "3rd DoChan", third), db.val, true)
		if got := db.calls.Load(); got != 1 {
			t.Errorf("the first caller's fn ran %d times, want 1", got)
		}
		if got := late.calls.Load(); got != 0 {
			t.Errorf("the fn of the callers that joined ran %d times, want 0", got)
		}
	})

	t.Run("Do and DoChan share one execution", func(t *testing.T) {
		var g group
		db := newLoader(t, "profile of user 42")
		first := startDo(&g, "user:42", db.fn)
		db.waitStartedBy(t, "Do", first)
		late := newQuick("from a second execution")
		ch := doChan(t, &g, "user:42", late.fn)

		db.release()
		want(t, "Do", await(t, "Do", first), db.val, true)
		wantResult(t, "DoChan", receive(t, "DoChan", ch), db.val, true)
		if got := late.calls.Load(); got != 0 {
			t.Errorf("the DoChan caller's fn ran %d times, want 0: it must join the Do in flight", got)
		}
	})
}

func TestChanAbandonedChannel(t *testing.T) {
	// A caller that gives up (its request timed out) stops reading its
	// channel. That must not block the delivery to the other callers, and no
	// goroutine may stay behind trying to send to it.
	checkLeaks := expectNoLeak(t)
	var g group
	db := newLoader(t, "profile of user 42")
	abandoned := doChan(t, &g, "user:42", db.fn)
	db.waitStarted(t)
	_ = abandoned // nobody ever reads this channel
	late := newQuick("from a second execution")
	reader := doChan(t, &g, "user:42", late.fn)

	db.release()
	wantResult(t, "the DoChan Result of the caller that still reads", receive(t, "DoChan", reader), db.val, true)
	checkLeaks()

	// The execution is over: the next call runs fn again.
	fresh := newQuick("fresh profile")
	wantResult(t, "DoChan after the execution finished", receive(t, "DoChan", doChan(t, &g, "user:42", fresh.fn)), "fresh profile", false)
}
