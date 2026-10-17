# 20 · Singleflight: stop the cache stampede

**Tier 5 · about 3 h** · Go: generics, a map of in-flight work under a `sync.Mutex`, closing a channel to wake every waiter, `recover` and re-panic across goroutines, `context.WithoutCancel`, reference counting · Problem solving: decide who waits on whom; separate the lifetime of the shared work from the lifetime of each caller

Your service caches user profiles for 60 s in front of Postgres, with the cache from 09-cache. The
entry for a popular profile expires at 12:00:00. In the next 50 ms, 1000 requests miss and each
one runs the same 200 ms query. The connection pool (100 connections) runs dry, queries queue up
and take 2 s, requests time out, clients retry, and the retries miss the cache too. This is the
**cache stampede** (or thundering herd), and it turns one expired key into an outage. The fix is
to collapse duplicate work: the first miss runs the query, and the 999 others wait for it and
share its result. This is what `golang.org/x/sync/singleflight` does, and Go's own `net` package
does the same for DNS lookups. Real requests also have deadlines: a client that hangs up must not
cancel the query that 999 others wait for, but a query that nobody waits for any more should stop.

## Task

Implement the stub in `singleflight.go`. `PanicError` is given.

```go
type Group[K comparable, V any] struct{ /* you design the fields */ } // the zero value is ready to use

type Result[V any] struct {
	Val    V
	Err    error
	Shared bool
}

type PanicError struct { // given: Error() and Unwrap() are done
	Value any    // what fn passed to panic
	Stack []byte // stack trace of the goroutine where fn panicked
}

// Do runs fn, or joins the execution in flight for key and returns its result.
func (g *Group[K, V]) Do(key K, fn func() (V, error)) (v V, err error, shared bool)
// DoChan is Do without waiting: the channel receives the result.
func (g *Group[K, V]) DoChan(key K, fn func() (V, error)) <-chan Result[V]
// Forget makes the next call with key start a new execution.
func (g *Group[K, V]) Forget(key K)
// DoContext is Do where each caller waits only as long as its own ctx allows.
func (g *Group[K, V]) DoContext(ctx context.Context, key K,
	fn func(ctx context.Context) (V, error)) (v V, err error, shared bool)
```

Words used below: an **execution** is one run of fn for a key. A **caller** is one call of Do,
DoChan or DoContext. A caller either **starts** an execution (it runs its own fn) or **joins** the
execution in flight for its key (its own fn is never called).

How a cache uses it:

```go
func (s *Server) profile(ctx context.Context, id string) (Profile, error) {
	if p, ok := s.cache.Get(id); ok {
		return p, nil
	}
	p, err, _ := s.loads.DoContext(ctx, id, func(ctx context.Context) (Profile, error) {
		p, err := s.db.LoadProfile(ctx, id)
		if err == nil {
			s.cache.Set(id, p)
		}
		return p, err
	})
	return p, err
}
```

## Rules

### Do

- The zero value of Group is ready to use. A Group is safe for concurrent use.
- A call with a key that has no execution in flight starts one. While it is in flight, every call
  with the same key joins it: it waits and returns the same v and err.
- Do returns exactly what fn returned, v and err, also both together (a partial result with an
  error).
- `shared` is true for every caller of an execution that had more than one caller, the one that
  started it included. A caller that was alone gets false.
- Nothing is cached. The execution leaves the group before any caller receives its result, so a
  call after that starts a new execution. That holds after an error too.
- Keys are independent: an execution in flight for one key never delays a call for another key.
  Do not hold the group's lock while fn runs.
- fn must not call the group with its own key: it would wait for itself. (Not tested.)

### DoChan

- DoChan returns at once with a channel. It never waits for fn.
- But it starts or joins the execution before it returns, not later in the background: a call
  with the same key after that joins the same execution, and the DoChan caller counts as a caller
  from that moment on (for `shared`, and for the reference count of DoContext below).
- The channel receives exactly one `Result` (the values Do would return), after fn has returned,
  never before.
