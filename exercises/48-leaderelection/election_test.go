package leaderelection_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	leaderelection "github.com/yornik/go-practice/exercises/48-leaderelection"
)

// ----- Several candidates on one store -----
//
// Every test here checks at the end that leadership never overlapped: no
// candidate's OnStartedLeading was called while another candidate's
// OnStartedLeading was still running (except in the hanging-store test,
// which uses the real clock, and the fencing test, which is about exactly
// that case).

// otherOf returns the candidate of pair that is not c.
func otherOf(c *candidate, pair ...*candidate) *candidate {
	if pair[0] == c {
		return pair[1]
	}
	return pair[0]
}

func TestElectionOneLeader(t *testing.T) {
	cl := newCluster(t)
	cands := []*candidate{cl.add("a"), cl.add("b"), cl.add("c")}
	for _, c := range cands {
		c.run() // all three race for the empty store
	}
	leader := cl.leader("one of a, b and c starts leading", cands...)
	cl.stays("exactly one candidate leads (the clock does not move, so no lease expires)", func() bool {
		n := 0
		for _, c := range cands {
			if c.started() {
				n++
			}
		}
		return n == 1
	})
	if got := cl.lease(); got.Holder != leader.id || got.Epoch != 1 {
		t.Errorf("lease = %s, want Holder %s, Epoch 1", show(got), leader.id)
	}
	for _, c := range cands {
		cl.eventually(c.id+" reports "+leader.id+" as the leader, once", func() bool {
			return slices.Equal(c.reported(), []string{leader.id})
		})
	}
}

func TestElectionTakeoverAfterRelease(t *testing.T) {
	cl := newCluster(t)
	a := cl.start("a", releaseOnCancel)
	cl.eventually("a starts leading", a.started)
	b, c := cl.start("b", releaseOnCancel), cl.start("c", releaseOnCancel)
	cl.eventually("b and c report a as the leader", func() bool {
		return slices.Equal(b.reported(), []string{"a"}) && slices.Equal(c.reported(), []string{"a"})
	})

	// A rolling update stops a. It releases the lease, and b or c takes over
	// at once. The clock does not move: only the release makes this possible.
	a.cancel()
	if err := a.result(t, "its ctx was cancelled"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run(a) = %v, want context.Canceled", err)
	}
	x := cl.leader("b or c starts leading after a released the lease", b, c)
	y := otherOf(x, b, c)
	cl.eventually(y.id+" reports "+x.id+" as the new leader", func() bool {
		return slices.Equal(y.reported(), []string{"a", x.id})
	})

	// Then x is stopped too, and y takes over.
	x.cancel()
	if err := x.result(t, "its ctx was cancelled"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run(%s) = %v, want context.Canceled", x.id, err)
	}
	cl.eventually(y.id+" starts leading after "+x.id+" released the lease", y.started)

	if got := cl.ledger.startedEpochs(); !slices.Equal(got, []uint64{1, 2, 3}) {
		t.Errorf("epochs in the order leadership started = %v, want [1 2 3]", got)
	}
	if got, want := y.reported(), []string{"a", x.id, y.id}; !slices.Equal(got, want) {
		t.Errorf("OnNewLeader calls of %s = %q, want %q", y.id, got, want)
	}
}

func TestElectionTakeoverAfterPartition(t *testing.T) {
	cl := newCluster(t)
	a := cl.start("a")
	cl.eventually("a starts leading", a.started)
	b, c := cl.start("b"), cl.start("c")
	cl.eventually("b and c report a as the leader", func() bool {
		return slices.Equal(b.reported(), []string{"a"}) && slices.Equal(c.reported(), []string{"a"})
	})

	// a's node loses its network. a cannot renew, so it must stop leading
	// after RenewDeadline (5 s), before its lease expires (10 s).
	a.store.set(failing)
	cl.clock.Advance(6 * time.Second)
	if err := a.result(t, "it could not renew for 6s (RenewDeadline 5s)"); !errors.Is(err, leaderelection.ErrLeaseLost) {
		t.Errorf("Run(a) = %v, want ErrLeaseLost", err)
	}
	cl.stays("b and c wait: a's lease is valid until 10s after a's last renew", func() bool {
		return !b.started() && !c.started()
	})
	cl.clock.Advance(5 * time.Second) // 11 s since a's last renew
	x := cl.leader("b or c starts leading 11s after a's last renew (LeaseDuration 10s)", b, c)
	y := otherOf(x, b, c)
	if got := x.startedEpochs(); !slices.Equal(got, []uint64{2}) {
		t.Errorf("%s: OnStartedLeading epochs = %v, want [2]", x.id, got)
	}

	// The same happens to x.
	x.store.set(failing)
	cl.clock.Advance(6 * time.Second)
	if err := x.result(t, "it could not renew for 6s (RenewDeadline 5s)"); !errors.Is(err, leaderelection.ErrLeaseLost) {
		t.Errorf("Run(%s) = %v, want ErrLeaseLost", x.id, err)
	}
	cl.stays(y.id+" waits: "+x.id+"'s lease is valid until 10s after its last renew", func() bool { return !y.started() })
	cl.clock.Advance(5 * time.Second)
	cl.eventually(y.id+" starts leading", y.started)

	if got := cl.ledger.startedEpochs(); !slices.Equal(got, []uint64{1, 2, 3}) {
		t.Errorf("epochs in the order leadership started = %v, want [1 2 3]", got)
	}
	if got := cl.lease(); got.Holder != y.id || got.Epoch != 3 {
		t.Errorf("lease = %s, want Holder %s, Epoch 3", show(got), y.id)
	}
}

