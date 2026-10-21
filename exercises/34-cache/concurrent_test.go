package cache_test

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cache "github.com/yornik/go-practice/exercises/34-cache"
)

// Step 6: many goroutines at once, and a large cache. Run with -race.

// written is a value in TestConcurrentEveryValueIsAccountedFor. ID is unique
// for every Set, so the test can follow each value.
type written struct {
	Key string
	ID  int64
}

func TestConcurrentEveryValueIsAccountedFor(t *testing.T) {
	// 8 goroutines call Set, SetWithTTL, Get, Delete, PurgeExpired, Keys and
	// Len on 256 keys, with a capacity of 64, while the clock moves. Then
	// the test checks the bookkeeping: every value ever written is either
	// still in the cache or was passed to OnEvict exactly once, and the
	// counters match the OnEvict calls.
	const (
		capacity   = 64
		keys       = 256
		goroutines = 8
		callsEach  = 3000
	)
	var (
		problems atomic.Int32 // stop reporting after a few
		finished atomic.Bool  // no t.Errorf after the test has returned
		mu       sync.Mutex
		gone     = map[int64]cache.Reason{} // value ID -> reason it was passed to OnEvict with
		all      = map[int64]string{}       // value ID -> key, every value written
		nextID   atomic.Int64
		gets     atomic.Uint64
		deletes  atomic.Uint64 // Delete calls that returned true
		c        *cache.Cache[string, written]
	)
	report := func(format string, args ...any) {
		if !finished.Load() && problems.Add(1) <= 5 {
			t.Errorf(format, args...)
		}
	}
	defer finished.Store(true)
	clock := newClock()
	within(t, "New", func() {
		c = cache.New(cache.Options[string, written]{
			Capacity: capacity,
			TTL:      50 * time.Millisecond,
			Now:      clock.Now,
			OnEvict: func(key string, v written, reason cache.Reason) {
				if v.Key != key {
					report("OnEvict(%q, %+v, %v): the value belongs to key %q", key, v, reason, v.Key)
				}
				if n := c.Len(); n > capacity {
					report("Len() = %d inside OnEvict, want at most %d", n, capacity)
				}
				mu.Lock()
				defer mu.Unlock()
				if earlier, dup := gone[v.ID]; dup {
					report("OnEvict called twice for value %+v: %v, then %v", v, earlier, reason)
				}
				gone[v.ID] = reason
			},
		})
	})

	var wg sync.WaitGroup
	panics := make(chan any, goroutines)
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					panics <- v
				}
			}()
			rng := rand.New(rand.NewPCG(uint64(g), 1))
			mine := map[int64]string{}
			defer func() { // record what this goroutine wrote, also after a panic
				mu.Lock()
				defer mu.Unlock()
				for id, key := range mine {
					all[id] = key
				}
			}()
			for range callsEach {
				key := fmt.Sprintf("k%03d", rng.IntN(keys))
				switch op := rng.IntN(100); {
				case op < 35:
					v := written{Key: key, ID: nextID.Add(1)}
					mine[v.ID] = key
					c.Set(key, v)
				case op < 40:
					v := written{Key: key, ID: nextID.Add(1)}
					mine[v.ID] = key
					c.SetWithTTL(key, v, []time.Duration{0, -time.Millisecond, 20 * time.Millisecond}[rng.IntN(3)])
				case op < 80:
					gets.Add(1)
					if v, ok := c.Get(key); ok && v.Key != key {
						report("Get(%q) = %+v: a value of another key", key, v)
					}
				case op < 90:
					if c.Delete(key) {
						deletes.Add(1)
					}
				case op < 93:
					c.PurgeExpired()
				case op < 96:
					if ks := c.Keys(); len(ks) > capacity {
						report("Keys() returned %d keys, want at most %d", len(ks), capacity)
					}
				default:
					if g == 0 {
						clock.Advance(10 * time.Millisecond)
					}
				}
				if n := c.Len(); n > capacity {
					report("Len() = %d, want at most %d, also while other goroutines call Set", n, capacity)
				}
			}
		}()
	}
	done := async(wg.Wait)
	select {
	case <-done:
	case <-time.After(2 * timeout):
		t.Fatalf("%d goroutines doing %d calls each did not finish within %v (deadlock?)", goroutines, callsEach, 2*timeout)
	}
	close(panics)
	for v := range panics {
		t.Fatalf("a call panicked: %v", v)
	}
	if t.Failed() {
		return
	}

	// Collect what is still in the cache.
	within(t, "PurgeExpired()", func() { c.PurgeExpired() })
	var live []string
	within(t, "Keys()", func() { live = c.Keys() })
	inCache := map[int64]bool{}
	for _, key := range live {
		var v written
		var ok bool
		within(t, "Get", func() { v, ok = c.Get(key) })
		gets.Add(1)
		if !ok || v.Key != key {
			t.Fatalf("Get(%q) = %+v, %v after Keys() listed it; want its value, true", key, v, ok)
		}
		inCache[v.ID] = true
	}
	var n int
	within(t, "Len()", func() { n = c.Len() })
	if n != len(live) {
		t.Errorf("after PurgeExpired: Len() = %d, len(Keys()) = %d; want them equal", n, len(live))
	}

	// Every value is accounted for exactly once.
	lost := 0
	for id, key := range all {
		_, evicted := gone[id]
		switch {
		case evicted && inCache[id]:
			t.Errorf("value %d of key %q is still in the cache, but OnEvict got it with reason %v", id, key, gone[id])
		case !evicted && !inCache[id]:
			lost++
		}
	}
	if lost > 0 {
		t.Errorf("%d of %d values written were lost: not in the cache, and never passed to OnEvict", lost, len(all))
	}
	for id, reason := range gone {
		if _, ok := all[id]; !ok {
			t.Errorf("OnEvict got value %d (reason %v), which was never written", id, reason)
		}
	}

	// The counters match the OnEvict calls.
	var reasons [4]uint64
	for _, r := range gone {
		if r >= 0 && int(r) < len(reasons) {
			reasons[r]++
		}
	}
	var st cache.Stats
	within(t, "Stats()", func() { st = c.Stats() })
	if st.Evictions != reasons[cache.Evicted] {
		t.Errorf("Stats().Evictions = %d, but OnEvict got %d calls with reason evicted", st.Evictions, reasons[cache.Evicted])
	}
	if st.Expirations != reasons[cache.Expired] {
		t.Errorf("Stats().Expirations = %d, but OnEvict got %d calls with reason expired", st.Expirations, reasons[cache.Expired])
	}
	if d := deletes.Load(); reasons[cache.Deleted] != d {
		t.Errorf("Delete returned true %d times, but OnEvict got %d calls with reason deleted", d, reasons[cache.Deleted])
	}
	if g := gets.Load(); st.Hits+st.Misses != g {
		t.Errorf("Stats() = %+v: Hits + Misses = %d, want %d (the number of Get calls)", st, st.Hits+st.Misses, g)
	}
}