- Do, DoChan and DoContext calls with the same key share one execution.
- The result is delivered even if nobody ever reads the channel. A caller that gives up and stops
  reading must not block the delivery to the other callers, and no goroutine may stay behind.

### Forget

- After `Forget(key)` the next call with key starts a new execution, even while the old one is
  still in flight. Calls after that join the new execution.
- Forget does not stop the old execution. Callers that already wait for it still receive its
  result, and fn's ctx stays alive while one of them waits.
- When the old execution finishes, it must not take the new one out of the group.
- Forget on a key with no execution in flight does nothing.

### Panics in fn

- A panic in fn is recovered in the goroutine that runs fn and becomes a `*PanicError`: `Value`
  is the panic value, `Stack` is `debug.Stack()` called inside the deferred function that
  recovers, so the trace shows the function that panicked.
- Every Do and DoContext caller that waits for that execution panics with that `*PanicError`, in
  its own goroutine. The caller that started it too. No caller is left waiting.
- DoChan callers receive a Result with `Err` set to the `*PanicError` and a zero `Val`. A panic
  must never crash the program from a goroutine that nobody can recover.
- The panic does not stick to the key: the next call starts a new execution.

### DoContext

- A DoContext caller waits until the result arrives or its own ctx is done, whichever comes
  first. If ctx is done first, DoContext returns the zero V, `ctx.Err()` and false at once, even
  if fn is still running and ignores its ctx. (The tests check the error with `errors.Is`.)
- If ctx is already done when DoContext is called, it returns that way at once: it neither
  starts nor joins an execution, so fn is not called.
- fn gets its own ctx. It has the values of the ctx of the caller that started the execution
  (trace IDs, loggers), but no deadline, and it is not cancelled while a caller still waits.
- **Reference count.** fn's ctx is cancelled when the last caller of the execution has left. A
  caller that leaves while others wait changes nothing for the others: they get the result as
  usual. Do and DoChan callers count as callers that never leave.
- When the last caller leaves, the execution also leaves the group, as with Forget: the next call
  starts a new execution right away, even while the old fn still runs. When the old fn returns,
  its result or panic is dropped (no crash), and it does not take a newer execution out of the
  group. A forgotten execution follows the same rules: when its last caller leaves, fn's ctx is
  cancelled, and the newer execution stays in the group.
- fn's ctx is also cancelled once fn has returned.
- `shared` follows the Do rule; callers that left count as callers. A caller that leaves gets
  false.
- No goroutine leaks: once fn has returned and every caller has returned, no goroutine of the
  group is left.

### All together

- Without Forget and without callers that leave, a key never has two executions at the same time.
- No data races under `-race` with all methods called from many goroutines. A caller never
  receives an error that fn returned because its ctx was cancelled while that caller still waited.

## Examples

Five callers, one execution:

```
t=0 ms    A: Do("user:42", load)          starts the execution; load runs
t=5 ms    B: Do("user:42", load)          joins; B's load is never called
t=7 ms    C: DoChan("user:42", load)      joins; returns a channel at once
t=9 ms    D: Do("user:7", load)           another key: runs at once, not delayed by user:42
t=200 ms  load for user:42 returns (p, nil)
          A and B return p, nil, true; C's channel receives {Val:p Err:<nil> Shared:true}
t=210 ms  E: Do("user:42", load)          nothing in flight: starts a new execution
```

A client hangs up, another one stays:

```
t=0 ms    A: DoContext(ctxA, "user:42", load)   starts; load's ctx has ctxA's values, no deadline
t=5 ms    B: DoContext(ctxB, "user:42", load)   joins
t=50 ms   ctxA is cancelled                     A returns "", context.Canceled, false at once
                                                load's ctx stays alive: B still waits
t=200 ms  load returns (p, nil)                 B returns p, nil, true
```

Nobody waits any more:

```
t=0 ms    A: DoContext(ctxA, "user:42", load)   starts
t=50 ms   ctxA is cancelled                     A returns; load's ctx is cancelled and the
                                                execution leaves the group
t=60 ms   C: DoContext(ctxC, "user:42", load)   starts a new execution at once
```

