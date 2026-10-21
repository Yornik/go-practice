package cache_test

import (
	"testing"
	"time"

	cache "github.com/yornik/go-practice/exercises/34-cache"
)

// Step 1: New, and the cache as a plain key/value store: Set, Get, Delete,
// Len. Every cache here is large enough that nothing is evicted, and has no
// TTL.

func TestNewPanicsOnInvalidOptions(t *testing.T) {
	// First the valid minimum: one entry, no TTL, no clock, no callback.
	var c *cache.Cache[string, int]
	within(t, "New(Options{Capacity: 1})", func() {
		c = cache.New(cache.Options[string, int]{Capacity: 1})
	})
	if c == nil {
		t.Fatal("New(Options{Capacity: 1}) = nil, want a cache")
	}

	// A cache that cannot hold anything, or a negative default TTL, is a
	// configuration bug. New panics, so it shows up at startup.
	tests := []struct {
		name string
		o    cache.Options[string, int]
	}{
		{"Capacity 0", cache.Options[string, int]{Capacity: 0}},
		{"Capacity -1", cache.Options[string, int]{Capacity: -1}},
		{"TTL -1s", cache.Options[string, int]{Capacity: 10, TTL: -time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mustPanic(t, "New(Options{"+tt.name+"})", func() { cache.New(tt.o) })
		})
	}
}

func TestNewDefaults(t *testing.T) {
	// Now and OnEvict are nil: the cache uses time.Now and calls no
	// callback. An hour does not pass during this test.
	var c *cache.Cache[string, int]
	within(t, "New(Options{Capacity: 10})", func() {
		c = cache.New(cache.Options[string, int]{Capacity: 10})
	})
	within(t, `Set("a", 1)`, func() { c.Set("a", 1) })
	within(t, `SetWithTTL("b", 2, time.Hour)`, func() { c.SetWithTTL("b", 2, time.Hour) })
	for key, want := range map[string]int{"a": 1, "b": 2} {
		var got int
		var ok bool
		within(t, "Get", func() { got, ok = c.Get(key) })
		if !ok || got != want {
			t.Errorf("Get(%q) = %d, %v; want %d, true", key, got, ok, want)
		}
	}
	within(t, `Delete("a") without OnEvict`, func() { c.Delete("a") })
}

func TestBasicSetGet(t *testing.T) {
	h := newCache(t, 10, 0)
	h.wantLen(0)
	h.wantMiss("a")

	h.Set("a", 1)
	h.Set("b", 2)
	h.wantHit("a", 1)
	h.wantHit("b", 2)
	h.wantMiss("c")
	h.wantLen(2)
}

func TestBasicSetReplacesValue(t *testing.T) {
	h := newCache(t, 10, 0)
	h.Set("a", 1)
	h.Set("a", 2)
	h.wantHit("a", 2)
	h.wantLen(1) // one entry per key
}

func TestBasicDelete(t *testing.T) {
	h := newCache(t, 10, 0)
	h.Set("a", 1)
	h.Set("b", 2)

	h.wantDelete("a", true)
	h.wantMiss("a")
	h.wantLen(1)
	h.wantDelete("a", false) // already gone
	h.wantDelete("never-set", false)
	h.wantHit("b", 2)
}

type config struct{ Endpoint string }

// routeKey is a struct key: any comparable type can be a key.
type routeKey struct{ Method, Path string }

func TestBasicGenericTypes(t *testing.T) {
	// A miss returns the zero value of V, whatever V is.
	t.Run("pointer values", func(t *testing.T) {
		h := newHarness(t, cache.Options[string, *config]{Capacity: 10})
		cfg := &config{Endpoint: "https://config.internal"}
		h.Set("tenant-a", cfg)
		h.wantHit("tenant-a", cfg)
		h.wantMiss("tenant-b") // nil, false
	})
	t.Run("int keys, string values", func(t *testing.T) {
		h := newHarness(t, cache.Options[int, string]{Capacity: 10})
		h.Set(404, "not found")
		h.wantHit(404, "not found")
		h.wantMiss(500) // "", false
	})
	t.Run("struct keys", func(t *testing.T) {
		h := newHarness(t, cache.Options[routeKey, float64]{Capacity: 10})
		h.Set(routeKey{Method: "GET", Path: "/healthz"}, 0.25)
		h.wantHit(routeKey{Method: "GET", Path: "/healthz"}, 0.25)
		h.wantMiss(routeKey{Method: "POST", Path: "/healthz"}) // 0, false
	})
}
