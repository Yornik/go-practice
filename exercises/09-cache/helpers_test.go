package cache_test

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	cache "github.com/yornik/go-practice/exercises/09-cache"
)

// timeout bounds every call into the cache, so a lock that is never released
// fails the test with a message instead of hanging the test binary.
const timeout = 5 * time.Second

// start is the time the fake clock shows when a test begins.
var start = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// fakeClock is a clock that moves only when the test says so. Pass its Now
// method as Options.Now. Safe for concurrent use.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// event is one call of OnEvict.
type event[K comparable, V comparable] struct {
	Key    K
	Value  V
	Reason cache.Reason
}

func (e event[K, V]) String() string {
	return fmt.Sprintf("(%#v, %#v, %v)", e.Key, e.Value, e.Reason)
}

// recorder records the calls of OnEvict. Pass its OnEvict method as
// Options.OnEvict. Safe for concurrent use.
type recorder[K comparable, V comparable] struct {
	mu     sync.Mutex
	events []event[K, V]
}

func (r *recorder[K, V]) OnEvict(key K, value V, reason cache.Reason) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event[K, V]{Key: key, Value: value, Reason: reason})
}

// Take returns the calls recorded since the last Take.
func (r *recorder[K, V]) Take() []event[K, V] {
	r.mu.Lock()
	defer r.mu.Unlock()
	got := r.events
	r.events = nil
	return got
}

// async runs f in a new goroutine. The returned channel receives the value f
// panicked with, or nil when f returns normally.
func async(f func()) <-chan any {
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	return done
}

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics (for example with "TODO: implement") or does not return within the
// timeout.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	select {
	case v := <-async(f):
		if v != nil {
			t.Fatalf("%s panicked: %v", what, v)
		}
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v (is the cache's lock held, or never released?)", what, timeout)
	}
}

// mustPanic fails the test unless f panics.
func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	select {
	case v := <-async(f):
		if v == nil {
			t.Errorf("%s did not panic, want a panic", what)
		}
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v", what, timeout)
	}
}

// harness wraps a cache under test. Every call goes through within, and
// the wrapper remembers the last call for failure messages. The cache uses
// a fake clock and records every OnEvict call.
type harness[K comparable, V comparable] struct {
	t     *testing.T
	c     *cache.Cache[K, V]
	clock *fakeClock
	rec   *recorder[K, V]
	last  string // the last call, such as `Set("a", 1)`
}

// newHarness creates a cache with o. It sets o.Now to a fake clock and
// o.OnEvict to a recorder.
func newHarness[K comparable, V comparable](t *testing.T, o cache.Options[K, V]) *harness[K, V] {
	t.Helper()
	h := &harness[K, V]{t: t, clock: newClock(), rec: &recorder[K, V]{}}
	o.Now = h.clock.Now
	o.OnEvict = h.rec.OnEvict
	within(t, fmt.Sprintf("New(Options{Capacity: %d, TTL: %v})", o.Capacity, o.TTL), func() {
		h.c = cache.New(o)
	})
	if h.c == nil {
		t.Fatalf("New(Options{Capacity: %d, TTL: %v}) = nil, want a cache", o.Capacity, o.TTL)
	}
	return h
}

// newCache creates a string -> int cache with a fake clock and a recorder.
func newCache(t *testing.T, capacity int, ttl time.Duration) *harness[string, int] {
	t.Helper()
	return newHarness(t, cache.Options[string, int]{Capacity: capacity, TTL: ttl})
}

func (h *harness[K, V]) call(what string, f func()) {
	h.t.Helper()
	h.last = what
	within(h.t, what, f)
}

func (h *harness[K, V]) Get(key K) (V, bool) {
	h.t.Helper()
	var v V
	var ok bool
	h.call(fmt.Sprintf("Get(%#v)", key), func() { v, ok = h.c.Get(key) })
	return v, ok
}

func (h *harness[K, V]) Set(key K, value V) {
	h.t.Helper()
	h.call(fmt.Sprintf("Set(%#v, %#v)", key, value), func() { h.c.Set(key, value) })
}

func (h *harness[K, V]) SetWithTTL(key K, value V, ttl time.Duration) {
	h.t.Helper()
	h.call(fmt.Sprintf("SetWithTTL(%#v, %#v, %v)", key, value, ttl), func() { h.c.SetWithTTL(key, value, ttl) })
}

func (h *harness[K, V]) Delete(key K) bool {
	h.t.Helper()
	var ok bool
	h.call(fmt.Sprintf("Delete(%#v)", key), func() { ok = h.c.Delete(key) })
	return ok
}

func (h *harness[K, V]) Len() int {
	h.t.Helper()
	var n int
	h.call("Len()", func() { n = h.c.Len() })
	return n
}

func (h *harness[K, V]) Keys() []K {
	h.t.Helper()
	var keys []K
	h.call("Keys()", func() { keys = h.c.Keys() })
	return keys
}

func (h *harness[K, V]) PurgeExpired() int {
	h.t.Helper()
	var n int
	h.call("PurgeExpired()", func() { n = h.c.PurgeExpired() })
	return n
}

func (h *harness[K, V]) Stats() cache.Stats {
	h.t.Helper()
	var s cache.Stats
	h.call("Stats()", func() { s = h.c.Stats() })
	return s
}

// Advance moves the fake clock forward.
func (h *harness[K, V]) Advance(d time.Duration) { h.clock.Advance(d) }

// wantHit calls Get(key) and fails the test unless it returns want, true.
func (h *harness[K, V]) wantHit(key K, want V) {
	h.t.Helper()
	if got, ok := h.Get(key); !ok || got != want {
		h.t.Errorf("Get(%#v) = %#v, %v; want %#v, true", key, got, ok, want)
	}
}

// wantMiss calls Get(key) and fails the test unless it returns the zero
// value and false.
func (h *harness[K, V]) wantMiss(key K) {
	h.t.Helper()
	var zero V
	if got, ok := h.Get(key); ok || got != zero {
		h.t.Errorf("Get(%#v) = %#v, %v; want %#v, false (a miss)", key, got, ok, zero)
	}
}

// wantDelete calls Delete(key) and fails the test unless it returns want.
func (h *harness[K, V]) wantDelete(key K, want bool) {
	h.t.Helper()
	if got := h.Delete(key); got != want {
		h.t.Errorf("Delete(%#v) = %v, want %v", key, got, want)
	}
}

// wantKeys calls Keys and fails the test unless it returns want, in order
// (most recently used first).
func (h *harness[K, V]) wantKeys(want ...K) {
	h.t.Helper()
	if got := h.Keys(); !slices.Equal(got, want) {
		h.t.Errorf("Keys() = %#v, want %#v (most recently used first)", got, want)
	}
}

// wantLen calls Len and fails the test unless it returns want.
func (h *harness[K, V]) wantLen(want int) {
	h.t.Helper()
	if got := h.Len(); got != want {
		h.t.Errorf("Len() = %d, want %d", got, want)
	}
}

// wantEvents fails the test unless the OnEvict calls since the last check
// are exactly want, in this order.
func (h *harness[K, V]) wantEvents(want ...event[K, V]) {
	h.t.Helper()
	if got := h.rec.Take(); !slices.Equal(got, want) {
		h.t.Errorf("after %s: OnEvict calls = %v, want %v", h.last, got, want)
	}
}

// ev is a short way to write an expected OnEvict call of a string -> int
// cache.
func ev(key string, value int, reason cache.Reason) event[string, int] {
	return event[string, int]{Key: key, Value: value, Reason: reason}
}