A bug in the loader:

```go
_, _, _ = g.Do("user:42", func() (Profile, error) {
	var p *Profile
	return *p, nil // nil pointer dereference
})
// Do panics in its caller's goroutine, and in every Do caller that joined, with a
// *singleflight.PanicError: Value is the runtime.Error, Stack shows the func literal above.
// A DoChan caller that joined receives Result{Err: that *PanicError}.
```

## Run the tests

    go test -race ./exercises/20-singleflight/
    go test -race -run TestDo -v ./exercises/20-singleflight/        # step 1: Do
    go test -race -run TestChan -v ./exercises/20-singleflight/      # step 2: DoChan
    go test -race -run TestForget -v ./exercises/20-singleflight/    # step 3: Forget
    go test -race -run TestPanic -v ./exercises/20-singleflight/     # step 4: panics
    go test -race -run TestContext -v ./exercises/20-singleflight/   # step 5: DoContext
    go test -race -run TestStress -v ./exercises/20-singleflight/    # step 6: everything at once

The 6 filters do not overlap. Do the steps in order: later tests also call the methods of earlier
steps. Always use `-race`.

How the tests work, so a failure makes sense:

- `helpers_test.go` has a fake backend, `loader`, that blocks until the test releases it and
  ignores its ctx on purpose, like a query that never checks for cancellation.
- A test cannot see the moment a goroutine joins an execution. Where callers must overlap, the
  tests use DoChan, which joins before it returns. Elsewhere they start callers and wait 50 ms. A
  caller that arrives after the execution has finished may rightly start a new one, so this never
  fails a correct solution; if no caller joined, the test tries again with a longer wait.
- Every wait has a 5 s timeout and says what it waited for. The package runs in about 2 s.
- A panic that is not recovered in a goroutine kills the whole test binary. If you see a panic
  and no test results, read the panic rules again.

## Docs you can use

- [`sync.Mutex`](https://pkg.go.dev/sync#Mutex), [`sync.WaitGroup`](https://pkg.go.dev/sync#WaitGroup)
- [`context.WithoutCancel`](https://pkg.go.dev/context#WithoutCancel),
  [`context.WithCancel`](https://pkg.go.dev/context#WithCancel)
- [`runtime/debug.Stack`](https://pkg.go.dev/runtime/debug#Stack)
- The spec: [Close](https://go.dev/ref/spec#Close),
  [Handling panics](https://go.dev/ref/spec#Handling_panics),
  [Type parameter declarations](https://go.dev/ref/spec#Type_parameter_declarations)
- [The Go Memory Model](https://go.dev/ref/mem): what closing a channel guarantees
- [Go Concurrency Patterns: Context](https://go.dev/blog/context)
- [Defer, Panic, and Recover](https://go.dev/blog/defer-panic-and-recover)
- [Tutorial: Getting started with generics](https://go.dev/doc/tutorial/generics)
- [`golang.org/x/sync/singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight): the
  real thing, without DoContext. Its DoChan crashes the program on a panic; yours delivers the
  `*PanicError`. Read the docs, not the source, until you are done.

## Stretch goals

- Stale-while-revalidate: wrap the cache from 09-cache so that an expired entry is still served
  for a grace period while one background DoChan refreshes it. No caller waits for the database.
- Metrics: count calls, executions and joins per group, and the executions in flight, as you
  would export them to Prometheus. The ratio of joins to calls shows how much load you saved.
- A deadline for the shared work: give fn's ctx its own timeout (say 2 s), so one hung query
  cannot hold a key while new callers keep joining it.
- Coalesce across replicas: with 20 replicas a stampede still sends 20 queries. groupcache sends
  each key to one owner replica; a lease (23-leaderelection) is another way.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 20 hint 1
    go run ./cmd/peek 20 hint 2
    go run ./cmd/peek 20 hint 3
    go run ./cmd/peek 20 explanation
    go run ./cmd/peek 20 walkthrough
    go run ./cmd/peek 20 solution
