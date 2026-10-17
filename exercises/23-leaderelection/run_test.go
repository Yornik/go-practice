package leaderelection_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	leaderelection "github.com/yornik/go-practice/exercises/23-leaderelection"
)

// ----- Config.Validate -----

// nopStore is a LeaseStore that does nothing, for configs that are only
// validated.
type nopStore struct{}

func (nopStore) Get(context.Context) (Lease, error)               { return Lease{}, nil }
func (nopStore) Update(_ context.Context, l Lease) (Lease, error) { return l, nil }

func TestConfigValidate(t *testing.T) {
	valid := func() leaderelection.Config {
		// Now, OnStoppedLeading and OnNewLeader are optional, so they are nil.
		return leaderelection.Config{
			ID:               "controller-7d4b9-x2x5q",
			Store:            nopStore{},
			LeaseDuration:    15 * time.Second,
			RenewDeadline:    10 * time.Second,
			RetryPeriod:      2 * time.Second,
			OnStartedLeading: func(context.Context, uint64) {},
		}
	}
	tests := []struct {
		name   string
		change func(*leaderelection.Config)
		want   []string // field names the error must contain; none: valid
	}{
		{"valid", func(*leaderelection.Config) {}, nil},
		{"empty ID", func(c *leaderelection.Config) { c.ID = "" }, []string{"ID"}},
		{"nil Store", func(c *leaderelection.Config) { c.Store = nil }, []string{"Store"}},
		{"nil OnStartedLeading", func(c *leaderelection.Config) { c.OnStartedLeading = nil }, []string{"OnStartedLeading"}},
		{"RetryPeriod 0", func(c *leaderelection.Config) { c.RetryPeriod = 0 }, []string{"RetryPeriod"}},
		{"negative RetryPeriod", func(c *leaderelection.Config) { c.RetryPeriod = -time.Second }, []string{"RetryPeriod"}},
		{"RenewDeadline equal to RetryPeriod", func(c *leaderelection.Config) { c.RenewDeadline = 2 * time.Second }, []string{"RenewDeadline"}},
		{"RenewDeadline below RetryPeriod", func(c *leaderelection.Config) { c.RenewDeadline = time.Second }, []string{"RenewDeadline"}},
		{"LeaseDuration equal to RenewDeadline", func(c *leaderelection.Config) { c.LeaseDuration = 10 * time.Second }, []string{"LeaseDuration"}},
		{"LeaseDuration below RenewDeadline", func(c *leaderelection.Config) { c.LeaseDuration = 5 * time.Second }, []string{"LeaseDuration"}},
		{"two problems: both are named", func(c *leaderelection.Config) {
			c.ID = ""
			c.LeaseDuration = 5 * time.Second
		}, []string{"ID", "LeaseDuration"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.change(&c)
			err := c.Validate()
			if len(tc.want) == 0 {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error that names %v", tc.want)
			}
			for _, field := range tc.want {
				if !strings.Contains(err.Error(), field) {
					t.Errorf("Validate() = %q, want the message to name %s", err, field)
				}
			}
		})
	}
}

// ----- Run with one candidate -----
//
// These tests use the fake clock: no lease expires and no deadline passes
// unless the test calls Advance. Candidates retry every 5 ms of real time.

func TestRunInvalidConfig(t *testing.T) {
	cl := newCluster(t)
	a := cl.add("a")
	a.run(func(c *leaderelection.Config) { c.LeaseDuration = c.RenewDeadline })
	err := a.result(t, "the config is invalid")
	if err == nil || !strings.Contains(err.Error(), "LeaseDuration") {
		t.Errorf("Run with LeaseDuration == RenewDeadline = %v, want the error of Validate", err)
	}
	if n := a.store.calls.Load(); n != 0 {
		t.Errorf("Run called the store %d times with an invalid config, want 0", n)
	}
	if a.started() || a.stopCount() != 0 || len(a.reported()) != 0 {
		t.Errorf("Run called callbacks with an invalid config, want none")
	}
}

