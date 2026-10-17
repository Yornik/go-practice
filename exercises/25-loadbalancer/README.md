# 25 · Capstone: a health-aware load balancer

**Tier 5 · about 6 h** · Go: `httputil.ReverseProxy` (`Rewrite`, `ModifyResponse`, `ErrorHandler`), `sync/atomic`, `sync.RWMutex`, goroutines bound to a context, `httptest` · Problem solving: design a small system from a failure-modes table; decide which failures are safe to retry

Every request your users send passes through a load balancer: kube-proxy, an Envoy sidecar, an
NGINX ingress. When a pod is OOM-killed or a deploy rolls, the load balancer decides whether
users see errors, and knobs like `max_fails`, `consecutive_5xx` or `retry_on: connect-failure`
only make sense once you know what happens inside. Here you build that inside: spread requests
over backends, stop sending to a backend that fails health checks (active) or real requests
(passive), retry a request that never reached a backend on another one, and drain a backend
before maintenance without cutting off a download. It combines 08 (time as a value), 12
(goroutine owners), 14 (timed recovery), 15 (draining) and 16 (HTTP handlers).

## Task

Implement the stubs in `strategy.go` and `loadbalancer.go`:

```go
// strategy.go
type Backend struct{ /* you design the fields */ }
func NewBackend(rawURL string) (*Backend, error) // absolute http or https URL with a host
func (b *Backend) URL() *url.URL
func (b *Backend) InFlight() int64 // requests begun and not yet ended
func (b *Backend) Begin()          // InFlight + 1
func (b *Backend) End()            // InFlight - 1

type Strategy interface{ Pick(bs []*Backend) *Backend }
func RoundRobin() Strategy
func LeastConnections() Strategy
func PowerOfTwo(randIntN func(n int) int) Strategy

// loadbalancer.go
type Config struct{ /* see the table */ }
type BackendStatus struct {
	URL                        string
	Healthy, Ejected, Draining bool
	InFlight                   int64
}
var ErrUnknownBackend = errors.New("unknown backend")

type LB struct{ /* you design the fields */ }
func New(c Config) (*LB, error)
func (lb *LB) ServeHTTP(w http.ResponseWriter, r *http.Request) // proxy one request
func (lb *LB) CheckHealth(ctx context.Context)                  // one round of health checks
func (lb *LB) RunHealthChecks(ctx context.Context)              // rounds until ctx is done; blocks
func (lb *LB) Drain(ctx context.Context, backend string) error  // stop new requests, wait for in-flight 0
func (lb *LB) Status() []BackendStatus                          // snapshot, in Config.Backends order
```

| Config field | Meaning | Zero value means |
|---|---|---|
| `Backends []string` | base URLs, like `http://10.0.0.7:8080` | error: at least 1 |
| `Strategy` | picks the backend for each attempt | `RoundRobin()` |
| `HealthPath` | path of the health check | `/healthz` |
| `HealthInterval` | time between two rounds of `RunHealthChecks` | 10s |
| `HealthTimeout` | limit for one health check | 2s |
| `UnhealthyAfter`, `HealthyAfter` | consecutive failed / passed checks to flip | 1 |
| `EjectAfter` | consecutive failed requests that eject a backend | passive ejection off |
| `EjectFor` | how long an ejection lasts | 30s |
| `MaxRetries` | extra attempts after a transport error | no retries |
| `Transport` | sends proxied requests and health checks | `http.DefaultTransport` |
| `Now` | the clock for ejection | `time.Now` |

A backend is **eligible** when it is healthy, not ejected and not draining. Only eligible backends
get new requests.

## Rules

### Backend and strategies

- `NewBackend` returns an error (and nil) unless the URL parses, the scheme is `http` or `https`,
  and the host is not empty. `URL().String()` gives the input back for URLs like the examples.
- `InFlight`, `Begin` and `End` are safe for concurrent use and lose no update.
- `Pick(bs)` gets only eligible backends, in `Config.Backends` order, at least 1. It returns an
  element of `bs`, must not modify `bs` and must be safe for concurrent use.
- **RoundRobin**: the k-th call of Pick (from 0) returns `bs[k % len(bs)]`, also when `len(bs)`
  changes between calls and when many goroutines pick at once (no turn lost or given twice).
  Each `RoundRobin()` has its own counter.
- **LeastConnections**: the backend with the fewest in flight; on a tie, the first in `bs`.
- **PowerOfTwo**: with 1 backend, return it without calling `randIntN`. Otherwise call `randIntN`
  exactly twice: `randIntN(n)` gives the first candidate `bs[i]`; `randIntN(n-1)` gives the second,
  counted among the other n-1 backends in list order (the list without `bs[i]`, not a rotation
  that starts after `bs[i]`). Fewer in flight wins; on a tie, the first candidate. `nil` means
  `math/rand/v2.IntN`.

