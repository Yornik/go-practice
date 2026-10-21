# 37 · Worker pool: bounded parallelism, ordered results, fail fast

**Tier 8 · about 2.5 h** · Go: goroutines, `sync.WaitGroup`, a buffered channel as a semaphore, `context.WithCancelCause` and `context.Cause`, `recover` in goroutines, generics · Problem solving: decide who owns and who waits for every goroutine; list every way the work can stop

Before a rollout you run a pre-flight check against 500 targets: a health probe per endpoint, a GET
per ArgoCD application, a TLS handshake per ingress host. One at a time at 200 ms each takes 100 s.
One goroutine per target opens 500 connections at once, hits the API server's rate limit and can
run the process out of file descriptors. What you want is what `xargs -P 16` gives you on the
shell, plus three things it does not: the results in the same order as the input, so you can line
them up with the targets; an early stop when one check fails, because the rollout is off anyway;
and no goroutine left behind, because this code runs inside a long-lived controller where a leak
grows with every reconcile until the pod is OOM-killed. And a nil pointer in one probe must not
crash the whole controller.

## Task

Implement the stubs in `workerpool.go` and `group.go`:

```go
type PanicError struct {
	Value any    // what the task passed to panic
	Stack []byte // stack trace of the goroutine that panicked
}
func (e *PanicError) Error() string // "panic: <Value>"
func (e *PanicError) Unwrap() error // Value if it is an error, else nil

// Map calls fn for every item, at most workers calls at once, returns the results in input
// order and stops at the first failure.
func Map[T, R any](ctx context.Context, in []T, workers int,
	fn func(ctx context.Context, item T) (R, error)) ([]R, error)

type Group struct{ /* you design the fields */ }
func WithContext(ctx context.Context) (*Group, context.Context) // ctx is cancelled on the first error and when Wait returns
func (g *Group) SetLimit(n int)            // at most n functions run at once; n < 0 means no limit
func (g *Group) Go(f func() error)         // run f in a new goroutine; blocks while the limit is reached
func (g *Group) TryGo(f func() error) bool // like Go, but never blocks; reports whether f was started
func (g *Group) Wait() error               // wait for every function; return the first error
```

`Group` is a small version of `golang.org/x/sync/errgroup`, the package most Go code uses for this.
The course uses the standard library only, so you build it yourself. Map and Group need the same
machinery: a limit, waiting, the first error, panics. You can write them in either order, and Map
can be built on top of Group.

## Rules

### PanicError

- `Error()` returns `"panic: "` followed by `Value` formatted with `%v`, for example `panic: boom`.
  It does not include the stack.
- `Unwrap()` returns `Value` if `Value` is an `error`, otherwise nil. Runtime panics (nil pointer
  dereference, write to a nil map, index out of range) are `runtime.Error` values, so
  `errors.As(err, &runtimeErr)` works through a `*PanicError`.

### Map: results

- `workers < 1`: return a nil slice and a non-nil error. fn is never called. Map checks this
  first, so it is an error even for empty input: a bad value is a bug in the caller, and it should
  show up in the first test run, not in the first real rollout.
- On success the result has `len(in)` elements and `result[i]` is what fn returned for `in[i]`,
  whatever order the calls finish in.
- fn is never called twice for the same item. On success it was called exactly once per item.
- Empty or nil `in` (and `workers >= 1`): return an empty, non-nil slice and a nil error. fn is
  never called. This holds even if `ctx` is already done: with no items, no result is missing.

### Map: concurrency

- At most `workers` calls of fn run at the same time.
- Map does use all its workers: with enough items, when the running calls block, exactly
  `workers` calls are running. The tests block calls on purpose and count them.
- `workers` may be larger than `len(in)`.
- The order in which the calls *start* is not specified. Only the order of the results is.
- fn receives a context derived from `ctx`: it has ctx's values and deadline. It is not cancelled
  before a call fails, `ctx` ends or Map returns. fn is called from several goroutines at once.

### Map: fail fast

When a call of fn returns a non-nil error or panics, Map:

1. cancels the context it passes to fn, with that error as the cause. Inside the other calls,
   `context.Cause(ctx)` returns that error and `ctx.Err()` returns `context.Canceled`;