func TestRunAcquiresAtOnce(t *testing.T) {
	cl := newCluster(t)
	// The candidate retries only once an hour, so it must make its first
	// attempt at once, not after one RetryPeriod.
	a := cl.start("a", slowRetries)
	cl.eventually("a acquires the empty lease at once and starts leading", a.started)

	want := Lease{Holder: "a", Epoch: 1, AcquireTime: t0, RenewTime: t0, Duration: 3 * time.Hour, Version: 1}
	if got := cl.lease(); !sameLease(got, want) {
		t.Errorf("lease in the store = %s, want %s", show(got), show(want))
	}
	if got := a.startedEpochs(); !slices.Equal(got, []uint64{1}) {
		t.Errorf("OnStartedLeading epochs = %v, want [1]", got)
	}
	if got := a.reported(); !slices.Equal(got, []string{"a"}) {
		t.Errorf("OnNewLeader calls = %q, want [a]: a candidate that acquires reports itself", got)
	}
	if err := a.ctx().Err(); err != nil {
		t.Errorf("the ctx passed to OnStartedLeading is done (%v) while a leads, want it live", err)
	}
}

func TestRunRenews(t *testing.T) {
	cl := newCluster(t)
	a := cl.start("a")
	cl.eventually("a starts leading", a.started)
	// 12 s in steps of 1 s: longer than RenewDeadline (5 s) and LeaseDuration
	// (10 s). A leader that renews in time keeps the lease forever.
	for range 12 {
		cl.clock.Advance(time.Second)
		now := cl.clock.Now()
		cl.eventually(fmt.Sprintf("a renews the lease with RenewTime %s (the time of Config.Now)", hms(now)), func() bool {
			return cl.lease().RenewTime.Equal(now)
		})
	}
	got := cl.lease()
	want := Lease{Holder: "a", Epoch: 1, AcquireTime: t0, RenewTime: at(12), Duration: leaseDuration, Version: got.Version}
	if !sameLease(got, want) {
		t.Errorf("lease after 12 s of renews = %s, want %s (any Version): a renew changes only RenewTime", show(got), show(want))
	}
	if a.stopCount() != 0 || a.exited() {
		t.Errorf("a stopped leading although every renew succeeded")
	}
}

func TestRunRetriesStoreErrors(t *testing.T) {
	cl := newCluster(t)
	a := cl.add("a")
	a.store.failNext.Store(3) // the API server is still starting
	a.run()
	cl.eventually("a starts leading after its first 3 store calls failed", func() bool { return a.started() || a.exited() })
	if a.exited() {
		err := a.result(t, "")
		t.Fatalf("Run returned %v after a store error, want it to try again every RetryPeriod", err)
	}
}

func TestRunWaitsForExpiry(t *testing.T) {
	cl := newCluster(t)
	// b holds the lease and promised to keep it for 20 s. b then dies.
	cl.put(Lease{Holder: "b", Epoch: 2, AcquireTime: t0, RenewTime: t0, Duration: 20 * time.Second})
	a := cl.start("a")
	cl.eventually("a reports b as the leader", func() bool { return slices.Equal(a.reported(), []string{"b"}) })

	cl.clock.Advance(19 * time.Second)
	cl.stays("a does not lead while b's lease is valid (b's Duration is 20s; a's LeaseDuration of 10s does not matter)",
		func() bool { return !a.started() })

	cl.clock.Advance(time.Second)
	cl.eventually("a acquires the lease 20s after b's last renew", a.started)
	got := cl.lease()
	want := Lease{Holder: "a", Epoch: 3, AcquireTime: at(20), RenewTime: at(20), Duration: leaseDuration, Version: got.Version}
	if !sameLease(got, want) {
		t.Errorf("lease = %s, want %s (any Version)", show(got), show(want))
	}
	if got := a.startedEpochs(); !slices.Equal(got, []uint64{3}) {
		t.Errorf("OnStartedLeading epochs = %v, want [3]", got)
	}
	if got := a.reported(); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("OnNewLeader calls = %q, want [b a]", got)
	}
}