func TestConcurrentHotKeys(t *testing.T) {
	// 8 goroutines read and write the same 2 keys, like the requests of
	// one busy tenant. The race detector (-race) reports any access to an
	// entry, the order of use or the counters that the lock does not cover,
	// and a collision on the same entry is likely with only 2 keys.
	const (
		goroutines = 8
		callsEach  = 2000
	)
	var c *cache.Cache[string, int]
	within(t, "New", func() {
		c = cache.New(cache.Options[string, int]{Capacity: 2, OnEvict: func(string, int, cache.Reason) {}})
	})
	hot := []string{"hot-even", "hot-odd"} // hot[i%2] only ever gets the value i
	var (
		wg     sync.WaitGroup
		gets   atomic.Uint64
		panics = make(chan any, goroutines)
		wrong  = make(chan string, goroutines)
	)
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					panics <- v
				}
			}()
			for i := range callsEach {
				key := hot[i%2]
				if g%2 == 0 {
					c.Set(key, i)
					continue
				}
				gets.Add(1)
				if v, ok := c.Get(key); ok && v%2 != i%2 {
					wrong <- fmt.Sprintf("Get(%q) = %d, true: a value written for the other key", key, v)
					return
				}
			}
		}()
	}
	select {
	case <-async(wg.Wait):
	case <-time.After(2 * timeout):
		t.Fatalf("%d goroutines doing %d calls each did not finish within %v (deadlock?)", goroutines, callsEach, 2*timeout)
	}
	close(panics)
	close(wrong)
	for v := range panics {
		t.Fatalf("a call panicked: %v", v)
	}
	for msg := range wrong {
		t.Error(msg)
	}
	var st cache.Stats
	within(t, "Stats()", func() { st = c.Stats() })
	if g := gets.Load(); st.Hits+st.Misses != g {
		t.Errorf("Stats() = %+v: Hits + Misses = %d, want %d (the number of Get calls)", st, st.Hits+st.Misses, g)
	}
	var keys []string
	within(t, "Keys()", func() { keys = c.Keys() })
	if len(keys) != 2 {
		t.Errorf("Keys() = %q at the end, want both hot keys", keys)
	}
}

