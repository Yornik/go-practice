# 09 · Cache: generic LRU with TTL

**Tier 2 · about 2 h** · Go: generics and `comparable`, the zero value of a type parameter, a map plus a doubly linked list, `sync.Mutex`, callbacks outside a lock, an injected clock · Problem solving: choose data structures from the required complexities; keep two structures in sync; decide what each operation does with an expired entry

Every request to your API needs the tenant's config from an internal config service. It answers
in 200 ms on a good day and falls over when 40 pods ask on every request, so you cache the
answers in memory. A plain map grows until the pod is OOM-killed, so the cache needs a
**capacity**: when full, it drops the **least recently used** entry (LRU). It must not serve a
config that changed an hour ago, so entries **expire** after a time to live (TTL). Values that
hold resources, such as a gRPC client per tenant, must be closed when they leave: a **callback
for every removal**. **Hit and miss counters** tell you whether the cache earns its memory.
`hashicorp/golang-lru` and Kubernetes' `LRUExpireCache` do this; here you build it.

## Task

Implement the stubs in `cache.go`. `Reason` with its constants and `String`, `Options` and
`Stats` are given. You write `HitRatio`, the `Cache` fields and its methods.

```go
type Reason int // Evicted, Expired, Deleted, Replaced; String() gives "evicted", ...

type Options[K comparable, V any] struct {
	Capacity int                                 // maximum number of entries (>= 1)
	TTL      time.Duration                       // TTL that Set uses; 0: never expire
	Now      func() time.Time                    // the clock; nil: time.Now
	OnEvict  func(key K, value V, reason Reason) // called for every removal; may be nil
}

type Stats struct{ Hits, Misses, Evictions, Expirations uint64 }
func (s Stats) HitRatio() float64 // Hits / (Hits + Misses); 0 before the first Get

type Cache[K comparable, V any] struct{ /* you design the fields */ }
func New[K comparable, V any](o Options[K, V]) *Cache[K, V]        // panics if Capacity < 1 or TTL < 0
func (c *Cache[K, V]) Get(key K) (V, bool)                         // value and true, or zero value and false
func (c *Cache[K, V]) Set(key K, value V)                          // SetWithTTL(key, value, o.TTL)
func (c *Cache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) // insert or replace
func (c *Cache[K, V]) Delete(key K) bool                           // true if a live entry was removed
func (c *Cache[K, V]) Len() int                                    // entries stored, expired ones included
func (c *Cache[K, V]) Keys() []K                                   // live keys, most recently used first
func (c *Cache[K, V]) PurgeExpired() int                           // remove all expired entries; how many
func (c *Cache[K, V]) Stats() Stats                                // counters since New
```

## Rules

### New and complexity

- `New` panics if `Capacity < 1` or `TTL < 0`: configuration bugs should fail at startup.
  `Now == nil` means `time.Now`; `OnEvict == nil` means no callback.
- `Get`, `Set`, `SetWithTTL`, `Delete` and `Len` take **O(1)** time: no loop over the entries.
  `Keys` and `PurgeExpired` may take O(n). `TestLargeCacheStaysFast` makes 350 000 calls on
  100 000 entries and gives up after 20 s; O(1) calls need under 2 s, also with `-race`.

### Order of use (LRU)

- An entry is **used** when Set or SetWithTTL writes it, or Get returns it. Nothing else changes
  the order: not Keys, not Len, not a miss.
- Set or SetWithTTL of a **new** key into a cache that holds `Capacity` entries first removes
  the least recently used entry. `Len()` never exceeds `Capacity`.
- Writing a key that holds a live entry replaces the value. Nothing else is removed.
- `Keys()` returns the keys of the live entries, most recently used first.

### Time to live

- An entry written at time `t` with a TTL `d > 0` is **live** while `now < t + d`, and
  **expired** from `t + d` on. Each call that needs the time reads `Options.Now`; the cache
  never remembers an earlier reading.
- TTL `0`: never expires. A **negative** TTL (SetWithTTL only) stores the entry already expired:
  a TTL computed as `deadline.Sub(now)` for a deadline that has passed must not mean "forever".
- Every write starts a new TTL. Get does **not** extend it.

### Expired entries

There is no timer and no background goroutine. An expired entry stays stored until an operation
**touches** it:

| Operation | Touches |
|---|---|
| `Get(key)`, `Set(key, ...)`, `SetWithTTL(key, ...)`, `Delete(key)` | the entry of that key |
| Set or SetWithTTL of a new key in a full cache | the least recently used entry |
| `PurgeExpired()` | every entry |

The operation removes the expired entry with reason `Expired`, then carries on as if the key
were absent: Get misses, Delete returns false, and Set inserts a new entry (not a "replace")
into the slot it just freed, so nothing else is evicted. An expired least recently used entry
removed to make room is an expiry, not an eviction: no live data was pushed out.

`Len` counts expired entries that are still stored: they use memory and count towards
`Capacity`. `Keys` skips them. Keys and Len never remove anything.

### OnEvict

| Reason | When | value passed |
|---|---|---|
| `Evicted` | a new key needs room; the least recently used entry was live | the removed value |
| `Expired` | an operation touched an expired entry (table above) | the removed value |
| `Deleted` | Delete removed a live entry | the removed value |
| `Replaced` | Set or SetWithTTL wrote a key that held a live entry | the **old** value |

