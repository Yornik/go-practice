package cache_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	cache "github.com/yornik/go-practice/exercises/09-cache"
)

// Step 2: capacity and the order of use (LRU). No TTL yet.

func TestLRUEvictsLeastRecentlyUsed(t *testing.T) {
	h := newCache(t, 3, 0)
	h.Set("a", 1)
	h.Set("b", 2)
	h.Set("c", 3)
	h.wantKeys("c", "b", "a")
	h.wantLen(3)

	h.Set("d", 4) // full: "a" is the least recently used
	h.wantKeys("d", "c", "b")
	h.wantLen(3)

	h.Set("e", 5)
	h.wantKeys("e", "d", "c")
	h.wantMiss("a")
	h.wantMiss("b")
}

func TestLRUGetMarksUsed(t *testing.T) {
	h := newCache(t, 3, 0)
	h.Set("a", 1)
	h.Set("b", 2)
	h.Set("c", 3)
	h.wantHit("a", 1) // "a" is now the most recently used
	h.wantKeys("a", "c", "b")

	h.Set("d", 4) // so "b" goes, not "a"
	h.wantKeys("d", "a", "c")
}

func TestLRUSetExistingMarksUsed(t *testing.T) {
	h := newCache(t, 3, 0)
	h.Set("a", 1)
	h.Set("b", 2)
	h.Set("c", 3)
	h.Set("a", 10) // replace: no eviction, "a" is the most recently used
	h.wantKeys("a", "c", "b")
	h.wantLen(3)

	h.Set("d", 4)
	h.wantKeys("d", "a", "c")
	h.wantHit("a", 10)
}

func TestLRUKeysAndLenDoNotMarkUsed(t *testing.T) {
	h := newCache(t, 3, 0)
	h.wantKeys() // empty cache: no keys

	h.Set("a", 1)
	h.Set("b", 2)
	h.Set("c", 3)
	h.wantKeys("c", "b", "a")
	h.wantKeys("c", "b", "a") // calling Keys again changes nothing
	h.wantLen(3)

	h.Set("d", 4) // "a" is still the least recently used
	h.wantKeys("d", "c", "b")
}

func TestLRUDeleteFreesSlot(t *testing.T) {
	h := newCache(t, 2, 0)
	h.Set("a", 1)
	h.Set("b", 2)
	h.wantDelete("a", true)

	h.Set("c", 3) // there is room again: "b" stays
	h.wantKeys("c", "b")
	h.wantLen(2)
}

func TestLRUCapacityOne(t *testing.T) {
	h := newCache(t, 1, 0)
	h.Set("a", 1)
	h.Set("b", 2)
	h.wantKeys("b")
	h.wantMiss("a")

	h.Set("b", 3) // replace the only entry
	h.wantKeys("b")
	h.wantHit("b", 3)
	h.wantLen(1)
}

func TestLRUWithoutOnEvict(t *testing.T) {
	// OnEvict is nil: evicting must not call it.
	var c *cache.Cache[string, int]
	within(t, "New(Options{Capacity: 1})", func() {
		c = cache.New(cache.Options[string, int]{Capacity: 1})
	})
	within(t, `Set("a", 1)`, func() { c.Set("a", 1) })
	within(t, `Set("b", 2) evicting "a" with OnEvict nil`, func() { c.Set("b", 2) })
	within(t, `Set("b", 3) replacing "b" with OnEvict nil`, func() { c.Set("b", 3) })
	var keys []string
	within(t, "Keys()", func() { keys = c.Keys() })
	if !slices.Equal(keys, []string{"b"}) {
		t.Errorf("Keys() = %q, want [\"b\"]", keys)
	}
}

// lruModel is a deliberately simple LRU: a slice of keys, the most recently
// used first. Every operation scans the slice, so it is far too slow for
// real use (see TestLargeCacheStaysFast), but it is easy to check by eye.
type lruModel struct {
	capacity int
	order    []string
	values   map[string]int
}

func (m *lruModel) touch(key string) {
	m.order = slices.DeleteFunc(m.order, func(k string) bool { return k == key })
	m.order = slices.Insert(m.order, 0, key)
}

func (m *lruModel) get(key string) (int, bool) {
	v, ok := m.values[key]
	if ok {
		m.touch(key)
	}
	return v, ok
}

func (m *lruModel) set(key string, v int) {
	if _, ok := m.values[key]; !ok && len(m.order) == m.capacity {
		lru := m.order[len(m.order)-1]
		m.order = m.order[:len(m.order)-1]
		delete(m.values, lru)
	}
	m.values[key] = v
	m.touch(key)
}

func (m *lruModel) delete(key string) bool {
	_, ok := m.values[key]
	delete(m.values, key)
	m.order = slices.DeleteFunc(m.order, func(k string) bool { return k == key })
	return ok
}

func TestLRUMatchesSimpleModel(t *testing.T) {
	// 3000 random Set, Get and Delete calls on 8 keys and a capacity of 4,
	// compared with lruModel after every call. This finds the list bugs that
	// the hand-written cases above miss: moving the last entry to the front,
	// deleting the first or the last entry, and so on. The seed is fixed, so
	// every run makes the same calls.
	h := newCache(t, 4, 0)
	m := &lruModel{capacity: 4, values: map[string]int{}}
	rng := rand.New(rand.NewPCG(9, 9))
	var history []string // the last calls, for the failure message
	for i := range 3000 {
		key := fmt.Sprintf("k%d", rng.IntN(8))
		var call, got, want string
		switch op := rng.IntN(10); {
		case op < 4:
			call = fmt.Sprintf("Set(%q, %d)", key, i)
			h.Set(key, i)
			m.set(key, i)
		case op < 8:
			call = fmt.Sprintf("Get(%q)", key)
			v, ok := h.Get(key)
			mv, mok := m.get(key)
			got, want = fmt.Sprint(v, ok), fmt.Sprint(mv, mok)
		default:
			call = fmt.Sprintf("Delete(%q)", key)
			got, want = fmt.Sprint(h.Delete(key)), fmt.Sprint(m.delete(key))
		}
		history = append(history, call)
		if len(history) > 8 {
			history = history[1:]
		}
		if got != want {
			t.Fatalf("call %d: %s = %s, want %s\nlast calls: %q", i+1, call, got, want, history)
		}
		if keys := h.Keys(); !slices.Equal(keys, m.order) {
			t.Fatalf("call %d: after %s: Keys() = %q, want %q\nlast calls: %q", i+1, call, keys, m.order, history)
		}
		if n := h.Len(); n != len(m.order) {
			t.Fatalf("call %d: after %s: Len() = %d, want %d\nlast calls: %q", i+1, call, n, len(m.order), history)
		}
	}
}
