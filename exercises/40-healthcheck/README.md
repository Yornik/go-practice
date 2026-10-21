# 40 · Health checks: /livez, /readyz and graceful shutdown

**Tier 9 · about 3 h** · Go: `net/http` and Go 1.22 `ServeMux` method patterns, `encoding/json` struct tags, fan-out with a timeout per check, `select` to bound a call that ignores its context, a cache under a mutex, `http.Server.Shutdown` · Problem solving: decide what each probe may depend on; put the shutdown steps in order

Your Go service runs on Kubernetes with 6 replicas, and the kubelet probes two endpoints. `/livez`
asks "is this process stuck?": after 3 failures the kubelet restarts the container. `/readyz` asks
"should this pod get traffic now?": while it fails, the pod gets no traffic from the Service.
Two outages show what goes wrong. First: somebody puts the database ping in the liveness check.
The database has a 2-minute blip, the kubelet restarts all 6 pods at once, they come back cold
and reconnect at the same moment, and a blip becomes a 15-minute outage. Second: every rollout
drops a few hundred requests with `502` or "connection reset", because a pod exits as soon as it
gets SIGTERM, while kube-proxy, the ingress controller and the cloud load balancer still send it
traffic for a few seconds. You write the small library that gets both right: a liveness endpoint
that depends on nothing, a readiness endpoint that runs the dependency checks concurrently with a
hard timeout each, and a shutdown sequence that fails readiness first, keeps serving while the
news spreads, and then lets the in-flight requests finish.

## Task

Implement the stubs in `healthcheck.go` and `serve.go`. The two sentinel errors are given.

```go
type Check struct {
	Name     string                          // unique, not empty
	Critical bool                            // if it fails, the pod is not ready
	Timeout  time.Duration                   // > 0: the most time the check gets
	Fn       func(ctx context.Context) error // nil means healthy
}
type Options struct {
	CacheTTL time.Duration    // reuse a /readyz result this long; <= 0: no cache
	Now      func() time.Time // the clock; nil means time.Now
}
var ErrInvalidCheck, ErrDuplicateCheck error

func New(o Options) *Health
func (h *Health) Register(c Check) error // add a check; validates it
func (h *Health) Handler() http.Handler  // serves GET /livez and GET /readyz
func (h *Health) SetDraining()           // from now on /readyz answers 503 "draining"

type ServeOptions struct {
	DrainDelay      time.Duration       // keep serving this long after SetDraining
	ShutdownTimeout time.Duration       // then wait this long for in-flight requests
	Sleep           func(time.Duration) // waits DrainDelay; nil means time.Sleep
}
// Serve runs srv on ln until ctx is done, then drains and shuts down.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, h *Health, o ServeOptions) error
```

In `main` it looks like this:

```go
h := healthcheck.New(healthcheck.Options{CacheTTL: 2 * time.Second})
err := h.Register(healthcheck.Check{Name: "postgres", Critical: true, Timeout: 300 * time.Millisecond, Fn: db.PingContext})
...
mux := http.NewServeMux()
mux.Handle("/livez", h.Handler())
mux.Handle("/readyz", h.Handler())
mux.Handle("/", app)
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
defer stop()
ln, err := net.Listen("tcp", ":8080")
...
err = healthcheck.Serve(ctx, &http.Server{Handler: mux}, ln, h,
	healthcheck.ServeOptions{DrainDelay: 10 * time.Second, ShutdownTimeout: 15 * time.Second})
```

## Rules

### Register

- An empty Name, a nil Fn or a Timeout <= 0: an error that matches `ErrInvalidCheck` (`errors.Is`).
  Every check needs a timeout: a check without one is how a hung database connection becomes a
  hung probe.
- A Name that is already registered: an error that matches `ErrDuplicateCheck`.
- A rejected check is not added. The check that was there first stays.

### Routes

| Request | Answer |
|---|---|
| `GET /livez` | 200, body `ok` (a trailing newline is fine). It runs no check, never waits for a running one, and stays 200 while draining. |
| `GET /readyz` | the readiness report below |
| another method on `/livez` or `/readyz` | 405, with an `Allow` header that lists `GET`. No check runs. |
| any other path | 404 |

### The readiness report

`Content-Type: application/json`. The body:

