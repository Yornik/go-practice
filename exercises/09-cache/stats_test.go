package cache_test

import (
	"testing"
	"time"

	cache "github.com/yornik/go-practice/exercises/09-cache"
)

// Step 5: Stats.

func TestStatsHitRatio(t *testing.T) {
	tests := []struct {
		s    cache.Stats
		want float64
	}{
		{cache.Stats{}, 0}, // no Get yet: 0, not NaN
		{cache.Stats{Hits: 3, Misses: 1}, 0.75},
		{cache.Stats{Misses: 5}, 0},
		{cache.Stats{Hits: 5, Evictions: 7, Expirations: 2}, 1},
	}
	for _, tt := range tests {
		var got float64
		within(t, "HitRatio", func() { got = tt.s.HitRatio() })
		if got != tt.want {
			t.Errorf("%+v.HitRatio() = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestStatsCounts(t *testing.T) {
	// One story, with the expected counters after each step.
	h := newCache(t, 2, 10*time.Second)
	want := func(step string, w cache.Stats) {
		t.Helper()
		if got := h.Stats(); got != w {
			t.Errorf("after %s: Stats() = %+v, want %+v", step, got, w)
		}
	}
	want("New", cache.Stats{})

	h.wantMiss("a")
	want(`Get("a") on an empty cache`, cache.Stats{Misses: 1})

	h.Set("a", 1)
	h.wantHit("a", 1)
	want(`Set("a", 1), Get("a")`, cache.Stats{Hits: 1, Misses: 1})

	h.Set("b", 2)
	h.Set("c", 3) // evicts "a"
	want(`Set("b", 2), Set("c", 3)`, cache.Stats{Hits: 1, Misses: 1, Evictions: 1})

	h.wantMiss("a")
	want(`Get("a") after it was evicted`, cache.Stats{Hits: 1, Misses: 2, Evictions: 1})

	h.Set("b", 20)          // replaced: not counted
	h.wantDelete("c", true) // deleted: not counted
	h.Keys()
	h.Len()
	want(`Set("b", 20), Delete("c"), Keys(), Len()`, cache.Stats{Hits: 1, Misses: 2, Evictions: 1})

	h.Advance(10 * time.Second)
	h.wantMiss("b") // expired: a miss and an expiration
	want(`Get("b") after its TTL`, cache.Stats{Hits: 1, Misses: 3, Evictions: 1, Expirations: 1})

	h.Set("d", 4)
	h.Advance(5 * time.Second)
	h.Set("e", 5)
	h.Advance(5 * time.Second) // "d" expired
	h.Set("f", 6)              // full: the expired "d" makes room
	want(`Set("f", 6) with the expired "d" as least recently used`,
		cache.Stats{Hits: 1, Misses: 3, Evictions: 1, Expirations: 2})

	h.Advance(5 * time.Second) // "e" expired
	h.PurgeExpired()
	want("PurgeExpired()", cache.Stats{Hits: 1, Misses: 3, Evictions: 1, Expirations: 3})

	h.SetWithTTL("g", 7, -time.Second)
	h.wantDelete("g", false)
	want(`Delete("g") of an expired entry`, cache.Stats{Hits: 1, Misses: 3, Evictions: 1, Expirations: 4})
}
