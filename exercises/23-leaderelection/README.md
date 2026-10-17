# 23 · Leader election: leases and fencing tokens

**Tier 5 · about 4 h** · Go: functional core and imperative shell, compare-and-swap on a version, context trees (`WithCancel`, `WithTimeout`, `WithoutCancel`), `time.Ticker`, owning a goroutine, failure injection · Problem solving: draw the lease timeline before you code; at every step ask "what if this process freezes right here?"

You run 3 replicas of a controller for availability, but only one may act: two replicas that both
reconcile a Deployment fight over its replica count. Kubernetes solves this with Lease objects:
kube-controller-manager and kube-scheduler run on every control-plane node, only the replica that
holds the lease works, and another takes over within seconds when it dies. You build the same
mechanism: a lease record in a store with compare-and-swap, a pure function that decides what to
write, and the loop that acquires, renews and steps down in time. Then the part most
implementations skip: a leader can freeze (a long GC pause, CPU throttling, a VM live migration)
right after it checked that it leads, and wake up after another replica took over. Only the
resource it writes to can stop it, by rejecting its old **fencing token**.

## Task

Implement the stubs in `lease.go`, `fencing.go` and `leaderelection.go`. The errors are given.

```go
type Lease struct {
	Holder      string        // who holds the lease; "" means nobody
	Epoch       uint64        // +1 on every acquire: the fencing token
	AcquireTime time.Time     // when the holder acquired it
	RenewTime   time.Time     // when the holder last acquired or renewed it
	Duration    time.Duration // the lease is valid until RenewTime + Duration
	Version     int64         // set by the store, for compare-and-swap
}
func Next(cur Lease, id string, now time.Time, d time.Duration) (Lease, bool) // pure: what id writes now

var ErrConflict error
type LeaseStore interface {
	Get(ctx context.Context) (Lease, error)
	Update(ctx context.Context, l Lease) (Lease, error) // compare-and-swap on Version
}
type MemoryStore struct{ /* you design the fields */ }
func NewMemoryStore() *MemoryStore

var ErrStaleToken error
type FencedResource struct{ /* you design the fields */ }    // the zero value is ready to use
func (r *FencedResource) Write(token uint64, v string) error // rejects a token lower than one seen
func (r *FencedResource) Read() (v string, token uint64)     // last value, highest token

var ErrLeaseLost error
type Config struct {
	ID               string
	Store            LeaseStore
	LeaseDuration    time.Duration                           // > RenewDeadline
	RenewDeadline    time.Duration                           // > RetryPeriod
	RetryPeriod      time.Duration                           // > 0
	Now              func() time.Time                        // nil: time.Now
	OnStartedLeading func(ctx context.Context, epoch uint64) // required
	OnStoppedLeading func()                                  // may be nil
	OnNewLeader      func(id string)                         // may be nil
	ReleaseOnCancel  bool
}
func (c Config) Validate() error
func Run(ctx context.Context, c Config) error // one candidate; returns when its leadership ends or ctx is done
```

## Rules

### Next: the pure core

| `cur` | Next returns | ok |
|---|---|---|
| `Holder == id`, also when expired | a **renew**: cur with `RenewTime = now`, `Duration = d`. Holder, Epoch, AcquireTime and Version stay. | true |
| `Holder == ""`, or expired | an **acquire**: `Holder = id`, `Epoch = cur.Epoch + 1`, `AcquireTime = RenewTime = now`, `Duration = d`, `Version = cur.Version` | true |
| another holder, not expired | cur, unchanged | false |

- Expired means `now >= cur.RenewTime + cur.Duration`. The Duration in the lease counts (the holder
  wrote it), not the candidate's d.
- A released lease (`Holder ""`) can be acquired at once, whatever its times say.

### MemoryStore

- A new store returns the zero Lease (`Holder ""`, `Version 0`).
- `Update(ctx, l)`: if `l.Version` equals the stored Version, it stores l with `Version + 1` and
  returns the stored lease. Any other Version, older or newer: an error that wraps `ErrConflict`,
  and nothing changes.
- Compare and store are one atomic step: when 8 goroutines read-modify-write at once, no update is
  lost.
- If ctx is done, Get and Update return `ctx.Err()` and change nothing, like an API client.

### FencedResource

- The zero value is ready: `Read()` returns `""` and 0.
- `Write(token, v)` accepts a token that is `>=` the highest token accepted so far (a leader writes
  many times with the same token), stores v, and remembers token as the highest. A lower token:
  an error that wraps `ErrStaleToken`, and nothing changes.
- Check and store are one atomic step: the highest token never goes down, also with concurrent
  writers. (The test catches a gap between the two reliably only with `-race`, which slows every
  step down.)

### Validate