```json
{"status": "fail", "checks": [
  {"name": "cache",    "status": "ok",      "duration_ms": 2},
  {"name": "postgres", "status": "timeout", "error": "no answer within 300ms", "duration_ms": 300},
  {"name": "search",   "status": "fail",    "error": "dial tcp 10.0.3.7:9200: connect: connection refused", "duration_ms": 1}
]}
```

- `checks` has one entry per registered check, sorted by name. It is always a JSON array:
  `[]` when there are no checks, never `null`.
- The `status` of a check:

  | status | when |
  |---|---|
  | `ok` | Fn returned nil within Timeout |
  | `timeout` | Fn did not return within Timeout, or it returned an error that matches `context.DeadlineExceeded` (this is what a check that honours its ctx returns) |
  | `fail` | Fn returned any other error, or Fn panicked |

- `error`: for `fail` with an error, exactly `err.Error()`. For `timeout` and for a panic, any
  message that is not empty. For `ok`, empty or left out.
- `duration_ms`: an integer, always present (also when it is 0), the milliseconds from the
  start of the check to its result, measured with `Options.Now` (the tests use a fake clock).
- The top-level `status` and the HTTP code:

  | status | code | when |
  |---|---|---|
  | `draining` | 503 | after SetDraining (see below) |
  | `ok` | 200 | every critical check is `ok`. Also with no checks, or no critical ones. |
  | `fail` | 503 | at least one critical check is `fail` or `timeout` |

- A non-critical check that fails or times out is in the report, but does not change the status.
- Extra fields are allowed.

### Running the checks

- All checks of a run start at the same time. A run takes as long as its slowest check, not the
  sum.
- Each check's ctx has a deadline Timeout after the check starts, and is cancelled when Timeout
  expires (`ctx.Err()` is then `context.DeadlineExceeded`).
- Every check has its own Timeout. A 30 ms timeout on one check does not cut a slower check short.
- A check that ignores its ctx must not hold up the answer: after its Timeout, /readyz reports it
  as `timeout` and moves on. When that check returns later, the goroutine that ran it must exit.
  The test counts goroutines.