func TestRunReportsNewLeaders(t *testing.T) {
	cl := newCluster(t)
	cl.put(Lease{Holder: "b", Epoch: 2, AcquireTime: t0, RenewTime: t0, Duration: leaseDuration})
	a := cl.start("a")
	cl.eventually("a reports b as the leader", func() bool { return slices.Equal(a.reported(), []string{"b"}) })
	cl.stays("OnNewLeader is called once per new holder, not once per attempt: calls = [b]",
		func() bool { return slices.Equal(a.reported(), []string{"b"}) })

	// c took over from b.
	cl.put(Lease{Holder: "c", Epoch: 3, AcquireTime: t0, RenewTime: t0, Duration: leaseDuration})
	cl.eventually("a reports c as the new leader: calls = [b c]", func() bool {
		return slices.Equal(a.reported(), []string{"b", "c"})
	})

	// c released the lease. Nobody holds it, so a acquires it at once,
	// without waiting for it to expire (the clock does not move).
	cl.put(Lease{Holder: "", Epoch: 3, AcquireTime: t0, RenewTime: t0, Duration: leaseDuration})
	cl.eventually("a acquires the released lease", a.started)
	if got := a.startedEpochs(); !slices.Equal(got, []uint64{4}) {
		t.Errorf("OnStartedLeading epochs = %v, want [4]", got)
	}
	if got := a.reported(); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Errorf("OnNewLeader calls = %q, want [b c a]: the empty holder of a released lease is never reported", got)
	}
}

func TestRunStepsDownAfterRenewDeadline(t *testing.T) {
	cl := newCluster(t)
	a := cl.start("a")
	cl.eventually("a starts leading", a.started)

	a.store.set(failing) // a's node loses its network
	cl.clock.Advance(4 * time.Second)
	cl.stays("a keeps leading and retrying while its renews fail for less than RenewDeadline (5s)",
		func() bool { return a.stopCount() == 0 && !a.exited() })

	cl.clock.Advance(2 * time.Second) // 6 s since the last renew
	err := a.result(t, "6s passed since its last renew, more than RenewDeadline (5s)")
	if !errors.Is(err, leaderelection.ErrLeaseLost) {
		t.Errorf("Run = %v, want ErrLeaseLost", err)
	}
	a.wantLeaderCtxDone(t)
	if n := a.stopCount(); n != 1 {
		t.Errorf("OnStoppedLeading was called %d times, want 1", n)
	}
}

func TestRunStepsDownWhenAnotherHolds(t *testing.T) {
	cl := newCluster(t)
	a := cl.start("a")
	cl.eventually("a starts leading", a.started)

	// Another process took the lease (for example an operator who edited
	// the Lease object). The clock does not move: no deadline passes, so
	// only the next renew attempt can tell a.
	written := cl.put(Lease{Holder: "b", Epoch: 2, AcquireTime: t0, RenewTime: t0, Duration: leaseDuration})
	err := a.result(t, "its next renew finds b as the holder")
	if !errors.Is(err, leaderelection.ErrLeaseLost) {
		t.Errorf("Run = %v, want ErrLeaseLost", err)
	}
	a.wantLeaderCtxDone(t)
	if n := a.stopCount(); n != 1 {
		t.Errorf("OnStoppedLeading was called %d times, want 1", n)
	}
	if got := a.reported(); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("OnNewLeader calls = %q, want [a b]", got)
	}
	if got := cl.lease(); !sameLease(got, written) {
		t.Errorf("lease = %s, want b's lease %s unchanged: a must not write after it lost the lease", show(got), show(written))
	}
}