func TestLargeCacheStaysFast(t *testing.T) {
	// Get, Set and Delete must take O(1) time: no work that grows with the
	// number of entries. This test makes 350 000 calls on a cache of 100 000
	// entries. With O(1) calls that takes well under a second, or 1 to 2 s
	// with -race. A solution that scans a slice or the whole map on every Get
	// or eviction does about 100 000 times more work and stops at the limit.
	const (
		n     = 100_000
		limit = 20 * time.Second
	)
	keys := make([]string, 2*n)
	for i := range keys {
		keys[i] = fmt.Sprintf("tenant-%06d/config", i)
	}
	var c *cache.Cache[string, int]
	within(t, "New", func() {
		c = cache.New(cache.Options[string, int]{Capacity: n, Now: newClock().Now})
	})

	var problem string
	var elapsed time.Duration
	run := func() {
		began := time.Now()
		defer func() { elapsed = time.Since(began) }()
		tooSlow := func(i int, phase string) bool {
			if i%1024 == 0 && time.Since(began) > limit {
				problem = fmt.Sprintf("%s: still at call %d after %v", phase, i, limit)
				return true
			}
			return false
		}
		// Fill the cache.
		for i := range n {
			if tooSlow(i, "Set of 100 000 new keys") {
				return
			}
			c.Set(keys[i], i)
		}
		// Use every entry once, in a scrambled order: 7919 is prime, so
		// i*7919 % n visits every index once.
		for i := range n {
			if tooSlow(i, "Get of 100 000 keys") {
				return
			}
			k := i * 7919 % n
			if v, ok := c.Get(keys[k]); !ok || v != k {
				problem = fmt.Sprintf("Get(%q) = %d, %v; want %d, true", keys[k], v, ok, k)
				return
			}
		}
		// 100 000 new keys: every Set evicts the least recently used entry.
		for i := n; i < 2*n; i++ {
			if tooSlow(i-n, "Set of 100 000 more keys, each evicting one") {
				return
			}
			c.Set(keys[i], i)
		}
		// Delete half of them, in the scrambled order.
		for i := range n / 2 {
			if tooSlow(i, "Delete of 50 000 keys") {
				return
			}
			k := n + i*7919%n
			if !c.Delete(keys[k]) {
				problem = fmt.Sprintf("Delete(%q) = false, want true", keys[k])
				return
			}
		}
	}
	select {
	case v := <-async(run):
		if v != nil {
			t.Fatalf("panicked: %v", v)
		}
	case <-time.After(limit + timeout):
		t.Fatalf("350 000 calls on a cache of 100 000 entries did not finish within %v; Get, Set and Delete must be O(1)", limit+timeout)
	}
	if problem != "" {
		t.Fatalf("%s; Get, Set and Delete must be O(1)", problem)
	}
	var size int
	within(t, "Len()", func() { size = c.Len() })
	if size != n/2 {
		t.Errorf("Len() = %d at the end, want %d", size, n/2)
	}
	t.Logf("350 000 calls on a cache of %d entries took %v", n, elapsed)
}
