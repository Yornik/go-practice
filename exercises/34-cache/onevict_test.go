package cache_test

import (
	"cmp"
	"slices"
	"sync"
	"testing"
	"time"

	cache "github.com/yornik/go-practice/exercises/34-cache"
)

// Step 4: OnEvict. wantEvents checks the calls made since the last check,
// right after the method returned: OnEvict runs before the method returns.

func TestOnEvictReasons(t *testing.T) {
	t.Run("evicted", func(t *testing.T) {
		h := newCache(t, 2, 0)
		h.Set("a", 1)
		h.Set("b", 2)
		h.wantEvents()
		h.Set("c", 3)
		h.wantEvents(ev("a", 1, cache.Evicted))
	})
	t.Run("replaced passes the old value", func(t *testing.T) {
		h := newCache(t, 2, 0)
		h.Set("a", 1)
		h.Set("a", 2)
		h.wantEvents(ev("a", 1, cache.Replaced))
		h.SetWithTTL("a", 3, time.Minute)
		h.wantEvents(ev("a", 2, cache.Replaced))
		h.wantHit("a", 3)
		h.wantEvents()
	})
	t.Run("deleted", func(t *testing.T) {
		h := newCache(t, 2, 0)
		h.Set("a", 1)
		h.wantDelete("a", true)
		h.wantEvents(ev("a", 1, cache.Deleted))
		h.wantDelete("a", false)
		h.wantEvents() // nothing was removed
	})
	t.Run("expired, on Get", func(t *testing.T) {
		h := newCache(t, 2, 10*time.Second)
		h.Set("a", 1)
		h.Advance(10 * time.Second)
		h.wantMiss("a")
		h.wantEvents(ev("a", 1, cache.Expired))
		h.wantMiss("a")
		h.wantEvents() // already removed
	})
	t.Run("expired, on Delete", func(t *testing.T) {
		h := newCache(t, 2, 10*time.Second)
		h.Set("a", 1)
		h.Advance(10 * time.Second)
		h.wantDelete("a", false)
		h.wantEvents(ev("a", 1, cache.Expired))
	})
	t.Run("expired, on Set of the same key", func(t *testing.T) {
		// The old value expired before Set came: it is not "replaced".
		h := newCache(t, 2, 10*time.Second)
		h.Set("a", 1)
		h.Advance(10 * time.Second)
		h.Set("a", 2)
		h.wantEvents(ev("a", 1, cache.Expired))
	})
	t.Run("expired, on Set of the same key in a full cache", func(t *testing.T) {
		// Removing the expired "a" frees its slot, so the live "b" stays,
		// although the cache was full and "b" is the least recently used.
		h := newCache(t, 2, 10*time.Second)
		h.SetWithTTL("b", 2, 0)
		h.Set("a", 1)
		h.Advance(10 * time.Second)
		h.Set("a", 3)
		h.wantEvents(ev("a", 1, cache.Expired))
		h.wantKeys("a", "b")
	})
	t.Run("expired, when making room", func(t *testing.T) {
		// The least recently used entry had expired: no live data was
		// pushed out, so this is an expiry, not an eviction.
		h := newCache(t, 2, 10*time.Second)
		h.Set("a", 1)
		h.Advance(5 * time.Second)
		h.Set("b", 2)
		h.Advance(5 * time.Second) // "a" expired, "b" live
		h.Set("c", 3)
		h.wantEvents(ev("a", 1, cache.Expired))
	})
	t.Run("expired, negative TTL", func(t *testing.T) {
		h := newCache(t, 2, 0)
		h.SetWithTTL("a", 1, -time.Second)
		h.wantEvents() // stored; nothing removed yet
		h.wantMiss("a")
		h.wantEvents(ev("a", 1, cache.Expired))
	})
	t.Run("expired, on PurgeExpired", func(t *testing.T) {
		h := newCache(t, 10, 10*time.Second)
		h.Set("a", 1)
		h.Set("b", 2)
		h.SetWithTTL("c", 3, 0)
		h.Advance(10 * time.Second)
		h.PurgeExpired()
		// One call per removed entry. The order is not specified, so the
		// test sorts the calls by key.
		got := h.rec.Take()
		slices.SortFunc(got, func(x, y event[string, int]) int { return cmp.Compare(x.Key, y.Key) })
		want := []event[string, int]{ev("a", 1, cache.Expired), ev("b", 2, cache.Expired)}
		if !slices.Equal(got, want) {
			t.Errorf("after PurgeExpired(): OnEvict calls (sorted by key) = %v, want %v", got, want)
		}
	})
	t.Run("nothing removed, no call", func(t *testing.T) {
		h := newCache(t, 2, 10*time.Second)
		h.Set("a", 1)
		h.wantMiss("x")
		h.wantDelete("x", false)
		h.wantKeys("a")
		h.wantLen(1)
		h.PurgeExpired()
		h.Advance(10 * time.Second) // "a" expired
		h.wantKeys()
		h.wantLen(1)
		h.wantEvents() // Keys and Len never remove anything
	})
}