2. starts no new call. With `workers == 1` that means: no call after the failing one;
3. waits until every running call has returned;
4. returns a nil slice and that first error. Errors that come later, such as the
   `context.Canceled` that the cancelled calls return, are dropped.

A panic in fn must not crash the program. Map recovers it in the goroutine where it happened and
treats it as an error of type `*PanicError`: `Value` is the panic value, and `Stack` is
`debug.Stack()` called inside the deferred function that recovers, so the trace contains the
function that panicked.

### Map: ctx ends

- If `ctx` is cancelled or its deadline passes before every item has been processed, Map starts no
  new call, waits for the running calls and returns a nil slice and an error: the first error of a
  failed call if there is one, otherwise `ctx.Err()` (`context.Canceled` or
  `context.DeadlineExceeded`).
- That includes a ctx that is already done when Map is called: fn is then never called.
- It also holds when the running calls ignore ctx and succeed. Items that never ran make the result
  incomplete, so Map returns `ctx.Err()`, never a slice with zero values in the gaps.
- A call that failed first wins, also when `ctx` ends later, while Map waits for the other calls.
  The report must name the real failure, not "context canceled".
- In short: Map returns a nil error only if fn returned nil for every item.

### Map: cleanup

When Map returns:

- no call of fn is still running;
- no goroutine started by Map is still running. The tests count goroutines with
  `runtime.NumGoroutine` and print every goroutine's stack if one is stuck;
- the context passed to fn is cancelled, also on success, so anything fn started with that
  context is told to stop.

### Group

- The zero value (`var g workerpool.Group`) is ready to use: no limit and no context.
- `WithContext(ctx)` returns a new Group and a context derived from ctx (same values, cancelled
  when ctx is). The group cancels that context the first time one of its functions returns an
  error or panics, with that error as the cause, or when Wait returns, whichever comes first. Not
  earlier.
- `SetLimit(n)`: at most n functions run at the same time. `n < 0` means no limit, also after an
  earlier positive limit. Call SetLimit before Go and TryGo. (With `SetLimit(0)` nothing can start;
  changing the limit while functions run is a programming error. The tests check neither.)
- `Go(f)` runs f in a new goroutine. While the limit is reached, Go blocks until a running function
  returns. Without a limit Go never blocks.
- Go starts f even after another function has failed. The group keeps the first error and never
  skips work on its own; f can check the group's context if it wants to skip.
- `TryGo(f)` starts f only if the limit allows it right now, and reports whether it did. It never
  blocks. When it returns false, f does not run. When a running function returns, its slot is free
  again. Without a limit TryGo always starts f.
- TryGo checks for a free slot and takes it in one step. When 20 goroutines call TryGo at the same
  moment and 2 slots are free, exactly 2 calls return true and none of them blocks. (A gap between
  "is a slot free?" and "take it" fails this only now and then; with `-race` it fails much more
  often.)
- `Wait()` blocks until every function started by Go or TryGo has returned. It returns the error
  of the function that failed first, as is (not wrapped), or nil.
- A panic in a function started by Go or TryGo is recovered in that function's goroutine and counts
  as an error of type `*PanicError` (same rules as in Map). It also cancels the group's context.
- Go and TryGo may be called from several goroutines at once, and many functions may fail at the
  same time. Call Wait after your last call to Go or TryGo.

## Examples

```go
addrs, err := workerpool.Map(ctx, []string{"api", "db", "cache"}, 2,
	func(ctx context.Context, name string) (string, error) {
		return name + ".prod.svc.cluster.local:443", nil
	})
// addrs = [api.prod.svc.cluster.local:443 db.prod.svc.cluster.local:443 cache.prod.svc.cluster.local:443]
// err   = nil
```

Fail fast with `workers = 1` and 10 items, where the 4th call fails:

```
call 1 ok, call 2 ok, call 3 ok, call 4 returns "probe failed: connection refused"
Map returns nil, "probe failed: connection refused"     (calls 5 to 10 never happen)
```