nil for a valid config; `Now`, `OnStoppedLeading` and `OnNewLeader` may be nil. Otherwise an error
whose message contains the name of every bad field, spelled as in Go (`RetryPeriod`, not
`retry period`):

| Problem | Named field |
|---|---|
| `ID == ""` | `ID` |
| `Store == nil` | `Store` |
| `OnStartedLeading == nil` | `OnStartedLeading` |
| `RetryPeriod <= 0` | `RetryPeriod` |
| `RenewDeadline <= RetryPeriod` | `RenewDeadline` |
| `LeaseDuration <= RenewDeadline` | `LeaseDuration` |

### Run: time

- Run reads the time only through `Now`: the times it writes into a lease, expiry (through Next)
  and the renew deadline. It waits between attempts with real timers of `RetryPeriod`.
- Most tests use a **fake clock** that moves only when the test calls `Advance`. LeaseDuration
  (10 s) and RenewDeadline (5 s) are measured on it, RetryPeriod is 5 ms of real time. So no lease
  expires and no deadline passes unless the test moves the clock, however slow the machine is.
- Durations mean the same on both clocks, instants do not. "2 s left until the deadline", measured
  with `Now`, is a fine real-time timeout; a `time.Time` from `Now` is not a deadline for a ctx.
- One test uses the real clock (`Now` nil): LeaseDuration 500 ms, RenewDeadline 250 ms,
  RetryPeriod 25 ms.

### Run: as a candidate

- An invalid config: Run returns the error of Validate at once, without a store call or a callback.
- The first attempt happens at once, then one every RetryPeriod. An attempt is Get, then
  `Next(cur, ID, Now(), LeaseDuration)`, then, if ok, Update with Next's lease. A store error or
  `ErrConflict` only means "try again at the next attempt"; Run does not return because of it.
- After a successful Update the candidate leads: Run calls `OnStartedLeading(leaderCtx, epoch)` in a
  new goroutine, with the Epoch of the lease it wrote. `leaderCtx` is derived from ctx.
- ctx done while a candidate: Run returns `ctx.Err()`. No store write, no OnStartedLeading, no
  OnStoppedLeading.

### Run: as the leader

- It renews every RetryPeriod: Get, then Update with Next's lease. RenewTime follows `Now`. A leader
  whose renews succeed keeps the lease forever.
