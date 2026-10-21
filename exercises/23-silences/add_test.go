package silences_test

import (
	"errors"
	"testing"
	"time"

	silences "github.com/yornik/go-practice/exercises/23-silences"
)

// Step 1: New, Add and Len.

func TestAddValid(t *testing.T) {
	h := newHarness(t)
	h.wantLen(0)
	h.mustAdd(
		sil("disk-2", "disk_full", "photo-store-2", time.Hour), // alert and target
		sil("disks", "disk_full", "", 2*time.Hour),             // every target
		sil("feeder-0412", "", "feeder-0412", time.Minute),     // every alert
		sil("short", "disk_full", "photo-store-1", time.Nanosecond),
	)
	h.wantLen(4)
}

func TestAddReplacesSameID(t *testing.T) {
	h := newHarness(t)
	h.mustAdd(sil("disk-2", "disk_full", "photo-store-2", time.Hour))
	h.mustAdd(sil("disk-2", "disk_full", "photo-store-2", 2*time.Hour))
	h.wantLen(1)
	h.mustAdd(sil("disk-2", "feeder_offline", "", 30*time.Minute))
	h.wantLen(1)
	h.mustAdd(sil("disk-3", "disk_full", "photo-store-3", time.Hour))
	h.wantLen(2)
}

func TestAddReplacesEndedSilence(t *testing.T) {
	h := newHarness(t)
	h.mustAdd(sil("disk-2", "disk_full", "photo-store-2", time.Hour))
	h.clock.Advance(2 * time.Hour) // disk-2 has ended; Expire has not run
	h.mustAdd(sil("disk-2", "disk_full", "photo-store-2", 3*time.Hour))
	h.wantLen(1)
}

func TestAddInvalid(t *testing.T) {
	cases := []struct {
		name string
		sil  silences.Silence
	}{
		{"empty ID", sil("", "disk_full", "photo-store-2", time.Hour)},
		{"empty Alert and Target", sil("everything", "", "", time.Hour)},
		{"Until equals now", sil("disk-2", "disk_full", "photo-store-2", 0)},
		{"Until before now", sil("disk-2", "disk_full", "photo-store-2", -time.Minute)},
		{"Until is the zero time", silences.Silence{ID: "until-migration-done", Alert: "disk_full", Target: "photo-store-2"}},
		{"all 3 problems", sil("", "", "", -time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.addInvalid(tc.sil)
			h.wantLen(0)
		})
	}
}

func TestAddInvalidLeavesStoreUnchanged(t *testing.T) {
	h := newHarness(t)
	h.mustAdd(sil("disk-2", "disk_full", "photo-store-2", time.Hour))
	h.clock.Advance(30 * time.Minute)
	h.addInvalid(sil("disk-2", "disk_full", "photo-store-2", 10*time.Minute)) // ends before now
	h.addInvalid(sil("disk-2", "", "", 2*time.Hour))
	h.wantLen(1)
}

func TestAddNilClockMeansTimeNow(t *testing.T) {
	var s *silences.Store
	within(t, "New(nil, nil)", lockHang, func() { s = silences.New(nil, nil) })
	if s == nil {
		t.Fatal("New(nil, nil) = nil, want a store")
	}
	// With the real clock, the year 2000 is in the past and the year 9999
	// is in the future.
	past := silences.Silence{ID: "y2k", Alert: "disk_full", Until: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)}
	future := silences.Silence{ID: "y9999", Alert: "disk_full", Until: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)}
	var errPast, errFuture error
	var n int
	within(t, "Add, Add and Len on a store with the real clock", lockHang, func() {
		errPast = s.Add(past)
		errFuture = s.Add(future)
		n = s.Len()
	})
	if !errors.Is(errPast, silences.ErrInvalid) {
		t.Errorf("with now nil: Add(Until: 2000-01-01) = %v, want an error that wraps ErrInvalid", errPast)
	}
	if errFuture != nil {
		t.Errorf("with now nil: Add(Until: 9999-01-01) = %v, want nil", errFuture)
	}
	if n != 1 {
		t.Errorf("with now nil: Len() = %d after 1 valid Add, want 1", n)
	}
}