- A panic in Fn must not crash the program. It counts as `fail`.
- The client going away (the request's context is cancelled) does not cancel the checks; only
  their Timeout does. A run's result can be shared (see Caching), so one client that hangs up
  must not leave a failure for the next probe. What the handler writes to a client that has gone is
  not tested.

### Caching

- `CacheTTL <= 0`: no result is reused once its run has ended; the next /readyz request runs
  the checks again.
- `CacheTTL > 0`: a result, `ok` or not, is reused while `Now() < finished + CacheTTL`, where
  `finished` is `Options.Now()` at the end of the run that produced it. From
  `finished + CacheTTL` on, the next request runs the checks again.
- Why from the end: otherwise a run slower than the TTL is stale when it ends, and every request
  that waited for it starts a run of its own.
- Requests that arrive while a run is in progress wait for that run and use its result. With
  `CacheTTL > 0` and a clock that does not move, 10 concurrent requests cause 1 run. With
  `CacheTTL <= 0`, sharing the run in progress is allowed, not required.

### Draining

- After SetDraining, /readyz answers 503 with `{"status":"draining","checks":[]}` and runs no
  check, even when a fresh cached result exists. There is no way back.
- SetDraining, /livez and a /readyz that answers `draining` never wait for a run in progress.
- SetDraining runs in Serve's goroutine while probes run in others, so the draining state must
  be safe to write and read at the same time (`-race` checks it).
- A /readyz request that arrived before SetDraining may still answer with check results.

### Serve

Serve runs `srv.Serve(ln)`. When ctx is done, it shuts down in this order:

1. `h.SetDraining()`: /readyz answers 503 from now on.
2. `Sleep(DrainDelay)`. The server keeps accepting and serving everything in the meantime, new
   connections included.
3. `srv.Shutdown` with a context that times out ShutdownTimeout after this call (the drain
   delay does not use it up). It stops accepting and waits for the in-flight requests, which
   complete normally.
4. Return nil. `http.ErrServerClosed` is the normal result of `srv.Serve` after Shutdown, not an
   error.

Other cases:

- In-flight requests still running after ShutdownTimeout: call `srv.Close()` to cut them, and
  return an error that matches `context.DeadlineExceeded`.
- `srv.Serve` fails before ctx is done (for example, the listener is closed): return its error at
  once, so that `errors.Is(err, net.ErrClosed)` still works. Do not wait for ctx, and do not call
  Sleep.
- `ServeOptions.Sleep == nil` means `time.Sleep`.

## Examples

```
postgres (critical) ok, search (non-critical) connection refused
  GET /readyz -> 200, status "ok"; search is listed with status "fail" and its error
postgres (critical, Timeout 300ms) hangs in a driver call that ignores ctx
  GET /readyz -> 503 after about 300ms, status "fail"; postgres has status "timeout"
  GET /livez  -> 200 "ok" at once, also while that run hangs
no checks registered
  GET /readyz -> 200 {"status":"ok","checks":[]}
```

A rollout, with `DrainDelay: 10s` and `ShutdownTimeout: 15s`:

```
t=0s    SIGTERM, ctx done    SetDraining. /readyz -> 503 draining, /livez -> 200
t=0-10s Sleep(10s)           endpoints, ingress and load balancers drop the pod; requests that
                             still arrive are served normally
t=10s   srv.Shutdown         listener closed, idle connections closed, in-flight requests finish
t=12s   last request done    Serve returns nil, main exits 0
t=30s   (SIGKILL)            never reached: terminationGracePeriodSeconds (default 30s) must be
                             longer than DrainDelay + ShutdownTimeout
```

## Run the tests

    go test ./exercises/40-healthcheck/
    go test -race ./exercises/40-healthcheck/
    go test -run 'TestRegister|TestRoutes' -v ./exercises/40-healthcheck/   # step 1: Register, /livez, routing
    go test -run TestReport -v ./exercises/40-healthcheck/                  # step 2: the JSON report
    go test -race -run TestRun -v ./exercises/40-healthcheck/               # step 3: concurrency, timeouts, panics
    go test -race -run TestCache -v ./exercises/40-healthcheck/             # step 4: caching
    go test -race -run TestDraining -v ./exercises/40-healthcheck/          # step 5: SetDraining
    go test -race -run TestServe -v ./exercises/40-healthcheck/             # step 6: Serve

A stub that panics stops the whole test binary, so run only the step you are working on. Use
`-race`: this exercise is about goroutines. If a test says "did not answer within 5s", something
waits for something it should not; the message names what. If the binary dies with
`panic: postgres client is nil`, a panic in a check goroutine was not recovered (`TestRunPanic`).

## Docs you can use

- [`net/http`](https://pkg.go.dev/net/http): `ServeMux` (read "Patterns"), `Server.Shutdown`,
  `Server.Close`, `Server.RegisterOnShutdown`, `ErrServerClosed`
- [`net/http/httptest`](https://pkg.go.dev/net/http/httptest): `NewRecorder`, `NewRequest`
- [`encoding/json`](https://pkg.go.dev/encoding/json): `Marshal` (struct tags, `omitempty`),
  `NewEncoder`
- [`context`](https://pkg.go.dev/context): `WithTimeout`, `WithoutCancel`, `DeadlineExceeded`
- [`sync`](https://pkg.go.dev/sync) and [`sync/atomic`](https://pkg.go.dev/sync/atomic)
- [`os/signal.NotifyContext`](https://pkg.go.dev/os/signal#NotifyContext)
- [Routing Enhancements for Go 1.22](https://go.dev/blog/routing-enhancements)
- [Go Concurrency Patterns: Context](https://go.dev/blog/context)
- Kubernetes: [Configure Liveness, Readiness and Startup Probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/),
  [Pod Lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/) (read
  "Termination of Pods"), [Kubernetes API health endpoints](https://kubernetes.io/docs/reference/using-api/health-checks/)

## Stretch goals

- Write the `main` around it. On a second SIGTERM or Ctrl-C, exit at once.
- `GET /readyz?verbose` with one plain-text line per check, and `GET /readyz/<name>` for one check,
  as the Kubernetes API server does.
- Run the checks in the background every few seconds and serve the last result, so a probe never
  waits. Compare it with the cache: what is stale, and when?
- Export a gauge `health_check_up{check="postgres"}` and a histogram of check durations, so you
  can alert on a flapping dependency before readiness flips.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 40 hint 1
    go run ./cmd/peek 40 hint 2
    go run ./cmd/peek 40 hint 3
    go run ./cmd/peek 40 explanation
    go run ./cmd/peek 40 walkthrough
    go run ./cmd/peek 40 solution