- It stops leading:
  1. at once, when a renew attempt reads a lease whose Holder is not its ID (another candidate, an
     operator who edited the lease, a released lease);
  2. when `Now() - RenewTime of its last successful renew >= RenewDeadline`. Until then it retries
     failed renews. A store call that **hangs** must not delay this: no renew attempt may run past
     the deadline. (The tests' hanging store returns only when the call's ctx is done. One test
     makes a's calls hang when 10 ms of a 30 s RenewDeadline are left, then passes the deadline.)
- A candidate leads at most once per call of Run, and it writes nothing after it stopped leading,
  except the release below.

### Run: when leadership ends

1. Cancel `leaderCtx`.
2. Wait until OnStartedLeading has returned. Run owns that goroutine.
3. Call OnStoppedLeading (once; not at all if this candidate never led).
4. Only if ctx was cancelled and ReleaseOnCancel is set: release. Read the lease; if its Holder is
   still this ID, Update it with `Holder ""` and everything else unchanged (the same Epoch);
   otherwise leave it alone. ctx is done by now, and MemoryStore refuses calls with a done ctx.
5. Return `ctx.Err()` after a cancel, or an error that wraps `ErrLeaseLost` when it stopped for
   one of the 2 reasons above.

Without ReleaseOnCancel the lease stays in the store until it expires.

### Callbacks

- `OnNewLeader(id)` is called each time the Get or the successful Update of an attempt shows a
  Holder that differs from the last holder this Run reported, including itself when it acquires.
  Never for `""`. It is called from Run's goroutine, in order. A candidate that sees b, then c,
  then acquires the lease after c released it, reports `[b c a]`. A leader whose renew reads b's
  lease reports b before it steps down: `[a b]`.
- Leadership never overlaps: no candidate's OnStartedLeading is called while another's has not
  returned, as long as callbacks return soon after their ctx is cancelled and no process freezes.
  The tests' OnStartedLeading takes 20 ms to wind down, like a controller that finishes its
  reconcile. Every fake-clock test checks this, except the fencing test (a leader that freezes).

## Examples

The Kubernetes defaults (LeaseDuration 15 s, RenewDeadline 10 s, RetryPeriod 2 s), candidates a
and b, an empty store:

```
time   a                                            b
00:00  Get: empty. Next: acquire. Update ok.        Get: empty. Next: acquire. Update: ErrConflict
       OnNewLeader("a"), OnStartedLeading(ctx, 1)
00:02  renew ok, RenewTime 00:02                    Get: held by a. OnNewLeader("a")
00:04  renew ok, RenewTime 00:04                    Get: held by a (valid until 00:19)
00:05  a's node loses its network: every renew fails
00:14  00:14 - 00:04 = RenewDeadline: cancel ctx,   Get: held by a (valid until 00:19)
       wait, OnStoppedLeading, return ErrLeaseLost
00:20                                               Get: expired at 00:19. Acquire with epoch 2.
                                                    OnNewLeader("b"), OnStartedLeading(ctx, 2)
```

Nobody leads from 00:14 to 00:20. The 5 s up to 00:19 are the safety margin: time for a to stop
its work, and room for a slow clock or a short pause.

Fencing, when a freezes for 20 s instead of losing its network:

```go
var deployment leaderelection.FencedResource
deployment.Write(1, "a: scale web to 3") // nil
// a freezes. Its lease expires, and b takes over with epoch 2.
deployment.Write(2, "b: scale web to 5") // nil
// a wakes up and finishes what it started.
deployment.Write(1, "a: scale web to 4") // ErrStaleToken; deployment keeps "b: scale web to 5"
```

In a controller's `main`:

```go
err := leaderelection.Run(ctx, leaderelection.Config{
	ID:               os.Getenv("POD_NAME"),
	Store:            store,
	LeaseDuration:    15 * time.Second,
	RenewDeadline:    10 * time.Second,
	RetryPeriod:      2 * time.Second,
	OnStartedLeading: func(ctx context.Context, epoch uint64) { runController(ctx, epoch) },
	OnNewLeader:      func(id string) { slog.Info("new leader", "id", id) },
	ReleaseOnCancel:  true, // on SIGTERM, hand over at once instead of after 15 s
})
if errors.Is(err, leaderelection.ErrLeaseLost) {
	os.Exit(1) // let Kubernetes restart the pod; it comes back as a candidate
}
```

## Run the tests

    go test -race ./exercises/23-leaderelection/
    go test -run TestNext -v ./exercises/23-leaderelection/          # step 1: Next
    go test -race -run 'TestMemoryStore|TestFenced|ExampleFenced' -v ./exercises/23-leaderelection/  # step 2: store, fencing
    go test -run TestConfig -v ./exercises/23-leaderelection/        # step 3: Validate
    go test -race -run TestRun -v ./exercises/23-leaderelection/     # step 4: Run, one candidate
    go test -race -run TestElection -v ./exercises/23-leaderelection/  # step 5: several candidates

A panicking stub stops the whole test binary, so run one step at a time; the 5 filters do not
overlap. Steps 4 and 5 use your MemoryStore, so do step 2 first. Before step 4, read
`helpers_test.go`: a `candidate` runs Run in a goroutine and records its callbacks, a `cluster`
holds the shared store and the fake clock, and a `netStore` cuts one candidate off (failing or
hanging). Every wait has a 5 s timeout and says what it waited for.

## Docs you can use

- [`context`](https://pkg.go.dev/context): `WithCancel`, `WithTimeout`,
  [`WithoutCancel`](https://pkg.go.dev/context#WithoutCancel)
- [`time.Ticker`](https://pkg.go.dev/time#Ticker), [`time.Time.Before`](https://pkg.go.dev/time#Time.Before),
  [Monotonic clocks](https://pkg.go.dev/time#hdr-Monotonic_Clocks)
- [`sync.Mutex`](https://pkg.go.dev/sync#Mutex), [`errors.Join`](https://pkg.go.dev/errors#Join)
- [Go Concurrency Patterns: Context](https://go.dev/blog/context)
- [Kubernetes: Leases](https://kubernetes.io/docs/concepts/architecture/leases/)
- [`k8s.io/client-go/tools/leaderelection`](https://pkg.go.dev/k8s.io/client-go/tools/leaderelection):
  the real thing, with the same three durations. Read its docs, not its source, until you are done.
- [Martin Kleppmann: How to do distributed locking](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html):
  the fencing token story.

## Stretch goals

- Tolerate clock skew like client-go: judge expiry by when *this* candidate saw the lease change,
  on its own clock, not by the RenewTime another node wrote.
- Add jitter to RetryPeriod (07-retry) so candidates do not hit the store in lockstep.
- An HTTP LeaseStore with `ETag` and `If-Match` (412 Precondition Failed is `ErrConflict`), tested
  with `httptest`, like `resourceVersion` in the Kubernetes API.
- Metrics: an `is_leader` gauge, a transitions counter, and the seconds since the last successful
  renew (alert on that one).

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 23 hint 1
    go run ./cmd/peek 23 hint 2
    go run ./cmd/peek 23 hint 3
    go run ./cmd/peek 23 explanation
    go run ./cmd/peek 23 walkthrough
    go run ./cmd/peek 23 solution
