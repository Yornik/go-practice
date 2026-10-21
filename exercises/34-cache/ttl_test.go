package cache_test

import (
	"testing"
	"time"
)

// Step 3: time to live. The clock is fake: it moves only on Advance.

func TestTTLExpiresAtDeadline(t *testing.T) {
	// An entry written at t with TTL 10s is live while now < t+10s.
	tests := []struct {
		after time.Duration
		live  bool
	}{
		{0, true},
		{5 * time.Second, true},
		{10*time.Second - time.Nanosecond, true},
		{10 * time.Second, false}, // the deadline itself is already expired
		{time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.after.String(), func(t *testing.T) {
			h := newCache(t, 10, 10*time.Second)
			h.Set("a", 1)
			h.Advance(tt.after)
			if tt.live {
				h.wantHit("a", 1)
			} else {
				h.wantMiss("a")
			}
		})
	}
}

func TestTTLZeroNeverExpires(t *testing.T) {
	const century = 100 * 365 * 24 * time.Hour
	t.Run("Options.TTL 0", func(t *testing.T) {
		h := newCache(t, 10, 0)
		h.Set("a", 1)
		h.Advance(century)
		h.wantHit("a", 1)
	})
	t.Run("SetWithTTL 0", func(t *testing.T) {
		h := newCache(t, 10, 10*time.Second)
		h.SetWithTTL("a", 1, 0)
		h.Advance(century)
		h.wantHit("a", 1)
	})
}

func TestTTLGetDoesNotExtend(t *testing.T) {
	h := newCache(t, 10, 10*time.Second)
	h.Set("a", 1)
	h.Advance(6 * time.Second)
	h.wantHit("a", 1)
	h.Advance(4 * time.Second) // 10s after Set
	h.wantMiss("a")
}

func TestTTLSetStartsNewTTL(t *testing.T) {
	h := newCache(t, 10, 10*time.Second)
	h.Set("a", 1)
	h.Advance(8 * time.Second)
	h.Set("a", 2) // live until 18s
	h.Advance(7 * time.Second)
	h.wantHit("a", 2) // 15s
	h.Advance(3 * time.Second)
	h.wantMiss("a") // 18s
}

func TestTTLSetWithTTL(t *testing.T) {
	h := newCache(t, 10, 10*time.Second)
	h.SetWithTTL("long", 1, 30*time.Second)
	h.Set("short", 2) // Options.TTL: 10s
	h.Advance(10 * time.Second)
	h.wantMiss("short")
	h.wantHit("long", 1)
	h.Advance(20 * time.Second) // 30s
	h.wantMiss("long")
}

func TestTTLNegativeIsAlreadyExpired(t *testing.T) {
	// A TTL computed from a deadline that has passed, for example
	// token.Expiry.Sub(now), must not mean "keep forever".
	h := newCache(t, 10, 0)
	h.SetWithTTL("token", 1, -time.Second)
	h.wantLen(1) // stored...
	h.wantKeys() // ...but not live
	h.wantMiss("token")
	h.wantLen(0)
}

func TestTTLExpiredEntriesStayUntilTouched(t *testing.T) {
	// Lazy expiry: no timer removes an entry when its TTL passes. Len counts
	// it until an operation removes it; Keys skips it but does not remove it.
	h := newCache(t, 10, 10*time.Second)
	h.Set("a", 1)
	h.Advance(5 * time.Second)
	h.Set("b", 2)
	h.Advance(5 * time.Second) // "a" expired, "b" is live

	h.wantLen(2)
	h.wantKeys("b")
	h.wantLen(2)
	h.wantMiss("a") // Get removes it
	h.wantLen(1)
}

func TestTTLDeleteExpired(t *testing.T) {
	h := newCache(t, 10, 10*time.Second)
	h.Set("a", 1)
	h.Advance(10 * time.Second)
	h.wantDelete("a", false) // it was not live
	h.wantLen(0)             // but it is removed
}

func TestTTLSetOnExpiredKey(t *testing.T) {
	h := newCache(t, 10, 10*time.Second)
	h.Set("a", 1)
	h.Advance(10 * time.Second)
	h.Set("a", 2) // a new entry with a new TTL: live until 20s
	h.wantLen(1)
	h.Advance(9 * time.Second)
	h.wantHit("a", 2)
	h.Advance(time.Second)
	h.wantMiss("a")
}

func TestTTLExpiredEntriesCountTowardsCapacity(t *testing.T) {
	h := newCache(t, 2, 10*time.Second)
	h.Set("a", 1)
	h.Advance(5 * time.Second)
	h.Set("b", 2)
	h.Advance(7 * time.Second) // 12s: "a" expired, "b" live until 15s
	h.wantLen(2)

	h.Set("c", 3) // full: the least recently used entry, "a", makes room
	h.wantLen(2)
	h.wantKeys("c", "b")
}

func TestTTLPurgeExpired(t *testing.T) {
	h := newCache(t, 10, 10*time.Second)
	h.Set("a", 1)
	h.Advance(5 * time.Second)
	h.Set("b", 2)
	h.SetWithTTL("c", 3, 0)
	h.wantHit("a", 1)          // order of use: a c b, but "a" expires first
	h.Advance(7 * time.Second) // 12s: only "a" expired

	if n := h.PurgeExpired(); n != 1 {
		t.Errorf("PurgeExpired() at 12s = %d, want 1", n)
	}
	h.wantLen(2)
	h.wantKeys("c", "b")

	h.Advance(3 * time.Second) // 15s: "b" expired too
	if n := h.PurgeExpired(); n != 1 {
		t.Errorf("PurgeExpired() at 15s = %d, want 1", n)
	}
	if n := h.PurgeExpired(); n != 0 {
		t.Errorf("PurgeExpired() again = %d, want 0", n)
	}
	h.wantLen(1)
	h.wantKeys("c")
}
