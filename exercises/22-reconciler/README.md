# 22 · A Kubernetes-style controller

**Tier 5 · about 4 h** · Go: a blocking queue with `sync.Cond`, sets as maps, `time.AfterFunc`, generics, fakes for failure injection, worker lifecycle with `context` and `sync.WaitGroup` · Problem solving: level-triggered vs edge-triggered; idempotency; testing eventual convergence

Someone runs `kubectl edit`, and seconds later ArgoCD undoes it. A controller logs `the object has
been modified; please apply your changes to the latest version` and carries on. A broken operator
retries one object 50 times a second until the API server throttles everybody. All three come from
the core that every controller shares, from kube-controller-manager to your own operator: a **work
queue** of names; a **reconcile** that compares the desired and the actual state of one name and
takes one step towards the desired one; and a **loop** that runs reconcile on several workers,
retries failures with backoff and re-checks everything now and then. You build that core and prove
that it converges while 30 % of the API calls fail.

## Task

Implement the stubs in `queue.go`, `store.go`, `reconcile.go` and `controller.go`:

```go
// queue.go: the semantics of client-go's workqueue
type Clock interface{ AfterFunc(d time.Duration, f func()) } // given; lets tests control time
type Queue[K comparable] struct{ /* you design the fields */ }
func NewQueue[K comparable](baseDelay, maxDelay time.Duration) *Queue[K] // real time
func NewQueueWithClock[K comparable](baseDelay, maxDelay time.Duration, clock Clock) *Queue[K]
func (q *Queue[K]) Add(k K)                       // queue k, unless it is waiting already
func (q *Queue[K]) Get() (K, bool)                // blocks; false after ShutDown once drained
func (q *Queue[K]) Done(k K)                      // k is no longer processing
func (q *Queue[K]) AddAfter(k K, d time.Duration) // Add once d has passed
func (q *Queue[K]) AddRateLimited(k K)            // Add after the per-key backoff
func (q *Queue[K]) Forget(k K)                    // reset the per-key backoff
func (q *Queue[K]) NumRequeues(k K) int           // AddRateLimited calls since Forget
func (q *Queue[K]) Len() int                      // keys waiting
func (q *Queue[K]) ShutDown()                     // stop accepting keys, wake every Get

// store.go: the API server, with optimistic concurrency
type Object struct{ Name string; Spec map[string]string; ResourceVersion int64 } // given
func (o Object) DeepCopy() Object
var ErrNotFound, ErrConflict, ErrAlreadyExists error // given
type Store interface{ /* Get, List, Create, Update, Delete */ } // given, see store.go
type MemoryStore struct{ /* you design the fields */ }
func NewMemoryStore() *MemoryStore                // and the 5 Store methods

// reconcile.go
type Action int // None, Created, Updated, Deleted (given)
func Reconcile(ctx context.Context, desired map[string]map[string]string, s Store, name string) (Action, error)

// controller.go
type Controller struct {
	Desired func() map[string]map[string]string // the desired state: name -> spec
	Store   Store
	Queue   *Queue[string]
	Workers int
	Resync  time.Duration
}
func (c *Controller) Run(ctx context.Context) error // blocks until ctx is done
```

Work in this order: Queue, store, Reconcile, Run.

## Rules

### Queue

- **Get** removes the oldest waiting key, marks it *processing* and returns `(k, true)`. While no
  key waits, it blocks until one is added or ShutDown is called. Adding a key that is already
  waiting does nothing; it keeps its place. **Len** counts waiting keys only: not processing keys,
  not keys whose AddAfter delay has not passed.
- **Processing** lasts from Get until Done. Adding a processing key does not queue it: the queue
  remembers the Add, and Done then puts the key at the back and wakes a blocked Get. Many Adds
  during processing queue the key once; without an Add, Done does not queue it. Other keys are
  handed out meanwhile. So no key is ever handed to 2 workers at once, and no Add is lost.
- **ShutDown** wakes every blocked Get, not just one. After it, Add, AddAfter and AddRateLimited
  add nothing, and a delayed key that comes due later is not added. Get still returns the keys that
  were waiting, then the zero key and `false` without blocking. Calling ShutDown twice is fine.
- **AddAfter(k, d)** with `d <= 0` is Add(k) now. Otherwise it returns at once, and k is added
  (like Add, so deduplicated) once d has passed on the queue's Clock. `NewQueue` uses real time.
  The tests use a fake clock that moves only when the test calls `Advance`, which runs the due
  functions in the test's goroutine.