func TestElectionHangingStore(t *testing.T) {
	// A real partition rarely fails fast: packets are dropped and the call
	// just never returns. This test uses the real clock and short durations.
	cl := newRealCluster(t)
	short := func(c *leaderelection.Config) {
		c.LeaseDuration = 500 * time.Millisecond
		c.RenewDeadline = 250 * time.Millisecond
		c.RetryPeriod = 25 * time.Millisecond
	}
	a := cl.start("a", short)
	cl.eventually("a starts leading", a.started)
	b := cl.start("b", short)

	a.store.set(hanging)
	err := a.result(t, "its store calls hang, and a leader must stop when RenewDeadline has passed: a hanging call must not keep it waiting")
	if !errors.Is(err, leaderelection.ErrLeaseLost) {
		t.Errorf("Run(a) = %v, want ErrLeaseLost", err)
	}
	a.wantLeaderCtxDone(t)
	cl.eventually("b starts leading with epoch 2 after a's lease expired", func() bool {
		return slices.Equal(b.startedEpochs(), []uint64{2})
	})
}

func TestElectionFencing(t *testing.T) {
	// The story from Kleppmann's "How to do distributed locking": the leader
	// freezes (a long GC pause, a VM migration, CPU throttling) right before
	// a write. While it is frozen, its lease expires and another candidate
	// takes over. Then it wakes up and writes, still sure that it leads.
	// Only the resource can stop that write, with the fencing token.
	cl := newCluster(t)
	var deployment leaderelection.FencedResource
	pause := make(chan struct{})
	aWoke := make(chan error, 1) // the error of a's write after the pause

	a := cl.add("a")
	a.run(func(c *leaderelection.Config) {
		c.OnStartedLeading = func(ctx context.Context, epoch uint64) {
			a.markStarted(ctx, epoch)
			defer a.markReturned()
			_ = deployment.Write(epoch, "a: scale web to 3 replicas")
			// a freezes here and does not look at ctx.
			select {
			case <-pause:
			case <-time.After(timeout):
			}
			aWoke <- deployment.Write(epoch, "a: scale web to 4 replicas")
		}
	})
	cl.eventually("a leads and writes with token 1", func() bool { _, token := deployment.Read(); return token == 1 })

	b := cl.add("b")
	b.run(func(c *leaderelection.Config) {
		c.OnStartedLeading = func(ctx context.Context, epoch uint64) {
			b.markStarted(ctx, epoch)
			defer b.markReturned()
			_ = deployment.Write(epoch, "b: scale web to 5 replicas")
			<-ctx.Done()
		}
	})
	cl.eventually("b reports a as the leader", func() bool { return slices.Equal(b.reported(), []string{"a"}) })

	// a cannot renew (its node is partitioned, or it is frozen). Run steps
	// down on time and cancels a's ctx, but a's OnStartedLeading is frozen.
	a.store.set(failing)
	cl.clock.Advance(6 * time.Second)
	cl.eventually("a's leader ctx is cancelled 6s after its last renew", func() bool {
		ctx := a.ctx()
		return ctx != nil && ctx.Err() != nil
	})
	cl.stays("Run(a) waits for OnStartedLeading to return before it calls OnStoppedLeading and returns",
		func() bool { return a.stopCount() == 0 && !a.exited() })

	// a's lease expires; b takes over with epoch 2 and writes.
	cl.clock.Advance(5 * time.Second)
	cl.eventually("b leads and writes with token 2", func() bool { _, token := deployment.Read(); return token == 2 })

	// a wakes up and writes with its old token.
	close(pause)
	select {
	case err := <-aWoke:
		if !errors.Is(err, leaderelection.ErrStaleToken) {
			t.Errorf("a's write after its pause: Write(1, ...) = %v, want ErrStaleToken (b wrote with token 2)", err)
		}
	case <-time.After(timeout):
		t.Fatalf("a's OnStartedLeading did not write after the pause")
	}
	if v, token := deployment.Read(); v != "b: scale web to 5 replicas" || token != 2 {
		t.Errorf("deployment.Read() = %q, %d; want b's value and token 2", v, token)
	}
	if err := a.result(t, "its OnStartedLeading returned after it lost the lease"); !errors.Is(err, leaderelection.ErrLeaseLost) {
		t.Errorf("Run(a) = %v, want ErrLeaseLost", err)
	}
	if n := a.stopCount(); n != 1 {
		t.Errorf("OnStoppedLeading of a was called %d times, want 1", n)
	}
}