### New

- Error and a nil `*LB` for: no backends, a URL that `NewBackend` rejects, the same URL string
  twice, any negative number or duration. Zero values get the defaults in the table.
- Backends start healthy, not ejected, not draining. New starts no goroutine.

### Proxying

- `ServeHTTP` builds the list of eligible backends that this request has not tried yet and calls
  `Pick` once per attempt.
- Nothing eligible on the first attempt (every backend unhealthy, ejected or draining): the LB
  answers **503**.
- Proxy with `httputil.ReverseProxy` and a `Rewrite` func that calls `SetURL(backend URL)` and
  `SetXForwarded()`. The backend then sees the client's method, path, query, body and headers, and:
  - `Host`: the backend's own host:port;
  - `X-Forwarded-For`: the client IP only. A value the client sent is dropped: the LB is the edge
    and does not trust it;
  - `X-Forwarded-Host`: the host the client asked for; `X-Forwarded-Proto`: `http`.
- The client gets the backend's response as it is: status (4xx and 5xx too), headers, body.
  A body without `Content-Length` (a stream, such as a chunked download) is passed on as it
  arrives: what the backend flushes reaches the client at once. `ReverseProxy` does this by
  itself; if you wrap `w`, keep `Flush` working (16).
- No response at all (a **transport error**, such as connection refused) and no retry: the LB
  answers **502**. The body of a 502 or 503 is up to you.
- **In flight**: `Begin` when an attempt is sent to a backend, `End` when that attempt is over:
  the whole response body has been copied to the client, or the transport failed. The count is
  exact at every moment and back to 0 after every request. Careful: when a backend breaks the
  connection in the middle of the body, ReverseProxy aborts with `panic(http.ErrAbortHandler)`
  (the `http.Server` recovers it). The count must drop anyway.

### Retries

- Only after a transport error. A 5xx is an answer: it goes to the client and is never retried.
- Only for `GET`, `HEAD` and `OPTIONS`, and only without a body (`ContentLength == 0`). The first
  attempt has already consumed a body.
- Each retry goes to an eligible backend this request has not tried yet. At most `MaxRetries`
  retries; stop earlier when no untried eligible backend is left. Then: **502**.
- No retry when the client's request context is done (the client has gone).

### Passive ejection

- Every attempt counts on its backend: a 5xx response or a transport error is a failure, any
  other response is a success and resets the count to 0. Transport errors that were retried
  count too.
- An attempt that ends while the client's context is done counts as nothing: no failure, no
  success.
- After `EjectAfter` consecutive failures the backend is ejected: not eligible while
  `Now() < time of that failure + EjectFor`, eligible again from that moment. The count starts
  again at 0, so a returning backend needs `EjectAfter` new failures to be ejected again.
- `EjectAfter == 0`: no ejection.

### Active health checks

- `CheckHealth(ctx)` sends one `GET <backend URL><HealthPath>` per backend, through
  `Config.Transport`, all at the same time, each limited by `HealthTimeout`. It returns when all
  checks have finished.
- A 2xx response passes. Any other status, a transport error or a timeout fails.
- A healthy backend becomes unhealthy after `UnhealthyAfter` failed checks in a row; an unhealthy
  one becomes healthy after `HealthyAfter` passed checks in a row. A pass resets the failure
  count, a failure resets the pass count.
- If `ctx` is done when a check ends, its result is ignored: nothing changes.
- `RunHealthChecks(ctx)` runs a round at once, then one every `HealthInterval`. It blocks. When
  `ctx` is done it stops waiting at once (the interval may be 1 h), and returns after the running
  round has finished, even if a check is slow to give up. No check is sent after it returned.

### Drain and Status

- `Drain(ctx, url)`: `url` must equal an entry of `Config.Backends`, otherwise an error that
  matches `ErrUnknownBackend` with `errors.Is`.
- The backend is draining at once: no new request and no retry goes to it, requests in flight
  finish normally. `Healthy` does not change. There is no undrain.
- Drain returns nil as soon as the backend's in-flight count is 0, at once if it is idle. A
  response body that is still being copied keeps the request in flight.
- Once Drain has returned nil, no request reaches the backend, not even one that was inside
  `Pick` when Drain was called: that one counts as in flight (Drain waits for it) or goes
  elsewhere. A test holds a request inside `Pick` with its own Strategy.
- Two Drains of the same backend may wait at the same time; both return when the count is 0.
- If `ctx` ends first, Drain returns `ctx.Err()`. The backend stays draining.
- `Status()` has one entry per backend in `Config.Backends` order: the URL as written, `Healthy`,
  `Ejected` (computed with `Now()`), `Draining`, `InFlight`.

### Concurrency

Every method is safe for concurrent use. In `TestConcurrentTraffic` 16 goroutines send 400
requests while health checks run and the clock moves; every response must come from exactly one
backend, and every in-flight count must end at 0. Run it with `-race`.