- **AddRateLimited(k)** is AddAfter(k, `baseDelay × 2^n`), capped at `maxDelay`, where
  n = NumRequeues(k) before the call. The call raises NumRequeues(k) by 1 and returns at once.
  With `(1s, 1m)` the delays are 1s, 2s, 4s, 8s, 16s, 32s, 1m, 1m. It never overflows: with
  `(5ms, 1000s)` the delay of the 100th call is exactly 1000s.
- **NumRequeues(k)** counts the AddRateLimited calls for k since the last Forget(k); 0 for an
  unknown key. Each key has its own count.
- **Forget(k)** sets the count to 0, so the next AddRateLimited delays by `baseDelay`. That is all
  it does: it does not remove a waiting k or cancel a retry that is already scheduled.
- **Concurrency.** All methods are safe for concurrent use (a test runs 8 workers and 4
  producers).

### Store

- **DeepCopy** returns an object whose Spec is a different map with the same content.
- **Copies.** MemoryStore shares no map with its callers. Changing the map you passed to Create or
  Update, or the Spec of an object that Get, List, Create or Update returned, does not change what
  is stored.
- **Errors** wrap the sentinels; the tests use `errors.Is`. The text is up to you.
- **ResourceVersion.** Each successful Create and Update gives the object a version greater than
  every version the store handed out before, for any object: one counter per store, like etcd's
  revision. So versions are > 0, and 0 is never current. Get returns the version of the last write.
- **Get(name)**: the object, or ErrNotFound.
- **Create(obj)**: ErrAlreadyExists if the name is taken (the stored object stays as it was).
  Otherwise it stores obj with a new version, ignoring the one passed in, and returns what it
  stored.
- **Update(obj)**: ErrNotFound if there is no such object; ErrConflict if `obj.ResourceVersion` is
  not the stored version, also when it is 0 (a "blind write" by a caller that never read the
  object). On error nothing changes. Otherwise it stores obj with a new version and returns what
  it stored. Compare and write are one atomic step: concurrent read-modify-write loops that
  retry on ErrConflict lose no update.
- **Delete(name)** removes the object, whatever its version, or returns ErrNotFound. The name can
  be created again.
- **List** returns every object, sorted by Name; an empty store gives an empty list and nil.
- MemoryStore is safe for concurrent use and ignores ctx.

### Reconcile

A name in `desired` should exist with that spec; a name not in `desired` should not exist.

| desired | in the store | Reconcile does | returns |
|---|---|---|---|
| has name | nothing | Create(name, desired spec) | `Created, nil` |
| has name | same spec | nothing | `None, nil` |
| has name | other spec | Update(the object it read, with the desired spec) | `Updated, nil` |
| no name | an object | Delete(name) | `Deleted, nil` |
| no name | nothing | nothing | `None, nil` |

- It calls Get exactly once and then writes at most once. It touches no other object.
- **Same spec**: the same keys with the same values; nil and an empty map are equal. A name in
  `desired` with a nil spec is desired, with an empty spec.
- **Update** sends the ResourceVersion its Get returned. If someone changed the object in between,
  the store answers ErrConflict and their change survives.
- **No retries inside.** If a store call fails, Reconcile returns `None` and an error that wraps
  the store's error. That includes ErrConflict, and ErrAlreadyExists when someone else created the
  object after the Get. The caller retries.
- A Get that fails with anything but ErrNotFound is an error, not "missing": no write.
- Exception: a Delete that returns ErrNotFound means someone else deleted it. Goal reached:
  `None, nil`.
- **Idempotent.** A second call right after a success returns `None, nil` and writes nothing.

### Controller

- **Config.** Run returns an error at once if Desired, Store or Queue is nil, Workers < 1 or
  Resync <= 0.
- **Queueing.** At the start and then every Resync, Run adds every name in `Desired()` and every
  name from `Store.List`. The List names are how unwanted objects get deleted. If List fails, Run
  still adds the desired names, keeps running and tries List again at the next resync.
- **Workers.** Run starts `Workers` goroutines. Each repeats: Get a name; call
  `Reconcile(ctx, c.Desired(), c.Store, name)` with a fresh `Desired()` each time, so changes are
  seen; on error `AddRateLimited(name)`, on success `Forget(name)`; in both cases `Done(name)`. At
  most Workers reconciles run at once, and with enough names exactly Workers do.