- OnEvict is called exactly once per removal or replacement, and never when nothing was removed.
- It runs in the goroutine that made the change, **after** the method's whole change is complete
  and **without holding the cache's lock**, before the method returns. Inside OnEvict a removed
  key is gone (Len does not count it, Get misses it); after `Evicted` the new key is already
  stored; after `Replaced` Get returns the new value.
- OnEvict may call any method of the same cache. While a slow OnEvict runs, other goroutines can
  use the cache.
- PurgeExpired calls OnEvict once per removed entry, in no particular order.

### Stats

- `Hits` / `Misses`: Get calls that returned true / false (absent or expired).
- `Evictions` / `Expirations`: removals with reason Evicted / Expired, by any operation.
- Nothing else changes them: not a replace, a Delete of a live entry, Keys or Len.
- `HitRatio()` is `Hits / (Hits + Misses)`, and 0 when both are 0.

### Concurrency

- All methods are safe for concurrent use. `Len()` never exceeds `Capacity`, not even for a
  moment while other goroutines call Set.
- Every value written is either still in the cache or was passed to OnEvict exactly once, and
  the counters match the OnEvict calls. The tests check both with 8 goroutines.
- `TestConcurrentHotKeys` has 8 goroutines call Set and Get on the same 2 keys. Run step 6 with
  `-race`: the race detector fails it on any access the lock does not cover.

## Examples

`Capacity` 3, no TTL:

```
call                                   OnEvict                    Keys() after
Set("a", 1); Set("b", 2); Set("c", 3)                             [c b a]
Get("a") = 1, true                                                [a c b]
Set("d", 4)                            ("b", 2, evicted)          [d a c]
Set("a", 10)                           ("a", 1, replaced)         [a d c]
Delete("c") = true                     ("c", 3, deleted)          [a d]
```

`Capacity` 2, `TTL` 10 s, fake clock:

```
00:00  Set("a", 1)
00:05  Set("b", 2)
00:09  Get("a") = 1, true            (Get does not extend the TTL of "a")
00:10  Len() = 2, Keys() = [b]       ("a" expired at 00:10 but is still stored)
00:10  Get("a") = 0, false           OnEvict("a", 1, expired)   Len() = 1
00:12  SetWithTTL("c", 3, 0)         (never expires)
00:15  Set("d", 4)                   full; least recently used is "b", expired at 00:15:
                                     OnEvict("b", 2, expired)   Keys() = [d c]
Stats() = {Hits: 1, Misses: 1, Evictions: 0, Expirations: 2}, HitRatio() = 0.5
```

## Run the tests

    go test ./exercises/09-cache/
    go test -race ./exercises/09-cache/
    go test -run 'TestNew|TestBasic' -v ./exercises/09-cache/            # step 1: New, Set(WithTTL), Get, Delete, Len
    go test -run TestLRU -v ./exercises/09-cache/                        # step 2: capacity and order of use
    go test -run TestTTL -v ./exercises/09-cache/                        # step 3: time to live
    go test -run TestOnEvict -v ./exercises/09-cache/                    # step 4: the callback
    go test -run TestStats -v ./exercises/09-cache/                      # step 5: counters
    go test -race -run 'TestConcurrent|TestLarge' -v ./exercises/09-cache/   # step 6: goroutines and O(1)

A stub that panics stops the whole test binary, so run one step at a time; the 6 filters do not
overlap. Every call into the cache has a 5 s timeout, so a lock that is never released fails a
test instead of hanging. `TestLRUMatchesSimpleModel` compares your cache with a slow, obviously
correct LRU and prints the last calls when they disagree.

## Docs you can use

- [An Introduction To Generics](https://go.dev/blog/intro-generics); the spec on
  [Type constraints](https://go.dev/ref/spec#Type_constraints) and
  [The zero value](https://go.dev/ref/spec#The_zero_value)
- [`container/list`](https://pkg.go.dev/container/list), [`sync.Mutex`](https://pkg.go.dev/sync#Mutex),
  [`time.Time.Before`](https://pkg.go.dev/time#Time.Before)
- [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [`github.com/hashicorp/golang-lru/v2`](https://pkg.go.dev/github.com/hashicorp/golang-lru/v2)
  and its [`expirable`](https://pkg.go.dev/github.com/hashicorp/golang-lru/v2/expirable)
  package: widely used Go LRU caches. Read the docs, not the source, until you are done.

## Stretch goals

- `Peek(key) (V, bool)`: read without marking the entry used or changing Stats (a debug page).
- An expiry index (a heap ordered by expiry time), so PurgeExpired touches only expired entries,
  and a janitor goroutine with a `Close` that stops it (12-workerpool tests for leaks).
- Export Stats as Prometheus counters; compute the hit ratio in PromQL with `rate()`.
- `GetOrLoad(key, load)`: what if 1000 requests miss the same key at once? See 20-singleflight.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 09 hint 1
    go run ./cmd/peek 09 hint 2
    go run ./cmd/peek 09 hint 3
    go run ./cmd/peek 09 explanation
    go run ./cmd/peek 09 walkthrough
    go run ./cmd/peek 09 solution