## Examples

```go
lb, err := loadbalancer.New(loadbalancer.Config{
	Backends:   []string{"http://10.0.0.7:8080", "http://10.0.0.8:8080"},
	Strategy:   loadbalancer.LeastConnections(),
	HealthPath: "/readyz",
	EjectAfter: 5,
	MaxRetries: 1,
})
go lb.RunHealthChecks(ctx)
log.Fatal(http.ListenAndServe(":8080", lb))
```

PowerOfTwo, in flight `[5 1 3 0]`, `randIntN` returns 2, then 1: first candidate `bs[2]`; the
others are `[bs[0] bs[1] bs[3]]`, number 1 is `bs[1]`; 1 < 3, so `bs[1]` wins. (Counting on from
`bs[2]` would give `bs[0]`: not the rule.)

Retries, backends `[dead, live]`, `MaxRetries: 1`:

```
GET  /items/1             dead: connection refused → retry on live → 200 "live"
POST /items               dead: connection refused → 502 (POST is not retried)
GET  /search + a body     dead: connection refused → 502 (the body is gone)
```

Passive ejection, `EjectAfter: 3`, `EjectFor: 30s`, `bad` answers 500:

```
12:00:00   GET → bad 500                       count 1 of 3
12:00:01   GET → bad 500                       count 2 of 3
12:00:02   GET → bad 500                       ejected until 12:00:02 + 30s = 12:00:32
12:00:02   GET → good 200
12:00:31.999999999                             still ejected
12:00:32   eligible again; GET → bad 500       count 1 of 3, not ejected
```

Health checks, `UnhealthyAfter: 2`, `HealthyAfter: 3`:

```
check status: 200  503  200  503  503  200  200  503  200  200  200
healthy:      yes  yes  yes  yes  no   no   no   no   no   no   yes
```

## Run the tests

    go test ./exercises/25-loadbalancer/
    go test -race ./exercises/25-loadbalancer/
    go test -run TestDrainWaitsForResponseBody -v ./exercises/25-loadbalancer/

Work in steps; each step only needs the ones before it. Add `-race` to each:

    go test -run 'TestBackend|TestStrategy' ./exercises/25-loadbalancer/       # step 1: strategies, no network
    go test -run 'TestNew|TestProxy|TestStatus' ./exercises/25-loadbalancer/   # step 2: proxy, in flight, Status
    go test -run TestRetry ./exercises/25-loadbalancer/                        # step 3: retries
    go test -run TestEject ./exercises/25-loadbalancer/                        # step 4: passive ejection
    go test -run TestHealth ./exercises/25-loadbalancer/                       # step 5: active health checks
    go test -run TestDrain ./exercises/25-loadbalancer/                        # step 6: drain
    go test -run TestConcurrent ./exercises/25-loadbalancer/                   # step 7: everything at once

The fake backends are `httptest` servers; a slow one holds each request until the test releases
it, and a "dead" one refuses every connection. "did not return within 5s" means something is
blocked.

## Docs you can use

- [`httputil.ReverseProxy`](https://pkg.go.dev/net/http/httputil#ReverseProxy) and
  [`ProxyRequest`](https://pkg.go.dev/net/http/httputil#ProxyRequest) (`SetURL`, `SetXForwarded`)
- [`net/http`](https://pkg.go.dev/net/http) (`RoundTripper`, `ErrAbortHandler`),
  [`net/url`](https://pkg.go.dev/net/url) (`Parse`, `URL.JoinPath`),
  [`net/http/httptest`](https://pkg.go.dev/net/http/httptest)
- [`sync/atomic`](https://pkg.go.dev/sync/atomic), [`sync`](https://pkg.go.dev/sync) (`RWMutex`,
  `WaitGroup`), [`context`](https://pkg.go.dev/context), [`time.Ticker`](https://pkg.go.dev/time#Ticker),
  [`math/rand/v2`](https://pkg.go.dev/math/rand/v2)
- [RFC 9110, section 9.2: safe and idempotent methods](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2)
- [Google SRE book: Load Balancing in the Datacenter](https://sre.google/sre-book/load-balancing-datacenter/)

## Stretch goals

- Never eject more than half of the backends, and if fewer than a third are eligible, use all of
  them (Envoy's `max_ejection_percent` and panic threshold).
- A retry budget: retries for at most 20 % of the requests of the last 10 s (08 has the tools).
- Dynamic backends: `SetBackends(urls)`, as an EndpointSlice watch would call it, plus `Undrain`.
- Per-backend metrics in the Prometheus text format (17) on an admin handler.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 25 hint 1
    go run ./cmd/peek 25 hint 2
    go run ./cmd/peek 25 hint 3
    go run ./cmd/peek 25 explanation
    go run ./cmd/peek 25 walkthrough
    go run ./cmd/peek 25 solution
