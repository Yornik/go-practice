# 07 · Retries with backoff, jitter and context

**Tier 2 · about 2.5 h** · Go: `context` and `select` with a timer, `errors.As` and errors that wrap two errors, generics, float-to-integer overflow, injecting time and randomness · Problem solving: split the pure policy from the loop with effects; list every way the loop can end

During a rollout of the config service, calls to it fail for a few seconds with
`connection refused` or `503`. One retry hides the blip; a naive retry loop causes the next
incident. 200 replicas that sleep a fixed 1 s retry in lockstep and hit the recovering service as
one wave. `time.Sleep` ignores the deadline and SIGTERM. A `400` is retried although it can never
succeed, and `Retry-After: 30` is ignored. You write the retry helper your team's clients share:
capped exponential backoff, full jitter, permanent errors, Retry-After, and a context that stops
it at once.

## Task

Implement the stubs in `retry.go` and `retryafter.go`:

```go
var ErrInvalidPolicy = errors.New("retry: invalid policy") // given

type Policy struct {
	MaxAttempts int           // total calls of op, including the first; >= 1
	Initial     time.Duration // backoff after the 1st failed attempt; > 0
	Max         time.Duration // upper limit for every delay; >= Initial
	Multiplier  float64       // growth per attempt; >= 1
	Jitter      bool          // full jitter: a random delay in [0, backoff)
	Sleep func(ctx context.Context, d time.Duration) error // nil: a real timer that honours ctx
	Rand  func() float64                                    // nil: math/rand/v2; returns [0, 1)
}
func (p Policy) Validate() error                    // nil, or an error that wraps ErrInvalidPolicy
func (p Policy) Backoff(attempt int) time.Duration  // delay after failed attempt n, no jitter

func ParseRetryAfter(v string, now time.Time) (time.Duration, bool) // a Retry-After header value

func Permanent(err error) error                     // marks err as "do not retry"
type RetryAfterError struct{ Err error; After time.Duration } // op asks for a specific delay
func (e *RetryAfterError) Error() string            // "<Err> (retry after <After>)"
func (e *RetryAfterError) Unwrap() error            // Err
type ExhaustedError struct{ Attempts int; Last error }
func (e *ExhaustedError) Error() string             // "giving up after <Attempts> attempts: <Last>"
func (e *ExhaustedError) Unwrap() error             // Last

func Do(ctx context.Context, p Policy, op func(ctx context.Context) error) error
func DoValue[T any](ctx context.Context, p Policy, op func(ctx context.Context) (T, error)) (T, error)
```

Suggested order: the policy, `ParseRetryAfter`, the error types, then the loop. `http_test.go`
shows how a real HTTP client uses all of it.

## Rules

### Policy

- `Validate` returns an error that wraps `ErrInvalidPolicy` when `MaxAttempts < 1`,
  `Initial <= 0`, `Multiplier < 1` or `Max < Initial` (so the zero `Policy` is invalid).
  `MaxAttempts == 1`, `Multiplier == 1` and `Max == Initial` are valid.
- `Backoff(n)` = `Initial × Multiplier^(n-1)`, at most `Max`, truncated to the nanosecond.
  `Backoff(1)` is `Initial`. `Multiplier` may have a fraction (1.5). `Backoff` ignores `Jitter`.
- `Backoff` never overflows: for any attempt number (the tests go up to 100000) and any `Max` up
  to the largest `time.Duration`, the result is between `Initial` and `Max`. One case lands
  exactly on 2^63 ns (`Initial` 2^62 ns × 2), 1 ns above the largest `Duration`: `Backoff(2)` is
  `Max`. An out-of-range float-to-integer conversion gives a CPU-dependent value, so these tests
  catch a mistake there on amd64, not always on arm64 (Apple Silicon).

### ParseRetryAfter

With `now` = Wed, 21 Oct 2026 07:27:00 UTC:

| Header value | Result |
|---|---|
| digits only, a number of seconds: `"120"` | `2m0s, true` |
| an HTTP-date in any of the 3 HTTP/1.1 formats (`net/http` knows them): `"Wed, 21 Oct 2026 07:28:00 GMT"`, `"Wed Oct 21 07:28:00 2026"` | date − now: `1m0s, true` |
| an HTTP-date at or before now: `"Wed, 21 Oct 2026 07:00:00 GMT"` | `0, true` (retry now) |
| more seconds than fit in a `time.Duration`: `"9223372037"` | `0, false` |
| anything else: `""`, `"soon"`, `"-1"`, `"1.5"`, `"120s"`, an RFC 3339 time | `0, false` |

`now` may be in any time zone and have a fraction of a second; the result is exact. The result
is never negative, and `false` always comes with 0 (the optional fuzz test checks both).

### The error types

- `Permanent(nil)` is nil. `Permanent(err)` has `err`'s message and wraps it (`errors.Is` works).
- `RetryAfterError.Error()`: `503 Service Unavailable (retry after 1.5s)`, with `After` formatted
  by `time.Duration.String`. `ExhaustedError.Error()`: `giving up after 3 attempts: connection
  refused`. Both `Unwrap` methods return the wrapped error.

### Do: attempts and delays

- Invalid policy: return the `Validate` error. op and Sleep are never called.
- op is called at most `MaxAttempts` times. op returns nil: return nil at once.
- op returns a permanent error, or an error that wraps one: return at once **the error that was
  passed to `Permanent`**, itself, not wrapped. On the last attempt too: a permanent error
  never becomes an `*ExhaustedError`.
- Any other error: if attempts are left, wait and try again; otherwise return a `*ExhaustedError`
  with `Attempts = MaxAttempts` and `Last` = the last attempt's error exactly as op returned it.
