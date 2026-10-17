package singleflight_test

import "testing"

func TestForgetStartsNewExecution(t *testing.T) {
	// An operator just fixed the bad config row: the load in flight read the
	// old value. Forget lets the next call start a fresh load at once.
	var g group
	old := newLoader(t, "old profile")
	oldCh := doChan(t, &g, "user:42", old.fn)
	old.waitStarted(t)

	forget(t, &g, "user:42")

	// The next call starts a new execution, although the old one still runs.
	cur := newLoader(t, "new profile")
	curCh := doChan(t, &g, "user:42", cur.fn)
	cur.waitStarted(t)

	// A call after that joins the new execution.
	late := newQuick("from a third execution")
	joinCh := doChan(t, &g, "user:42", late.fn)

	// The old execution finishes. Its caller gets its result. The old
	// execution must not take the new one out of the group when it ends.
	old.release()
	wantResult(t, "the caller of the forgotten execution", receive(t, "DoChan", oldCh), "old profile", false)
	afterCh := doChan(t, &g, "user:42", late.fn)

	cur.release()
	wantResult(t, "the caller that started the new execution", receive(t, "DoChan", curCh), "new profile", true)
	wantResult(t, "a caller that joined the new execution", receive(t, "DoChan", joinCh), "new profile", true)
	wantResult(t, "a caller that joined after the old execution finished", receive(t, "DoChan", afterCh), "new profile", true)
	// Every channel has delivered, so every fn that ran has returned.
	if got := late.calls.Load(); got != 0 {
		t.Errorf("%d calls ran their own fn while the new execution was in flight; they must join it. "+
			"When the forgotten execution finished, did it remove the new one from the group?", got)
	}
	if got := old.calls.Load() + cur.calls.Load(); got != 2 {
		t.Errorf("fn ran %d times in total, want 2: one old and one new execution", got)
	}
}

func TestForgetWithoutExecution(t *testing.T) {
	var g group
	forget(t, &g, "user:42") // never used: nothing to forget
	q := newQuick("profile of user 42")
	want(t, `Do("user:42")`, do(t, &g, "user:42", q.fn), "profile of user 42", false)
	forget(t, &g, "user:42") // finished: nothing to forget
	want(t, `Do("user:42") again`, do(t, &g, "user:42", q.fn), "profile of user 42", false)
	if got := q.calls.Load(); got != 2 {
		t.Errorf("fn was called %d times, want 2", got)
	}
}
