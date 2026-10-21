# 33 · Token bucket and per-client limits

**Tier 7 · about 2 h** · Go: `sync.Mutex`, `time.Time` arithmetic and an injected clock, float64 state, maps with eviction, concurrency tests with `-race` · Problem solving: model state as "a value at time t" instead of a ticker; find the read-modify-write steps that must be atomic

Your team runs the internal config API that 300 services call. A misconfigured CronJob in another
team's namespace starts 50 pods that poll `/v1/config` in a tight loop: 2000 requests per second
from one client. The p99 latency goes from 40 ms to 4 s for every caller, and you get paged. A
global limit rejects the good clients as often as the noisy one. A token bucket per client does
not: each client ID or IP may burst a little and then gets a steady rate, and only the noisy one
gets `429 Too Many Requests`. Two details make it production code: the count stays exact under
concurrent requests, and the map of buckets does not keep every IP it has ever seen, scanners
included, until the pod is OOM-killed.

## Task

Implement the stubs in `ratelimit.go`:

```go
var ErrExceedsBurst, ErrNoRefill error // given

type Limiter struct{ /* you design the fields */ }
func New(rate float64, burst int, now func() time.Time) *Limiter // rate tokens/s, at most burst; starts full
func (l *Limiter) Allow() bool                           // AllowN(1)
func (l *Limiter) AllowN(n int) bool                     // take n tokens if there are n; all or nothing
func (l *Limiter) Tokens() float64                       // the count now; negative while reservations are open
func (l *Limiter) Reserve(n int) (time.Duration, error)  // take n now, on credit if needed; return the wait
func (l *Limiter) Cancel(n int)                          // give n tokens back

type Keyed struct{ /* you design the fields */ }
func NewKeyed(rate float64, burst int, idle time.Duration, now func() time.Time) *Keyed
func (k *Keyed) Allow(key string) bool                   // one bucket per key, created full
func (k *Keyed) Len() int                                // number of buckets held
func (k *Keyed) Sweep() int                              // remove idle, full buckets; return how many
```

Suggested order: Limiter without Reserve and Cancel, then Reserve and Cancel, then Keyed.
`http_test.go` shows how a server uses Keyed.

## Rules

### The bucket

- `rate` is in tokens per second, `burst` is the capacity. A new Limiter is full: `Tokens() == burst`.
- `now` is the clock. `nil` means `time.Now`. The tests pass a fake clock and move it by hand.
- No goroutine, no timer. Each call computes the refill from the time since the previous call:
  `tokens = min(burst, tokens + rate × elapsed seconds)`. After an hour without calls the bucket
  is full, never more than full.
- Tokens are a float64 and fractions count. With rate 4, two calls 125 ms apart refill
  0.5 + 0.5 = 1 token.
- `rate == 0`: the bucket never refills.
- The clock goes backwards (now is before the previous call): that counts as no elapsed time, so
  no refill and no tokens lost. The earlier time becomes the new reference and the refill
  continues from there; it does not wait for the clock to catch up. (The time between two
  `time.Now` readings is never negative, see Monotonic Clocks below. An injected clock can go
  back.)
- New panics if `rate` is negative, NaN or +Inf, or if `burst < 1`. (`strconv.ParseFloat`
  accepts `"NaN"` and `"Inf"`, so a config value can be one.) NewKeyed panics in the same cases
  and if `idle < 0`. `rate == 0` and `idle == 0` are valid.

### Allow, AllowN, Tokens

- `AllowN(n)`: if there are at least n tokens, take n and return true. Otherwise take nothing and
  return false. So `AllowN(n)` with `n > burst` is always false.
- `Allow()` is `AllowN(1)`.
- `Tokens()` returns the count now, after the refill, with its fraction. It is negative while
  reservations are open (see below).
- n is at least 1 in AllowN, Reserve and Cancel. The tests never pass less.

### Reserve and Cancel

`Reserve(n)` is for callers that wait instead of failing, such as a batch job or an outgoing
client. It brings the bucket up to date, then applies the first row that matches:

| Case | Returns | Tokens afterwards |
|---|---|---|
| `n > burst` | `0, ErrExceedsBurst` | unchanged |
| at least n tokens | `0, nil` | n fewer |
| fewer than n tokens, `rate > 0` | `wait, nil`: `wait = (n − tokens) / rate` seconds | n fewer, so negative |
| fewer than n tokens, `rate == 0` | `0, ErrNoRefill` | unchanged |

- A negative count is a debt. The next reservation queues behind it: its wait includes the time
  to pay the debt back. Allow and AllowN return false until the refill has paid the debt and
  brought the tokens they need.
- The errors may be wrapped. The tests use `errors.Is`.
- `Cancel(n)` brings the bucket up to date and adds n tokens. The count never goes above `burst`.

### Keyed

- One bucket per key, with the rate and burst given to NewKeyed, created full on the first
  `Allow(key)`. Keys are independent. `Allow(key)` behaves like `Limiter.Allow` on key's bucket.
- `Len()` is the number of buckets held.
- A key's last use is the time of its latest `Allow(key)`, also one that returned false: a
  client that gets 429s is still active.
- `Sweep()` removes every bucket that is both:
  - **idle**: now − last use `>=` idle, and
  - **full**: its count, refilled up to now, equals `burst`. (The count stored at the last call
    is usually lower: refill first, then compare.)

  It returns the number of buckets removed.
