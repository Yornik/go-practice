# 46 · Broadcaster: watch streams with slow consumers

**Tier 10 · about 3.5 h** · Go: channels and their close rules, `select` with `default`, `sync.Mutex`, generics, a ring buffer, `sync/atomic` · Problem solving: decide who may send on and who may close each channel; make the invariant hold by construction; reason about interleavings

Lev Seitkali sends noodle soup up for his husband, Daniyar Seitkali, the CTO. Lantern streams
changes to many watchers: revocations to 90 000 agents of HUSH, the runaway AI, rollout events to a
dashboard over server-sent events, config changes to sidecars, sync status to a CLI with `--watch`.
One producer, many consumers. The Kubernetes API server solves the same problem in its watch cache.
Two things go wrong in real life, and HUSH's copies do both on purpose. First, one watcher stops
reading: a laptop with the dashboard open goes to sleep, a client sits behind a full TCP window. If
the producer waits for it, every other watcher stalls. If the producer queues for it without a
limit, memory grows until the pod is OOM-killed. Second, watchers reconnect all the time (network
blips, deploys, load balancer idle timeouts) and want to continue where they stopped, without
missing an event. That works while the server still has the events after the client's last sequence
number. When it does not, the server must say so (Kubernetes answers `410 Gone`), so that the client
re-lists the full state instead of silently missing changes. In this exercise you build that stream.

## Task

Implement the stubs in `broadcaster.go`. `Policy` with its constants and `String`, `Event` and the
3 errors are given.

```go
type Policy int // DropOldest, DropNewest, Disconnect
type Event[T any] struct {
	Seq   uint64 // 1 for the first event, +1 per Publish
	Value T
}
var ErrClosed, ErrSlowConsumer, ErrGone error

func New[T any](history int) *Broadcaster[T]                     // keeps the last `history` events for resume
func (b *Broadcaster[T]) Publish(v T) (uint64, error)            // never waits for a consumer; returns the seq
func (b *Broadcaster[T]) Subscribe(buf int, p Policy) *Subscription[T] // live events only
func (b *Broadcaster[T]) SubscribeFrom(after uint64, buf int, p Policy) (*Subscription[T], error) // replay, then live
func (b *Broadcaster[T]) Close()                                 // ends every subscription
func (b *Broadcaster[T]) Subscribers() int                       // number of active subscriptions

func (s *Subscription[T]) C() <-chan Event[T] // the events; closed when the subscription ends
func (s *Subscription[T]) Unsubscribe()       // ends the subscription
func (s *Subscription[T]) Err() error         // why it ended: nil, ErrSlowConsumer or ErrClosed
func (s *Subscription[T]) Dropped() uint64    // events discarded by DropOldest or DropNewest
```

You design the fields of `Broadcaster` and `Subscription`.

## Rules

### Publish and Subscribe

- `Publish(v)` gives the event the next sequence number and returns it: 1 for the first event, then
  +1 per call. Concurrent calls get distinct numbers with no holes.
- `Subscribe(buf, p)` receives the events published after Subscribe returns, none published before
  it was called (even if they are in the history). Events published while Subscribe runs may or may
  not be included, but nothing in between is skipped.
- Every active subscription receives every event (subject to its own buffer), in `Seq` order and
  without duplicates, also when several goroutines Publish at once. The `Seq` of an event is the
  number `Publish` returned for that value.
- `Subscribers()` is the number of active subscriptions: started and not yet ended.
- While a subscription is active, `Err()` is nil.

### Buffer and policy

The buffer of a subscription is every event delivered to it that the consumer has not received from
`C` yet. It holds at most `buf` events. `buf < 1` counts as 1. When an event arrives and the buffer
is full, the policy decides:

| Policy | What happens | `Dropped()` |
|---|---|---|
| `DropOldest` | the oldest buffered event is discarded, the new one is buffered | +1 |
| `DropNewest` | the new event is discarded | +1 |
| `Disconnect` | the subscription ends with `ErrSlowConsumer`; the new event is not delivered | stays 0 |