func TestOnEvictCanUseCache(t *testing.T) {
	// OnEvict calls Len, Keys and Get on the same cache. If the cache held
	// its lock while it calls OnEvict, this would deadlock, and the call
	// would not return within the timeout. It also shows what OnEvict sees:
	// the change is complete when it runs.
	type seen struct {
		Event event[string, int]
		Len   int
		Keys  []string
		Value int  // what Get(key) returned inside OnEvict
		Found bool // ok from that Get
	}
	var (
		mu  sync.Mutex
		got []seen
		c   *cache.Cache[string, int]
	)
	clock := newClock()
	within(t, "New", func() {
		c = cache.New(cache.Options[string, int]{
			Capacity: 2,
			TTL:      10 * time.Second,
			Now:      clock.Now,
			OnEvict: func(key string, value int, reason cache.Reason) {
				s := seen{Event: ev(key, value, reason), Len: c.Len(), Keys: c.Keys()}
				s.Value, s.Found = c.Get(key)
				mu.Lock()
				defer mu.Unlock()
				got = append(got, s)
			},
		})
	})
	check := func(call string, want seen) {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if len(got) != 1 {
			t.Fatalf("after %s: OnEvict was called %d times, want once", call, len(got))
		}
		g := got[0]
		got = nil
		if g.Event != want.Event || g.Len != want.Len || !slices.Equal(g.Keys, want.Keys) ||
			g.Value != want.Value || g.Found != want.Found {
			t.Errorf("after %s: inside OnEvict%v: Len() = %d, Keys() = %q, Get(%q) = %d, %v;\n"+
				"want Len() = %d, Keys() = %q, Get(%q) = %d, %v",
				call, g.Event, g.Len, g.Keys, g.Event.Key, g.Value, g.Found,
				want.Len, want.Keys, want.Event.Key, want.Value, want.Found)
		}
	}

	within(t, `Set("a", 1)`, func() { c.Set("a", 1) })
	within(t, `Set("b", 2)`, func() { c.Set("b", 2) })

	within(t, `Set("c", 3) with OnEvict calling the cache`, func() { c.Set("c", 3) })
	// When OnEvict runs, Set is complete: "a" is gone and "c" is stored.
	check(`Set("c", 3)`, seen{Event: ev("a", 1, cache.Evicted), Len: 2, Keys: []string{"c", "b"}})

	within(t, `Set("b", 20) with OnEvict calling the cache`, func() { c.Set("b", 20) })
	// "b" already holds the new value.
	check(`Set("b", 20)`, seen{Event: ev("b", 2, cache.Replaced), Len: 2, Keys: []string{"b", "c"}, Value: 20, Found: true})

	within(t, `Delete("c") with OnEvict calling the cache`, func() { c.Delete("c") })
	check(`Delete("c")`, seen{Event: ev("c", 3, cache.Deleted), Len: 1, Keys: []string{"b"}})

	clock.Advance(10 * time.Second)
	within(t, `Get("b") with OnEvict calling the cache`, func() { c.Get("b") })
	check(`Get("b") after the TTL`, seen{Event: ev("b", 20, cache.Expired), Len: 0, Keys: []string{}})

	within(t, `Set("d", 4)`, func() { c.Set("d", 4) })
	clock.Advance(10 * time.Second)
	within(t, `PurgeExpired() with OnEvict calling the cache`, func() { c.PurgeExpired() })
	check(`PurgeExpired()`, seen{Event: ev("d", 4, cache.Expired), Len: 0, Keys: []string{}})
}

func TestOnEvictSlowCallbackDoesNotBlockOthers(t *testing.T) {
	// OnEvict closes a connection and that takes a while. Meanwhile other
	// goroutines must be able to use the cache.
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock() // also when the test fails early

	var c *cache.Cache[string, int]
	within(t, "New", func() {
		c = cache.New(cache.Options[string, int]{
			Capacity: 10,
			OnEvict: func(key string, value int, reason cache.Reason) {
				if key == "slow" {
					close(entered)
					<-release
				}
			},
		})
	})
	within(t, `Set("slow", 1)`, func() { c.Set("slow", 1) })
	within(t, `Set("a", 2)`, func() { c.Set("a", 2) })

	deleted := async(func() { c.Delete("slow") })
	select {
	case <-entered:
	case v := <-deleted:
		t.Fatalf(`Delete("slow") returned (panic: %v) without calling OnEvict`, v)
	case <-time.After(timeout):
		t.Fatalf(`Delete("slow") did not call OnEvict within %v`, timeout)
	}

	// OnEvict for "slow" is blocked in another goroutine right now.
	within(t, `Get("a") while OnEvict runs in another goroutine`, func() { c.Get("a") })
	within(t, `Set("b", 3) while OnEvict runs in another goroutine`, func() { c.Set("b", 3) })
	within(t, "Len() while OnEvict runs in another goroutine", func() { c.Len() })
	within(t, "Keys() while OnEvict runs in another goroutine", func() { c.Keys() })

	unblock()
	select {
	case v := <-deleted:
		if v != nil {
			t.Fatalf(`Delete("slow") panicked: %v`, v)
		}
	case <-time.After(timeout):
		t.Fatalf(`Delete("slow") did not return within %v after OnEvict returned`, timeout)
	}
}