func TestRunHangingStore(t *testing.T) {
	// A real partition rarely fails fast: packets are dropped and the call
	// just never returns. The leader must still stop at its deadline, so no
	// renew attempt may run past it.
	cl := newCluster(t)
	a := cl.start("a", func(c *leaderelection.Config) {
		c.LeaseDuration = time.Minute
		c.RenewDeadline = 30 * time.Second
	})
	cl.eventually("a starts leading", a.started)

	// a's renews fail until only 10 ms are left until its deadline.
	a.store.set(failing)
	cl.clock.Advance(30*time.Second - 10*time.Millisecond)
	n := a.store.calls.Load()
	cl.eventually("a keeps trying to renew while its renews fail", func() bool { return a.store.calls.Load() >= n+2 })

	// Now its calls hang. Once a call hangs, the clock passes the deadline.
	a.store.set(hanging)
	n = a.store.calls.Load()
	cl.eventually("a tries to renew again", func() bool { return a.store.calls.Load() > n })
	cl.clock.Advance(time.Second)
	err := a.result(t, "its deadline passed while its store calls hang. No renew attempt may run past the deadline: "+
		"10ms were left when the calls started to hang, and RenewDeadline (30s) is far too long as a timeout")
	if !errors.Is(err, leaderelection.ErrLeaseLost) {
		t.Errorf("Run = %v, want ErrLeaseLost", err)
	}
	a.wantLeaderCtxDone(t)
}

func TestRunCancel(t *testing.T) {
	t.Run("leader without ReleaseOnCancel", func(t *testing.T) {
		cl := newCluster(t)
		a := cl.start("a")
		cl.eventually("a starts leading", a.started)
		a.cancel()
		if err := a.result(t, "its ctx was cancelled"); !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, want context.Canceled", err)
		}
		a.wantLeaderCtxDone(t)
		if n := a.stopCount(); n != 1 {
			t.Errorf("OnStoppedLeading was called %d times, want 1", n)
		}
		if got := cl.lease(); got.Holder != "a" || got.Epoch != 1 {
			t.Errorf("lease = %s, want it still held by a with epoch 1: without ReleaseOnCancel it stays until it expires", show(got))
		}
	})

	t.Run("leader with ReleaseOnCancel", func(t *testing.T) {
		cl := newCluster(t)
		a := cl.start("a", releaseOnCancel)
		cl.eventually("a starts leading", a.started)
		a.cancel()
		if err := a.result(t, "its ctx was cancelled"); !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, want context.Canceled", err)
		}
		a.wantLeaderCtxDone(t)
		if got := cl.lease(); got.Holder != "" || got.Epoch != 1 {
			t.Errorf("lease = %s, want it released: Holder \"\", Epoch 1. (ctx is done when Run releases, and MemoryStore refuses calls with a done ctx.)", show(got))
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.stops != 1 || a.holderAtStop != "a" {
			t.Errorf("OnStoppedLeading calls = %d, store Holder during the call = %q; want 1 call while the store still showed a: release the lease after OnStoppedLeading",
				a.stops, a.holderAtStop)
		}
	})

	t.Run("candidate", func(t *testing.T) {
		cl := newCluster(t)
		written := cl.put(Lease{Holder: "b", Epoch: 2, AcquireTime: t0, RenewTime: t0, Duration: leaseDuration})
		a := cl.start("a", releaseOnCancel)
		cl.eventually("a reports b as the leader", func() bool { return slices.Equal(a.reported(), []string{"b"}) })
		a.cancel()
		if err := a.result(t, "its ctx was cancelled"); !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, want context.Canceled", err)
		}
		if a.started() || a.stopCount() != 0 {
			t.Errorf("a never led, but OnStartedLeading or OnStoppedLeading was called")
		}
		if got := cl.lease(); !sameLease(got, written) {
			t.Errorf("lease = %s, want b's lease %s unchanged", show(got), show(written))
		}
	})
}

func TestRunReleaseKeepsOthersLease(t *testing.T) {
	cl := newCluster(t)
	// a retries once an hour: after it acquired the lease, it makes no more
	// attempts in this test, so it cannot notice that b took over.
	a := cl.start("a", releaseOnCancel, slowRetries)
	cl.eventually("a starts leading", a.started)
	written := cl.put(Lease{Holder: "b", Epoch: 2, AcquireTime: t0, RenewTime: t0, Duration: leaseDuration})

	a.cancel()
	if err := a.result(t, "its ctx was cancelled"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run = %v, want context.Canceled", err)
	}
	if got := cl.lease(); !sameLease(got, written) {
		t.Errorf("lease = %s, want b's lease %s unchanged: release only a lease that is still yours", show(got), show(written))
	}
}