- Do waits by calling `p.Sleep(ctx, delay)` exactly once between two attempts, also when the
  delay is 0, never before the first or after the last. The delay after failed attempt `n`:

| op's error | Delay |
|---|---|
| is or wraps a `*RetryAfterError` (use `errors.As`) | `After`, clamped to `[0, Max]`; no jitter |
| anything else, `Jitter == false` | `Backoff(n)` |
| anything else, `Jitter == true` | `Rand() × Backoff(n)`, truncated to the nanosecond |

- The backoff depends only on the attempt number: a Retry-After delay neither resets it nor holds
  it back.
- Rand is called exactly once per jittered delay and never otherwise. `Rand == nil`: use
  `math/rand/v2`. `Sleep == nil`: use a real timer (see below).

### Do: the context

- op receives `ctx` (or a context derived from it).
- **op is never called once ctx is done.** Check `ctx.Err()` before every attempt yourself: a
  Sleep can return nil although ctx is done (a `select` with a fired timer and a closed
  `ctx.Done()` picks either one).
- ctx done before the first attempt: return an error that matches `ctx.Err()` (`errors.Is`).
- ctx done before a later attempt (it ended during the previous attempt or the sleep): return an
  error that matches **both** `ctx.Err()` and the last attempt's error.
- Sleep returns an error (a custom Sleep may give up for its own reasons, with ctx not done):
  return an error that matches both Sleep's error and the last attempt's error.
- ctx ends during the *last* attempt and op fails: either the `*ExhaustedError` or the combined
  error is fine (untested).
- op's own result counts: if ctx ends while op runs and op returns nil, Do returns nil; if op
  returns a permanent error, Do returns that error.
- The default Sleep waits with a real timer and returns `ctx.Err()` as soon as ctx is done. The
  tests use a 1 hour backoff and cancel ctx: Do must return at once.

### DoValue and concurrency

- `DoValue` follows every rule of `Do`. On success it returns op's value. On **any** error it
  returns the zero value of `T`, even if op returned a value with its error.
- `Do` and `DoValue` may be called from many goroutines at once with the same `Policy`.

## Examples

```
Policy{MaxAttempts: 5, Initial: 100ms, Max: 1s, Multiplier: 2}
Backoff(1), ..., Backoff(6) = 100ms 200ms 400ms 800ms 1s 1s

op: refused, refused, refused, ok      -> sleeps 100ms 200ms 400ms, returns nil
op: refused 5 times                    -> sleeps 100ms 200ms 400ms 800ms, returns
                                          *ExhaustedError "giving up after 5 attempts: ..."
with Jitter: true, Rand: 0.5 0.25 0.75 0
op: refused 5 times                    -> sleeps 50ms 50ms 300ms 0s
op: &RetryAfterError{After: 30s}, ok   -> sleeps 1s (capped at Max), returns nil
op: refused, Permanent(errBadRequest)  -> sleeps 100ms, returns errBadRequest itself

the deadline passes while Do sleeps after attempt 2 (your wording may differ):
err.Error()                              "context deadline exceeded after 2 attempts, last error: connection refused"
errors.Is(err, context.DeadlineExceeded) true
errors.Is(err, errRefused)               true
```

## Run the tests

    go test ./exercises/07-retry/
    go test -race ./exercises/07-retry/
    go test -run TestPolicy -v ./exercises/07-retry/               # step 1: Validate and Backoff
    go test -run ParseRetryAfter -v ./exercises/07-retry/          # step 2: ParseRetryAfter
    go test -run TestErrors -v ./exercises/07-retry/               # step 3: the error types
    go test -race -run TestDo -v ./exercises/07-retry/             # step 4: Do and DoValue
    go test -fuzz=FuzzParseRetryAfter ./exercises/07-retry/        # optional, Ctrl-C to stop

The step filters do not overlap, and a "TODO: implement" panic shows up as a failing test, not a
crash. A fake Sleep records the delays, so the tests take about a second. Use `-race`: only then
does `TestDoConcurrentUse` find an unsafe shared random source.

## Docs you can use

- [`context`](https://pkg.go.dev/context) and [Go Concurrency Patterns: Context](https://go.dev/blog/context)
- [`time.NewTimer`](https://pkg.go.dev/time#NewTimer), [`Timer.Stop`](https://pkg.go.dev/time#Timer.Stop)
- [`errors`](https://pkg.go.dev/errors) (`Is`, `As`, `Unwrap() []error`),
  [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) (more than one `%w`),
  [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- [`math.Pow`](https://pkg.go.dev/math#Pow), [`math/rand/v2`](https://pkg.go.dev/math/rand/v2),
  [`net/http.ParseTime`](https://pkg.go.dev/net/http#ParseTime)
- The spec: [Conversions](https://go.dev/ref/spec#Conversions) ("Conversions between numeric types")
- [RFC 9110, section 10.2.3: Retry-After](https://www.rfc-editor.org/rfc/rfc9110.html#section-10.2.3)
  and [section 5.6.7: Date/Time Formats](https://www.rfc-editor.org/rfc/rfc9110.html#section-5.6.7)

## Stretch goals

- Deadline awareness: if the next attempt cannot start before ctx's deadline, return at once.
- An `OnRetry(attempt, err, delay)` hook to log retries with `log/slog` and count them in a
  Prometheus counter.
- A retry budget per client: retry only while retries are under, say, 10% of recent requests.
- An `http.RoundTripper` that retries idempotent requests on 429, 502, 503 and 504. What happens
  to the request body between attempts (`Request.GetBody`)?

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 07 hint 1
    go run ./cmd/peek 07 hint 2
    go run ./cmd/peek 07 hint 3
    go run ./cmd/peek 07 explanation
    go run ./cmd/peek 07 walkthrough
    go run ./cmd/peek 07 solution