- Sweep is not a use. A bucket that Sweep kept is still idle since its last Allow.
- A removed key that comes back gets a new, full bucket.
- With `rate == 0`, a bucket that gave out a token is never full again, so Sweep never removes it.

### Concurrency and precision

- Every method of Limiter and Keyed is safe for concurrent use.
- With a clock that does not move, goroutines that call `AllowN(n)` on a full bucket get exactly
  `burst / n` trues in total, rounded down (99 / 3 = 33, 5 / 2 = 2). Keyed: exactly `burst` trues
  per key, also when the goroutines all hit a new key at the same moment.
- Concurrent `Reserve(1)` calls each get their own slot: the waits are 0 (`burst` times), then
  1/rate, 2/rate, 3/rate, ... Concurrent `Reserve(2)` + `Cancel(2)` pairs leave the bucket full.
- The tests compare `Tokens()` with a tolerance of 1e-9 and waits with a tolerance of 1 µs, so
  float rounding does not matter.

## Examples

A Limiter with rate 4 (one token every 250 ms) and burst 4. The clock starts at 0:

```
clock   call          returns                Tokens() afterwards
0       AllowN(3)     true                   1
0       AllowN(2)     false                  1      all or nothing
0       Allow()       true                   0
125ms   Allow()       false                  0.5
250ms   Allow()       true                   0      0.5 + 0.5 = 1 token, taken
1h      Tokens()      4                      4      capped at burst
1h      Reserve(4)    0s, nil                0
1h      Reserve(1)    250ms, nil             -1
1h      Reserve(2)    750ms, nil             -3     queues behind the first reservation
1h      Allow()       false                  -3
1h      Cancel(2)                            -1
1h      Reserve(5)    0s, ErrExceedsBurst    -1
```

Again rate 4 and burst 4. The Limiter was drained to 0 at 12:00:00, then the clock jumps back
an hour:

```
11:00:00.000   Tokens() = 0    no refill, nothing lost
11:00:00.500   Tokens() = 2    the refill continues from 11:00:00
```

Keyed with rate 1, burst 2 and idle 1 minute:

```
clock   call                  returns
0s      Allow("10.0.0.1")     true
0s      Allow("10.0.0.1")     true
0s      Allow("10.0.0.1")     false    its bucket is empty
0s      Allow("10.0.0.2")     true     another client is not affected
30s     Allow("10.0.0.3")     true
60s     Sweep()               2        .1 and .2: idle for 60s and full again. .3: idle for 30s only
60s     Len()                 1
```

## Run the tests

    go test ./exercises/33-ratelimit/
    go test -race ./exercises/33-ratelimit/
    go test -race -run TestLimiter -v ./exercises/33-ratelimit/    # step 1: New, Allow, AllowN, Tokens
    go test -race -run TestReserve -v ./exercises/33-ratelimit/    # step 2: Reserve and Cancel
    go test -race -run TestKeyed -v ./exercises/33-ratelimit/      # step 3: Keyed

The step filters do not overlap. A fake clock drives every test, so the package runs in about a
second. A "TODO: implement" panic fails only its own test, with the stack. A test still running
after 10 s prints every goroutine's stack and stops the run: most likely a method locks a mutex
that its own goroutine already holds (`sync.Mutex` is not reentrant).

Use `-race`: a missing lock does not always change a count, but the race detector sees it.

## Docs you can use

- [`sync.Mutex`](https://pkg.go.dev/sync#Mutex)
- [`time.Time.Sub`](https://pkg.go.dev/time#Time.Sub), [`time.Duration.Seconds`](https://pkg.go.dev/time#Duration.Seconds),
  and the time package's section [Monotonic Clocks](https://pkg.go.dev/time#hdr-Monotonic_Clocks)
- [`math.IsNaN`](https://pkg.go.dev/math#IsNaN), [`math.IsInf`](https://pkg.go.dev/math#IsInf),
  the built-ins [`min`](https://pkg.go.dev/builtin#min) and [`delete`](https://pkg.go.dev/builtin#delete)
- The spec: [For statements with range clause](https://go.dev/ref/spec#For_range) (what happens
  when you delete during a range)
- [Go maps in action](https://go.dev/blog/maps)
- [The Go Memory Model](https://go.dev/ref/mem), [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate): the limiter most Go code
  uses. Its documentation describes the same model. Read the docs, not the source, until you are done.
- [RFC 6585, section 4: 429 Too Many Requests](https://www.rfc-editor.org/rfc/rfc6585#section-4)

## Stretch goals

- `Wait(ctx, n)`: reserve, then wait with a timer and ctx as in 32-retry. If ctx ends first,
  cancel the reservation and return `ctx.Err()`.
- A precise `Retry-After` for the 429: a Keyed method that says how long until the key's next
  token. Use it in the middleware from `http_test.go`.
- Sharding: Keyed with 16 shards, each a map with its own mutex, the shard picked with
  `hash/maphash`. Write a benchmark with `b.RunParallel` and compare `-cpu 1,4,8`.
- Run Sweep in the background: a `RunSweeper(ctx, k, every)` that the caller starts with `go` and
  stops with ctx. Or a hard cap on the number of keys, with LRU eviction (34-cache builds one).

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 33 hint 1
    go run ./cmd/peek 33 hint 2
    go run ./cmd/peek 33 hint 3
    go run ./cmd/peek 33 explanation
    go run ./cmd/peek 33 walkthrough
    go run ./cmd/peek 33 solution
