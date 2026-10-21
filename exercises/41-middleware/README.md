# 41 · HTTP middleware that operators need

**Tier 9 · about 3 h** · Go: closures as decorators, wrapping `http.ResponseWriter` (the optional-interface problem, `Unwrap`, `http.ResponseController`), context values with an unexported key type, `log/slog`, `recover`, a non-blocking semaphore, `httptest` · Problem solving: reason explicitly about the order of middleware; decide where each piece of state lives

At 04:10, in an autumn storm, Marisol Quispe Mamani, the support lead, answers a customer in
Quechua. The customer replies with a screenshot: a 500 with request ID `edge-7f3a`, which no log
line contains. A sidecar stripped the ID on the way in. You want the request ID from their error
page to find every log line of that request, in every service it passed. You want an access log line
per request with status, size and duration. A nil map in one handler must become a 500 and a stack
trace, not a dropped connection. A 2 GB upload must not run the pod out of memory. And when traffic
doubles, the service must answer the excess with a fast 503 and `Retry-After`, instead of queueing
until every request times out. You write this once, as middleware: functions that wrap an
`http.Handler`, like `chi/middleware`, `gorilla/handlers` or the filters in Envoy and NGINX. Each
one is short. The hard parts are the `ResponseWriter` you must wrap to see the status without
breaking streaming, and the order you stack them in. In this exercise you write these 5 middleware.

## Task

Implement the stubs in `middleware.go`. The type `Middleware` and the constant `RequestIDHeader`
are given.

```go
type Middleware func(http.Handler) http.Handler
func Chain(h http.Handler, m ...Middleware) http.Handler // m[0] is the outermost

const RequestIDHeader = "X-Request-ID"
func RequestID(gen func() string) Middleware   // reuse a valid incoming ID, else gen(); response header; context
func RequestIDFrom(ctx context.Context) string  // the ID RequestID stored, or ""

func AccessLog(l *slog.Logger, now func() time.Time) Middleware // one log record per request
func Recover(l *slog.Logger) Middleware                         // panic -> log record and, if possible, 500
func MaxBytes(n int64) Middleware                               // request bodies of at most n bytes
func LoadShed(maxInFlight int, retryAfter time.Duration) Middleware // over the limit -> 503 at once
```

## Rules

### Chain

- `Chain(h, a, b, c)` is `a(b(c(h)))`: `a` sees the request first and the response last.
- Chain calls each middleware exactly once, when Chain is called, not per request.
- `Chain(h)` with no middleware behaves like `h`.

### RequestID and RequestIDFrom

- A valid incoming `X-Request-ID` is 1 to 128 characters, each one of `A-Z a-z 0-9 . _ -`.
  RequestID reuses a valid incoming ID and does not call `gen`.
- Anything else (no header, empty, too long, a space, a newline, a quote, a slash, non-ASCII...)
  is replaced: RequestID calls `gen` exactly once.
- It sets the ID as the `X-Request-ID` response header **before** it calls the next handler: once
  the handler writes, the header is on the wire.
- The next handler gets a request whose context carries the ID: `RequestIDFrom(r.Context())`.
- `RequestIDFrom` returns `""` for a context without an ID. The context key is of an unexported
  type: a value stored under a plain string key such as `"request_id"` is not the request ID.

### MaxBytes

- A request that declares a length above `n` gets `413 Request Entity Too Large`. The next
  handler is not called. Exactly `n` bytes is allowed. Use `r.ContentLength`: net/http parses
  the `Content-Length` header into it, and `httptest.NewRequest` sets only the field.
- Otherwise the next handler gets the body wrapped with `http.MaxBytesReader`. When the body has
  no declared length (`ContentLength == -1`, as with chunked uploads) and is longer than `n`, the
  handler's Read returns an error of type `*http.MaxBytesError` with `Limit == n`. Answering 413
  then is the handler's job (see the example).
- Bodies of at most `n` bytes, and requests without a body, reach the handler intact: it reads
  exactly what the client sent, without an error.

### LoadShed

- At most `maxInFlight` requests run the next handler at the same time.
- A request over the limit gets `503 Service Unavailable` with the header `Retry-After` set to
  `retryAfter` in whole seconds, **rounded up**: 30s gives `30`, 2s gives `2`, 1.5s gives `2`,
  1ms gives `1`. The next handler is not called.
- LoadShed never waits for a free slot. The answer comes at once, also while the other requests
  are still running.