- Other code may call `c.Queue.Add` while Run runs, like a watch event handler.
- **Stop.** Once ctx is done, no new reconcile starts: a worker that still gets a name marks it
  Done without reconciling, and if ctx is done before Run is called, Run writes nothing. Run calls
  `Queue.ShutDown`, waits for every goroutine it started and returns nil. It also waits for a
  worker or a resync that is stuck in a slow store call that ignores ctx: after Run returns,
  nothing calls the store.
- **Convergence.** The last test starts with 10 outdated and 5 unwanted objects, wants 30, and
  fails 30 % of the store calls (seeded `math/rand/v2`). Half of the failed writes are applied
  anyway, like a request that timed out after the server committed it. With 4 workers and extra
  Adds, the store must match within 10 s (well under 1 s is normal), and no name may be in two
  store calls at once.

## Examples

```
Add(web) Add(api) Add(web)   waiting [web api]                      Len 2
Get() -> web, true           waiting [api]   processing {web}       Len 1
Add(web)                     waiting [api]   processing {web}       Len 1   (remembered)
Get() -> api, true           waiting []      processing {web api}   Len 0
Get()                        blocks: web is processing
Done(web)                    waiting [web]; the blocked Get returns web, true
```

A reconcile that loses a race, with `baseDelay = 5ms`:

```
Reconcile(web): Get -> {replicas: 1}, ResourceVersion 7
operator:       kubectl scale -> {replicas: 2}, ResourceVersion 8
Reconcile(web): Update({replicas: 3}, ResourceVersion 7) -> ErrConflict -> None, err
worker:         AddRateLimited(web), Done(web)                      NumRequeues 1
5ms later:      Reconcile(web): Get -> version 8, Update(version 8) -> Updated, nil
worker:         Forget(web), Done(web)                              NumRequeues 0
```

## Run the tests

    go test -race ./exercises/22-reconciler/
    go test -race -run TestQueue -v ./exercises/22-reconciler/                      # step 1: Queue
    go test -race -run 'TestObject|TestMemoryStore' -v ./exercises/22-reconciler/   # step 2: store
    go test -race -run TestReconcile -v ./exercises/22-reconciler/                  # step 3: Reconcile
    go test -race -run TestController -v ./exercises/22-reconciler/                 # step 4: Run

A stub that panics stops the whole test binary, so run only the step you are on. Always use
`-race`. Some tests wait 100 ms on purpose to see that nothing extra happens. "did not return
within 5s" or "timed out" means something is blocked, and the message says what. A test that is
still running after 60 s panics and prints every goroutine's stack: look for a method that waits
for a lock it already holds (`sync.Mutex` is not reentrant).
`helpers_test.go` has the fake clock and `hookStore`, the wrapper the tests use to fail store calls
or to change an object behind your back.

## Docs you can use

- [`sync.Cond`](https://pkg.go.dev/sync#Cond), [`sync.Mutex`](https://pkg.go.dev/sync#Mutex),
  [`sync.WaitGroup`](https://pkg.go.dev/sync#WaitGroup)
- [`time.AfterFunc`](https://pkg.go.dev/time#AfterFunc), [`time.NewTicker`](https://pkg.go.dev/time#NewTicker)
- [`maps`](https://pkg.go.dev/maps), [`slices`](https://pkg.go.dev/slices),
  [`errors.Is`](https://pkg.go.dev/errors#Is), [`context`](https://pkg.go.dev/context)
- The spec: [Index expressions](https://go.dev/ref/spec#Index_expressions) (the comma-ok form for maps)
- [Kubernetes: Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
- [Kubernetes API concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/), section
  "Resource versions"
- [`k8s.io/client-go/util/workqueue`](https://pkg.go.dev/k8s.io/client-go/util/workqueue): the
  real queue. Read the docs, not the source, until you are done.

## Stretch goals

- **Watch instead of poll.** Give the store a `Watch` (21-broadcaster) and call `Queue.Add` per
  event. Resync becomes a safety net that runs every 10 minutes.
- **Metrics.** Queue depth, reconcile duration and retries, like client-go's `workqueue_depth`.
  Which one would you alert on?
- **An overall rate limit.** Per-key backoff does not stop 10 000 failing keys from hammering the
  API. Add a token bucket (08-ratelimit) on top, as client-go does.
- **Prune protection.** If `Desired()` suddenly returns an empty map (a broken Git fetch), Run
  deletes everything. Refuse to delete more than N objects per resync.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 22 hint 1
    go run ./cmd/peek 22 hint 2
    go run ./cmd/peek 22 hint 3
    go run ./cmd/peek 22 explanation
    go run ./cmd/peek 22 walkthrough
    go run ./cmd/peek 22 solution
