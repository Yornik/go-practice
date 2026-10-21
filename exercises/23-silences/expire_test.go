package silences_test

import (
	"testing"
	"time"
)

// Step 3: Expire and Len. The tests in this step do not check onExpire.

func TestExpireBoundary(t *testing.T) {
	h := newHarness(t)
	disk2 := sil("disk-2", "disk_full", "chkpt-store-2", time.Hour)
	h.mustAdd(disk2)
	h.clock.Advance(time.Hour - time.Nanosecond)
	h.wantExpire() // 1 ns before Until: still active
	h.wantLen(1)
	h.clock.Advance(time.Nanosecond)
	h.wantExpire(disk2) // at Until: ended
	h.wantLen(0)
	h.wantExpire() // already removed
}

func TestExpireBoundarySortedByUntilThenID(t *testing.T) {
	late := sil("a-late", "disk_full", "chkpt-store-3", 3*time.Hour)
	active := sil("b-active", "", "vl-gpu-0412", 5*time.Hour)
	early := sil("z-early", "high_latency", "chkpt-store-1", time.Hour)
	disk1 := sil("disk-1", "disk_full", "chkpt-store-1", 2*time.Hour)
	disk2 := sil("disk-2", "disk_full", "chkpt-store-2", 2*time.Hour)
	disk3 := sil("disk-3", "disk_full", "chkpt-store-3", 2*time.Hour)
	disk4 := sil("disk-4", "disk_full", "chkpt-store-4", 2*time.Hour)
	node := sil("node-0412", "node_offline", "vl-gpu-0412", 2*time.Hour)
	upgrade := sil("upgrade", "", "chkpt-store-2", 2*time.Hour)
	for run := range runs {
		h := newHarness(t)
		h.mustAdd(late, disk3, upgrade, active, disk1, node, early, disk4, disk2)
		h.clock.Advance(4 * time.Hour)
		h.wantExpire(early, disk1, disk2, disk3, disk4, node, upgrade, late)
		h.wantLen(1) // b-active is still active
		if t.Failed() {
			t.Fatalf("failed on fresh store %d of %d", run+1, runs)
		}
	}
}

func TestExpireBoundaryLenCountsEndedSilences(t *testing.T) {
	h := newHarness(t)
	h.mustAdd(
		sil("disk-1", "disk_full", "chkpt-store-1", time.Hour),
		sil("disk-2", "disk_full", "chkpt-store-2", time.Hour),
		sil("disk-3", "disk_full", "chkpt-store-3", 3*time.Hour),
	)
	h.clock.Advance(2 * time.Hour)
	h.wantLen(3) // 2 have ended, but Expire has not removed them
	h.wantNoMatch("disk_full", "chkpt-store-1")
	h.wantLen(3) // Match removes nothing
	h.wantExpire(
		sil("disk-1", "disk_full", "chkpt-store-1", time.Hour),
		sil("disk-2", "disk_full", "chkpt-store-2", time.Hour),
	)
	h.wantLen(1)
}

func TestExpireBoundaryNothingEnded(t *testing.T) {
	h := newHarness(t)
	h.wantExpire() // empty store
	h.mustAdd(sil("disk-2", "disk_full", "chkpt-store-2", time.Hour))
	h.clock.Advance(59 * time.Minute)
	h.wantExpire()
	h.wantLen(1)
}

func TestExpireBoundaryReplacedSilenceIsGone(t *testing.T) {
	t.Run("replaced before it ended", func(t *testing.T) {
		h := newHarness(t)
		h.mustAdd(sil("disk-2", "disk_full", "chkpt-store-2", time.Hour))
		extended := sil("disk-2", "disk_full", "chkpt-store-2", 3*time.Hour)
		h.mustAdd(extended)
		h.clock.Advance(2 * time.Hour)
		h.wantExpire() // the old Until has passed, but the old silence is gone
		h.clock.Advance(time.Hour)
		h.wantExpire(extended)
	})
	t.Run("replaced after it ended, before Expire", func(t *testing.T) {
		h := newHarness(t)
		h.mustAdd(sil("disk-2", "disk_full", "chkpt-store-2", time.Hour))
		h.clock.Advance(2 * time.Hour)
		h.mustAdd(sil("disk-2", "disk_full", "chkpt-store-2", 3*time.Hour))
		h.wantExpire()
		h.wantLen(1)
	})
}