- Checking for a free slot and taking it is one step. With a gap between the two, 2 requests can
  both see the last free slot: then 1 request too many runs, or 1 waits. `TestLoadShedBurst` and
  `TestLoadShedNeverOverLimit` repeat that race many times, but it shows only now and then: run
  them more than once.
- A slot is free again when the next handler returns, and also when it panics. LoadShed does not
  recover the panic: it passes through.
- The slots belong to the `Middleware` value that LoadShed returns. Wrap two routes with the same
  value, and they share the limit. `maxInFlight` is at least 1 (the tests do not try 0).

### The ResponseWriter wrapper (AccessLog and Recover)

AccessLog and Recover must see what the handler writes, so they pass the next handler a wrapper
around the `ResponseWriter`. The client must get exactly what it would get without the wrapper.

- **Status**: the status the client got. That is the first `WriteHeader` call; or 200 if `Write`
  or `Flush` came first; or 200 if the handler wrote nothing at all. A later `WriteHeader` does
  not change it: net/http ignores it too. (Interim 1xx codes such as 103 Early Hints are out of
  scope: the tests do not send them.)
- **Bytes**: the sum of the `n` that the wrapped `Write` returned. If it writes less than it was
  given, count what it wrote. (net/http refuses a body for a 204 response: `Write` returns
  `0, http.ErrBodyNotAllowed`.)
- **Flush**: the wrapper implements `http.Flusher`. `Flush` flushes the writer underneath and
  sends the header (200 if `WriteHeader` was not called). If the writer underneath cannot flush,
  `Flush` does nothing. It does not panic.
- **Unwrap**: the wrapper has a method `Unwrap() http.ResponseWriter` that returns the writer it
  wraps. Through it, `http.NewResponseController(w).SetWriteDeadline(...)` reaches the real
  writer, also through several wrappers.
- Requests run at the same time. Each log record has the status and bytes of its own request,
  and no request waits for another.

### AccessLog

After the next handler returns, AccessLog logs one record at level INFO with message `request`
and these attributes:

| Key | Type | Value |
|---|---|---|
| `method` | string | `r.Method` |
| `path` | string | `r.URL.Path`, without the query string (it can hold tokens) |
| `status` | int | the status the client got (see above) |
| `bytes` | int64 | the response body bytes written (see above) |
| `duration` | `time.Duration` | `now()` after the handler returned minus `now()` before it was called |
| `request_id` | string | `RequestIDFrom(r.Context())`, `""` if there is none |

- Use `slog.Duration` (the JSON handler writes nanoseconds). `now == nil` means `time.Now`; the
  tests pass a fake clock that the handler moves forward.
- AccessLog does not recover panics: a panic passes through it unchanged.

### Recover

- No panic: the response passes through unchanged and Recover logs nothing.
- A panic before the header was sent: log the record below, then answer
  `500 Internal Server Error`.
- A panic after the header was sent (by `WriteHeader`, `Write` or `Flush`): only log. No second
  `WriteHeader`, and not a single byte added to the body.
- The record: level ERROR, message `panic`, attributes `method`, `path` and `request_id` (as in
  AccessLog, so no query string), `panic` (the panic value formatted with `%v`, a string) and
  `stack` (the stack trace of the goroutine that panicked, as `runtime/debug.Stack` gives it
  inside the deferred function).
- A panic with `http.ErrAbortHandler` is not recovered: Recover panics again with the same value,
  logs nothing and writes nothing. net/http then aborts the response without logging.

### The recommended order

Stack all five in this order, outermost first:

```go
handler := middleware.Chain(mux,
	middleware.RequestID(newID),       // every response and every log line gets the ID
	middleware.AccessLog(logger, nil), // sees the final status, also the 500, 503 and 413 below
	middleware.Recover(logger),        // catches panics in everything below it
	middleware.LoadShed(200, 5*time.Second),
	middleware.MaxBytes(1<<20),
)
```

The `TestStack` tests use this order with `LoadShed(1, 10*time.Second)` and `MaxBytes(1024)`.
With this order every response, including 500, 503 and 413, has `X-Request-ID`; every request has
one access log record with the status the client saw; the panic record and the access record
share the request ID; and on a real `httptest.Server` a streaming handler's `Flush` and
`SetWriteDeadline` reach the connection through the whole stack.

## Examples

A request without an ID, behind the recommended stack, with the JSON handler (the handler took
1.5 s):

