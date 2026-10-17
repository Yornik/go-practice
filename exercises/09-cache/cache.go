// Package cache is an in-memory key/value cache with a fixed capacity. When
// it is full it removes the least recently used entry (LRU), entries can
// expire after a time to live (TTL), and a callback reports every removal.
package cache

import (
	"fmt"
	"time"
)

// Reason says why an entry left the cache. OnEvict receives it.
type Reason int

const (
	// Evicted: Set or SetWithTTL added a new key to a full cache, and this
	// entry was the least recently used one and still live.
	Evicted Reason = iota
	// Expired: the entry's TTL had passed when an operation touched it.
	Expired
	// Deleted: Delete removed the entry.
	Deleted
	// Replaced: Set or SetWithTTL wrote a new value for a key that held a
	// live entry. OnEvict receives the old value.
	Replaced
)

// String returns "evicted", "expired", "deleted" or "replaced".
func (r Reason) String() string {
	switch r {
	case Evicted:
		return "evicted"
	case Expired:
		return "expired"
	case Deleted:
		return "deleted"
	case Replaced:
		return "replaced"
	}
	return fmt.Sprintf("Reason(%d)", int(r))
}

// Options configure a Cache.
type Options[K comparable, V any] struct {
	// Capacity is the maximum number of entries the cache stores. It must be
	// at least 1.
	Capacity int
	// TTL is the time to live that Set gives an entry. 0 means entries
	// written by Set never expire. It must not be negative.
	TTL time.Duration
	// Now returns the current time. nil means time.Now. Tests pass a fake
	// clock here.
	Now func() time.Time
	// OnEvict, if not nil, is called once for every value that leaves the
	// cache, with the reason. It is called in the goroutine that made the
	// change, after the method's whole change is complete, without holding
	// the cache's lock, and before that method returns. So it may call
	// methods of the cache. It may be called from several goroutines at the
	// same time.
	OnEvict func(key K, value V, reason Reason)
}

// Stats are counters since the cache was created.
type Stats struct {
	Hits        uint64 // Get calls that found a live entry
	Misses      uint64 // Get calls that found nothing, or only an expired entry
	Evictions   uint64 // removals with reason Evicted
	Expirations uint64 // removals with reason Expired
}

// HitRatio returns Hits / (Hits + Misses), or 0 if there was no Get yet.
func (s Stats) HitRatio() float64 {
	panic("TODO: implement")
}

// Cache is a fixed-capacity LRU cache with optional expiry. Create it with
// New. All methods are safe for concurrent use.
type Cache[K comparable, V any] struct {
	// TODO: add fields.
}

// New returns an empty cache. It panics if o.Capacity < 1 or o.TTL < 0:
// those are configuration bugs, and a cache that cannot hold anything should
// fail at startup, not quietly cache nothing.
func New[K comparable, V any](o Options[K, V]) *Cache[K, V] {
	panic("TODO: implement")
}

// Get returns the value stored for key and true, and marks the entry as the
// most recently used one. If there is no live entry for key, Get returns the
// zero value of V and false. An expired entry is removed (reason Expired)
// and Get reports a miss. Get does not extend the entry's TTL. Get takes
// O(1) time.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	panic("TODO: implement")
}

// Set stores value for key with the TTL from Options and marks the entry as
// the most recently used one. It is SetWithTTL(key, value, o.TTL).
func (c *Cache[K, V]) Set(key K, value V) {
	panic("TODO: implement")
}

// SetWithTTL stores value for key with the given ttl and marks the entry as
// the most recently used one. ttl 0 means the entry never expires; a
// negative ttl stores the entry already expired.
//
// If key holds a live entry, its value is replaced (OnEvict receives the old
// value with reason Replaced) and the TTL starts again. If key holds an
// expired entry, that entry is removed (reason Expired) and key counts as
// new. If key is new and the cache is full, the least recently used entry is
// removed first, with reason Evicted, or Expired if it had already expired.
// SetWithTTL takes O(1) time.
func (c *Cache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) {
	panic("TODO: implement")
}

// Delete removes the entry for key and reports whether a live entry was
// removed (reason Deleted). If the entry had expired, it is removed with
// reason Expired and Delete returns false. Delete takes O(1) time.
func (c *Cache[K, V]) Delete(key K) bool {
	panic("TODO: implement")
}

// Len returns the number of entries stored, including expired entries that
// have not been removed yet. It never exceeds Capacity. Len takes O(1) time.
func (c *Cache[K, V]) Len() int {
	panic("TODO: implement")
}

// Keys returns the keys of the live entries, the most recently used first.
// It skips expired entries but does not remove them, and it does not change
// the order of use or the Stats.
func (c *Cache[K, V]) Keys() []K {
	panic("TODO: implement")
}

// PurgeExpired removes every expired entry (reason Expired) and returns how
// many it removed.
func (c *Cache[K, V]) PurgeExpired() int {
	panic("TODO: implement")
}

// Stats returns a copy of the counters.
func (c *Cache[K, V]) Stats() Stats {
	panic("TODO: implement")
}