- `Publish` never waits for a consumer, however many of them stop reading.
- An event leaves the buffer the moment the consumer receives it from `C`, so the next `Publish`
  finds that room free. A consumer that receives every event before the next `Publish` never
  overflows, even with `buf = 1`.
- `Publish` applies the policy before it returns: right after it, `Dropped()` and `Subscribers()`
  already include its effect.
- A slow subscription does not affect the others or the producer.
- With `DropOldest` and `DropNewest`, every event published while the subscription is active is
  either received from `C` or counted in `Dropped()`. `DropOldest` never discards the event that
  just arrived; `DropNewest` never discards an event that is already buffered.
- With `Disconnect`, the consumer receives a run of events without a gap. To continue, it calls
  `SubscribeFrom` with the last `Seq` it received.

### Ending a subscription

A subscription ends in one of 3 ways. Once it has ended, it gets no new events, it no longer counts
in `Subscribers()`, and `C` is closed **after the events already in the buffer**: the consumer
receives them first, then a `for range sub.C()` loop ends. A consumer blocked in a receive on `C`
wakes up.

| Cause | `Err()` once `C` is closed |
|---|---|
| `sub.Unsubscribe()` | `nil` |
| buffer full with `Disconnect` | `ErrSlowConsumer` |
| `b.Close()` | `ErrClosed` |

- The first cause wins. `Err()` does not change afterwards: Close after a disconnect leaves
  `ErrSlowConsumer`; Close after Unsubscribe leaves `nil`; Unsubscribe after Close leaves `ErrClosed`.
- `Unsubscribe` is idempotent and safe at any time: twice, after a disconnect, after Close, from
  another goroutine, while Publish or Close runs. Publish keeps working for the others.
- `Close` is idempotent. It ends every active subscription. Afterwards `Subscribers()` is 0,
  `Publish` returns `0, ErrClosed` and publishes nothing, and `Subscribe` returns a subscription that
  has already ended (`C` closed, `Err()` = `ErrClosed`).

### Resume with SubscribeFrom

- `SubscribeFrom(after, buf, p)` delivers every event with `Seq > after`: first the ones from the
  history, then live ones, with no gap and no duplicate between them, even while other goroutines
  Publish.
- The broadcaster keeps the last `history` published events (`history <= 0`: none).
- `after` equal to the last published seq (0 before the first Publish) replays nothing; this works
  with any history.
- If an event with `Seq > after` is no longer kept, return nil and an error that wraps `ErrGone`.
- If `after` is larger than the last published seq, also return nil and `ErrGone`. That seq comes
  from another stream, typically from before a restart that began numbering at 1 again.
  Pretending to resume would give the client different events under the same numbers.
- The replayed events go through the subscription's buffer and policy exactly like live events,
  as if they were published at that moment. Replaying events 1 to 10 into a buffer of 3:
  `DropOldest` keeps 8 to 10 and `DropNewest` keeps 1 to 3 (`Dropped()` 7 each); `Disconnect`
  ends the subscription at once, and `C` delivers events 1 to 3 and then closes.
- After Close, `SubscribeFrom` returns nil and `ErrClosed`, whatever `after` is. (`ErrGone` would
  send every client to re-list against a server that is shutting down.)

### Concurrency

- Every method is safe to call from any goroutine, at the same time as any other method,
  including `Err` and `Dropped` while the policy acts.
- No mix of Publish, Subscribe, SubscribeFrom, Unsubscribe and Close may panic (send on a closed
  channel, close of a closed channel) or race. The tests run them all at once.
- If your design starts goroutines, none may be left once Close has returned and every `C` has
  been drained.

## Examples

`buf = 3`, the consumer does not read, events 1 to 10 are published:

```
DropOldest   buffered [8 9 10]   Dropped 7   still active
DropNewest   buffered [1 2 3]    Dropped 7   still active
Disconnect   buffered [1 2 3]    Dropped 0   ended: C delivers 1 2 3, then closes; Err() = ErrSlowConsumer
```

`New(3)`, events 1 to 5 published, so 3, 4 and 5 are kept:

```
SubscribeFrom(5, ...)   ok, replays nothing
SubscribeFrom(2, ...)   ok, replays 3 4 5, then live events
SubscribeFrom(1, ...)   ErrGone   (2 is no longer kept)
SubscribeFrom(0, ...)   ErrGone
SubscribeFrom(6, ...)   ErrGone   (from the future: another stream)
```

A watch client that survives slowness and restarts:

```go
last := relist() // full state from the source of truth, and the seq it matches
for {
	sub, err := b.SubscribeFrom(last, 100, broadcaster.Disconnect)
	if errors.Is(err, broadcaster.ErrGone) {
		last = relist()
		continue
	}
	if err != nil {
		return err // ErrClosed: shutting down
	}
	for ev := range sub.C() {
		apply(ev.Value)
		last = ev.Seq
	}
	if err := sub.Err(); !errors.Is(err, broadcaster.ErrSlowConsumer) {
		return err // nil after Unsubscribe, ErrClosed after Close
	}
	// We were too slow: resume from the last event we applied.
}
```

## Run the tests

    go test ./exercises/46-broadcaster/
    go test -race ./exercises/46-broadcaster/
    go test -run TestSlowDropOldest -v ./exercises/46-broadcaster/

Work in steps; the names are chosen so that these filters do not overlap:

    go test -run TestLive -v ./exercises/46-broadcaster/                 # step 1: New, Publish, Subscribe, C, Err, Dropped, Subscribers
    go test -run TestSlow -v ./exercises/46-broadcaster/                 # step 2: buffers and policies
    go test -run TestEnd -v ./exercises/46-broadcaster/                  # step 3: Unsubscribe and Close
    go test -run TestResume -v ./exercises/46-broadcaster/               # step 4: history and SubscribeFrom
    go test -race -count=5 -run TestConcurrent ./exercises/46-broadcaster/  # step 5: everything at once

Steps 1 to 4 wrap every call, so a missing method fails one test with "panicked: TODO: implement".
Step 5 calls the methods from many goroutines without a wrapper: a panic there (for example "send
on closed channel") kills the test binary and prints the goroutine's stack. Always use `-race` for
step 5; a race-free run once is no proof, so run it several times. The whole package takes a few
seconds.

## Docs you can use

- [Effective Go: Channels](https://go.dev/doc/effective_go#channels)
- The spec: [Channel types](https://go.dev/ref/spec#Channel_types),
  [Send statements](https://go.dev/ref/spec#Send_statements),
  [Receive operator](https://go.dev/ref/spec#Receive_operator),
  [Select statements](https://go.dev/ref/spec#Select_statements),
  [Close](https://go.dev/ref/spec#Close),
  [Type parameter declarations](https://go.dev/ref/spec#Type_parameter_declarations)
- [`sync.Mutex`](https://pkg.go.dev/sync#Mutex), [`sync/atomic`](https://pkg.go.dev/sync/atomic)
- [An Introduction To Generics](https://go.dev/blog/intro-generics)
- [Share Memory By Communicating](https://go.dev/blog/codelab-share)
- [The Go Memory Model](https://go.dev/ref/mem), [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [Kubernetes API concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/): the
  sections "Efficient detection of changes" and "410 Gone responses" describe the contract this
  exercise copies.

## Stretch goals

- An SSE endpoint: an `http.Handler` that writes `id: <seq>` and `data: <value>` per event and
  flushes, resumes from the `Last-Event-ID` request header with SubscribeFrom, answers 410 on
  `ErrGone`, and calls Unsubscribe when the request's context is done.
- Filtered subscriptions (`func(T) bool`). Why does a watcher with a narrow filter now get
  `ErrGone` more often, and how do Kubernetes BOOKMARK events fix that?
- A grace period for `Disconnect`: wait a short, bounded time for room before giving up, as the
  Kubernetes watch cache does. What does that cost the producer and the other watchers?
- Metrics: subscribers, dropped events and disconnects as Prometheus metrics (42-promtext has the
  format).

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 46 hint 1
    go run ./cmd/peek 46 hint 2
    go run ./cmd/peek 46 hint 3
    go run ./cmd/peek 46 explanation
    go run ./cmd/peek 46 walkthrough
    go run ./cmd/peek 46 solution