```
GET /orders?token=abc
-> 201, X-Request-ID: 7c1e9a0b
{"time":"...","level":"INFO","msg":"request","method":"GET","path":"/orders","status":201,"bytes":9,"duration":1500000000,"request_id":"7c1e9a0b"}
```

The handler panics with a nil map write:

```
GET /stats   (X-Request-ID: edge-7f3a)
-> 500, X-Request-ID: edge-7f3a
{"level":"ERROR","msg":"panic","method":"GET","path":"/stats","request_id":"edge-7f3a","panic":"assignment to entry in nil map","stack":"goroutine 7 [running]:\n..."}
{"level":"INFO","msg":"request",...,"status":500,"bytes":22,...,"request_id":"edge-7f3a"}
```

A handler that reads a body of unknown length behind `MaxBytes(1 << 20)`:

```go
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "body over 1 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "bad JSON", http.StatusBadRequest)
	return
}
```

## Run the tests

    go test ./exercises/41-middleware/
    go test -race ./exercises/41-middleware/
    go test -run TestRequestIDReplacesInvalid -v ./exercises/41-middleware/

One step at a time, in this order (a stub that panics stops the whole test binary, and the
filters do not overlap):

    go test -race -run TestChain ./exercises/41-middleware/       # step 1
    go test -race -run TestRequestID ./exercises/41-middleware/   # step 2
    go test -race -run TestMaxBytes ./exercises/41-middleware/    # step 3
    go test -race -count=5 -run TestLoadShed ./exercises/41-middleware/  # step 4
    go test -race -run TestAccessLog ./exercises/41-middleware/   # step 5 (uses RequestID)
    go test -race -run TestRecover ./exercises/41-middleware/     # step 6
    go test -race -run TestStack ./exercises/41-middleware/       # step 7: everything together

`helpers_test.go` has the `spyWriter` the tests use instead of a real connection. It follows the
rules of the `ResponseWriter` that net/http gives a handler, and records what the client would
see. Read it: it is a compact statement of those rules.

## Docs you can use

- [`net/http`](https://pkg.go.dev/net/http): `Handler`, `HandlerFunc`, `ResponseWriter`,
  `Flusher`, `ResponseController`, `NewResponseController`, `MaxBytesReader`, `MaxBytesError`,
  `ErrAbortHandler`, `Error`, `Request.WithContext`
- [`net/http/httptest`](https://pkg.go.dev/net/http/httptest)
- [`context.WithValue`](https://pkg.go.dev/context#WithValue) (read the paragraph about key types)
- [`log/slog`](https://pkg.go.dev/log/slog): `Logger.LogAttrs`, `slog.String`, `slog.Int`,
  `slog.Duration`, `NewJSONHandler`
- [`runtime/debug.Stack`](https://pkg.go.dev/runtime/debug#Stack)
- [`slices.Backward`](https://pkg.go.dev/slices#Backward)
- The spec: [Handling panics](https://go.dev/ref/spec#Handling_panics),
  [Select statements](https://go.dev/ref/spec#Select_statements),
  [Embedded fields in struct types](https://go.dev/ref/spec#Struct_types)
- [Effective Go: Embedding](https://go.dev/doc/effective_go#embedding)
- [Defer, Panic, and Recover](https://go.dev/blog/defer-panic-and-recover)
- [Structured Logging with slog](https://go.dev/blog/slog)
- [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110): 413 (section 15.5.14), 503 (15.6.4),
  `Retry-After` (10.2.3)

## Stretch goals

- **Abort truncated responses.** After a panic mid-body the client gets a 200 with half a JSON
  document and may think it is complete. Make Recover log and then `panic(http.ErrAbortHandler)`
  when the header was already sent, so net/http cuts the connection. What must AccessLog do then
  to still log the request?
- **Metrics.** A middleware that records `http_requests_total{method,route,code}` and an
  `http_request_duration_seconds` histogram. Why `route` (the `ServeMux` pattern, `r.Pattern`)
  and not `path`? (27-accesslog has the answer.)
- **Per-route limits and priorities.** Shed `/export` before `/healthz`: never shed the probes
  that Kubernetes uses to decide whether to kill the pod (40-healthcheck).
- **Timeouts.** Wrap with `http.TimeoutHandler` and decide where in the order it goes. What does
  it do to `Flush`?

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 41 hint 1
    go run ./cmd/peek 41 hint 2
    go run ./cmd/peek 41 hint 3
    go run ./cmd/peek 41 explanation
    go run ./cmd/peek 41 walkthrough
    go run ./cmd/peek 41 solution