Fail fast with `workers = 4`: 3 calls are running and waiting on `<-ctx.Done()` when the 4th call
fails. Map cancels their ctx (cause: the 4th call's error), they return `context.Canceled`, Map
waits for them and returns the 4th call's error, not `context.Canceled`.

A panic:

```go
_, err := workerpool.Map(ctx, ids, 3, func(ctx context.Context, i int) (int, error) {
	if i == 7 {
		panic("probe crashed on item 7")
	}
	return i, nil
})
var pe *workerpool.PanicError
errors.As(err, &pe) // true
pe.Value            // "probe crashed on item 7"
err.Error()         // "panic: probe crashed on item 7"
```

ctx is cancelled during the 2nd call (`workers = 1`, 5 items), and fn ignores ctx and succeeds:

```
Map returns nil, context.Canceled      (not [0 1 0 0 0], nil)
```

A call fails, and while Map waits for a slow call to wind down, the operator presses Ctrl-C
(`workers = 2`):

```
Map returns nil, "probe failed: connection refused"     (not context.Canceled)
```

Group, syncing many applications with at most 10 at a time:

```go
g, ctx := workerpool.WithContext(ctx)
g.SetLimit(10)
for _, app := range apps {
	g.Go(func() error { return syncApp(ctx, app) }) // blocks while 10 syncs run
}
if err := g.Wait(); err != nil { // the first sync error; ctx is cancelled now
	return err
}
```

## Run the tests

    go test ./exercises/37-workerpool/
    go test -race ./exercises/37-workerpool/
    go test -run TestMapStopsAtFirstError -v ./exercises/37-workerpool/

To work on one part at a time, select its tests. You can do the 2 parts in either order, and the
filters do not overlap:

    go test -race -run 'TestPanicError|TestMap' -v ./exercises/37-workerpool/   # part 1: PanicError and Map
    go test -race -run TestGroup -v ./exercises/37-workerpool/                  # part 2: Group

Always use `-race` here: this exercise is about concurrency. The whole package takes a few seconds,
because some tests wait 100 ms on purpose to see that nothing extra happens. If a test reports
"did not return within 5s" or "timed out", something is blocked, and the message says what. A
panic that is not recovered in a goroutine kills the whole test binary: that is exactly the bug the
panic rule is about.

## Docs you can use

- [`sync.WaitGroup`](https://pkg.go.dev/sync#WaitGroup), [`sync.Once`](https://pkg.go.dev/sync#Once),
  [`sync/atomic`](https://pkg.go.dev/sync/atomic)
- [`context.WithCancelCause`](https://pkg.go.dev/context#WithCancelCause),
  [`context.Cause`](https://pkg.go.dev/context#Cause)
- [`runtime/debug.Stack`](https://pkg.go.dev/runtime/debug#Stack),
  [`runtime.NumGoroutine`](https://pkg.go.dev/runtime#NumGoroutine)
- The spec: [Handling panics](https://go.dev/ref/spec#Handling_panics),
  [Select statements](https://go.dev/ref/spec#Select_statements),
  [Send statements](https://go.dev/ref/spec#Send_statements)
- [The Go Memory Model](https://go.dev/ref/mem)
- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines)
- [Go Concurrency Patterns: Context](https://go.dev/blog/context)
- [Defer, Panic, and Recover](https://go.dev/blog/defer-panic-and-recover)
- [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [`golang.org/x/sync/errgroup`](https://pkg.go.dev/golang.org/x/sync/errgroup): the real thing.
  Its documentation describes the same Group API. Read the docs, not the source, until you are done.

## Stretch goals

- `MapAll`: a best-effort variant for reports. It runs every item, never stops early, and returns
  every result plus `errors.Join` of all failures, each wrapped with its index
  (`item 17: connection refused`).
- Per-call timeout: an option that gives each call its own `context.WithTimeout`, so one hung
  endpoint cannot hold a worker forever.
- Streaming input: take an `iter.Seq[T]` instead of a slice, for inputs you cannot hold in memory.
  What happens to "results in input order"? (38-logmerge has the tools.)
- Progress: an `OnDone func(i int, err error)` callback, and an in-flight count you could export
  as a Prometheus gauge.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 37 hint 1
    go run ./cmd/peek 37 hint 2
    go run ./cmd/peek 37 hint 3
    go run ./cmd/peek 37 explanation
    go run ./cmd/peek 37 walkthrough
    go run ./cmd/peek 37 solution
