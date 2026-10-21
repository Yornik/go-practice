# 39 · Circuit breaker: stop hammering a dead dependency

**Tier 8 · about 3.5 h** · Go: an explicit state machine, `sync.Mutex` and how long to hold it, results that arrive after the state changed, `defer` when fn panics, callbacks outside a lock, an injected clock · Problem solving: draw the state diagram and the transition table before you code; walk through what concurrent calls see while the state changes

Your checkout service calls the payments API with a 2 s timeout. Payments goes down. Now every
checkout request waits 2 s for nothing. At 300 requests per second that is 600 requests in flight
at any moment, each holding a goroutine, a connection and a slot in your server's queue. Latency
burns through your SLO, memory grows, and the services that call *you* start timing out too: the
outage spreads up the call graph. Meanwhile payments, trying to come back, gets the full load plus
everybody's retries. A circuit breaker cuts this loop. After a few failures in a row it fails fast,
in microseconds instead of 2 s, so you can serve a fallback. After a pause it lets one or two probe
requests through to find out whether payments is back. Envoy and Istio do something similar per
upstream host (outlier detection); inside a Go service you put a small state machine like this one
in front of each dependency.

## Task

Implement the stubs in `circuitbreaker.go`. `State` with its constants and `String`, the two
errors, `Settings` and `Counts` are given.

```go
type State int // Closed, Open, HalfOpen; String() gives "closed", "open", "half-open"

var ErrOpen         error // Execute rejected the call: the breaker is Open
var ErrTooManyCalls error // Execute rejected the call: HalfOpen, and every probe slot is busy

type Settings struct {
	FailureThreshold int                  // failures in a row in Closed that open the breaker (>= 1)
	OpenTimeout      time.Duration        // how long the breaker stays Open (> 0)
	HalfOpenMaxCalls int                  // probes that may run at the same time in HalfOpen (>= 1)
	SuccessThreshold int                  // probe successes in a row that close the breaker (>= 1)
	IsFailure        func(err error) bool // which errors are failures; nil: every non-nil error
	Now              func() time.Time     // the clock; nil: time.Now
	OnStateChange    func(from, to State) // called once for every state change; may be nil
}

type Counts struct {
	Requests, Successes, Failures, ConsecutiveSuccesses, ConsecutiveFailures uint64
}

type Breaker struct{ /* you design the fields */ }
func New(s Settings) (*Breaker, error)           // a Closed breaker, or an error for bad settings
func (b *Breaker) Execute(fn func() error) error // call fn if the state allows it
func (b *Breaker) State() State                  // the current state
func (b *Breaker) Counts() Counts                // the counts since the last state change
```

These are the only four state changes:

| From | To | When |
|---|---|---|
| Closed | Open | `ConsecutiveFailures` reaches `FailureThreshold` |
| Open | HalfOpen | at least `OpenTimeout` has passed since the breaker opened |
| HalfOpen | Open | a probe fails |
| HalfOpen | Closed | `ConsecutiveSuccesses` reaches `SuccessThreshold` |

## Rules

### New

- `New` returns a nil `*Breaker` and an error if `FailureThreshold < 1`, `OpenTimeout <= 0`,
  `HalfOpenMaxCalls < 1` or `SuccessThreshold < 1`. The error message contains the name of the bad
  field, for example `OpenTimeout`.
- `IsFailure`, `Now` and `OnStateChange` may be nil, with the meaning given in `Settings` above.
- A new breaker is Closed and its counts are all 0.

### Calls and results

- A call is **admitted** when Execute calls fn. Execute then returns fn's error as is, not wrapped.
- A call is **rejected** when Execute returns `ErrOpen` or `ErrTooManyCalls` without calling fn.
  A rejected call changes nothing: no counts, no state change, and it does not make the breaker
  stay Open longer.
- The result of an admitted call is a success or a failure:

| fn | counts as |
|---|---|
| returns nil | success (IsFailure is not called) |
| returns an error, and IsFailure is nil or returns true | failure |
| returns an error, and IsFailure returns false | success: the dependency answered, the request was bad (a 404) |
| panics | failure (IsFailure is not called); Execute then panics with the same value |

### Counts

- `Requests` counts admitted calls. It goes up when the call is admitted, before fn runs, so while
  calls are running, `Requests` is larger than `Successes + Failures`.
- A success adds 1 to `Successes` and `ConsecutiveSuccesses` and sets `ConsecutiveFailures` to 0.
  A failure adds 1 to `Failures` and `ConsecutiveFailures` and sets `ConsecutiveSuccesses` to 0.
- Every state change resets all five counts to 0. In Open they are always 0.

### Closed

- Every call is admitted.
- The failure that makes `ConsecutiveFailures` equal to `FailureThreshold` opens the breaker. With
  threshold 3: fail, fail: still Closed; the third failure in a row: Open. A success in between
  starts the run again.

### Open

- Every call is rejected with `ErrOpen`.
- The breaker opens at the moment its opening failure is recorded: the time `Now()` returns after
  fn has returned, not when fn started.
- Once `Now()` is at least `OpenTimeout` after that moment, the breaker is HalfOpen. There is no
  timer and no goroutine: the next Execute or State call sees that the time has passed and makes
  the change. Execute then treats the call as a HalfOpen call, so it can be admitted as a probe.

### HalfOpen

- At most `HalfOpenMaxCalls` probes run at the same time. A call while that many probes are
  running is rejected with `ErrTooManyCalls`. When a probe returns, its slot is free again.
- A probe failure opens the breaker again. The open timeout starts again at that moment.
- When `ConsecutiveSuccesses` reaches `SuccessThreshold`, the breaker closes.
  `SuccessThreshold` may be larger than `HalfOpenMaxCalls`; the probes then take turns.

### OnStateChange

- It is called once for every state change, with the old and the new state (`from != to`).
- It is called without the breaker's lock held, so it may call State, Counts and Execute. Other
  goroutines can use the breaker while it runs, also while a later change waits for its report.
- Reports never overlap and come in the order of the changes: the report of a later change
  starts only after the report of an earlier one has returned. So a state gauge that the
  callback sets ends on the current state.
- With one goroutine, the report of a change has returned before the Execute or State call that
  made the change returns (or panics). With several goroutines a report may come a moment later,
  but once every call has returned, every change has been reported.
- A change made by a State or Execute call *inside* the callback cannot be reported while the
  callback runs (no overlap). It is reported right after the callback returns.

### Concurrency

- A Breaker is safe for concurrent use.
- Execute holds no lock while fn runs. While fn of one call is running, Execute, State and Counts
  in other goroutines do not wait for it. The HalfOpen tests already rely on this: they start
  probes that block.
- Checking for a free probe slot and taking it is one step. When 50 goroutines call Execute at
  the same moment on a HalfOpen breaker with `HalfOpenMaxCalls` 3, exactly 3 are admitted.
- **Results from an older state are ignored.** Every state change starts a new *generation*. The
  result of a call counts only if the breaker is still in the generation in which the call was
  admitted. Otherwise the result changes nothing: no counts, no state change, and no probe slot
  of the new generation is freed (the call never held one). So a slow probe that fails after
  another probe has closed the breaker does not open it again. Execute still returns fn's error
  to its own caller.
- A panic in fn is recorded before it reaches the caller, so a panicking probe opens the breaker
  and does not keep its slot.

## Examples

`FailureThreshold` 3, `OpenTimeout` 30 s, `HalfOpenMaxCalls` 1, `SuccessThreshold` 2. Counts are
written as Requests/Successes/Failures/ConsecutiveSuccesses/ConsecutiveFailures.

```
time   fn returns    Execute returns  state after  counts after  OnStateChange
00:00  nil           nil              closed       1/1/0/1/0
00:01  503           503              closed       2/1/1/0/1
00:02  503           503              closed       3/1/2/0/2
00:03  503           503              open         0/0/0/0/0     (closed, open)
00:10  (not called)  ErrOpen          open         0/0/0/0/0
00:33  nil           nil              half-open    1/1/0/1/0     (open, half-open)
00:34  nil           nil              closed       0/0/0/0/0     (half-open, closed)
```

At 00:33, 30 s have passed since 00:03, so that Execute changes the state to half-open and then
admits its call as a probe.

A result from an older state, with `FailureThreshold` 1, `OpenTimeout` 10 s,
`HalfOpenMaxCalls` 1 and `SuccessThreshold` 1:

```
00:00  call A is admitted in closed; its fn is slow
00:01  call B fails                      -> open
00:11  State()                           -> half-open
00:12  call A's fn returns a 503         -> Execute returns the 503 to A's caller,
                                            but the breaker ignores it: still half-open, counts 0/0/0/0/0
00:13  call C succeeds as a probe        -> closed
```

Call A says how payments behaved before the breaker opened. It says nothing about the probes.

In a service:

```go
payments, err := circuitbreaker.New(circuitbreaker.Settings{
	FailureThreshold: 5,
	OpenTimeout:      30 * time.Second,
	HalfOpenMaxCalls: 1,
	SuccessThreshold: 2,
	IsFailure: func(err error) bool {
		var se *StatusError
		return !(errors.As(err, &se) && se.Code < 500) // a 4xx: payments answered
	},
	OnStateChange: func(from, to circuitbreaker.State) {
		slog.Warn("payments circuit breaker", "from", from, "to", to)
	},
})
// ...
err = payments.Execute(func() error { return charge(ctx, order) })
if errors.Is(err, circuitbreaker.ErrOpen) || errors.Is(err, circuitbreaker.ErrTooManyCalls) {
	return queueForLater(order) // fail fast and degrade, instead of waiting 2 s for a timeout
}
```

## Run the tests

    go test -race ./exercises/39-circuitbreaker/
    go test -run 'TestNew|TestClosed' -v ./exercises/39-circuitbreaker/          # step 1: New, Closed, Counts, IsFailure
    go test -run TestOpen -v ./exercises/39-circuitbreaker/                      # step 2: Open and the timeout
    go test -race -run TestHalfOpen -v ./exercises/39-circuitbreaker/            # step 3: HalfOpen probes
    go test -race -run TestOnStateChange -v ./exercises/39-circuitbreaker/       # step 4: OnStateChange
    go test -race -run 'TestConcurrent|TestStale|TestPanic' -v ./exercises/39-circuitbreaker/   # step 5

The 5 filters do not overlap. Every call into the breaker runs with a 5 s timeout, so a panic or
a lock that is never released fails one test with a message instead of crashing or hanging the
run. The tests use a fake clock (`Settings.Now`), so the package runs in about a second. Use
`-race` from step 3 on.

## Docs you can use

- [`sync.Mutex`](https://pkg.go.dev/sync#Mutex), [`time.Time.Sub`](https://pkg.go.dev/time#Time.Sub),
  [`errors.Join`](https://pkg.go.dev/errors#Join)
- The spec: [Defer statements](https://go.dev/ref/spec#Defer_statements),
  [Handling panics](https://go.dev/ref/spec#Handling_panics)
- [Defer, Panic, and Recover](https://go.dev/blog/defer-panic-and-recover)
- [The Go Memory Model](https://go.dev/ref/mem), [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [Martin Fowler: CircuitBreaker](https://martinfowler.com/bliki/CircuitBreaker.html): the pattern
  in one page.
- [`github.com/sony/gobreaker`](https://pkg.go.dev/github.com/sony/gobreaker): a widely used Go
  breaker with a similar API. Read its docs, not its source, until you are done.

## Stretch goals

- Return an `*OpenError` with a `RetryAfter` duration (until the breaker turns HalfOpen), so an
  HTTP handler can answer 503 with a `Retry-After` header. 32-retry parses that header.
- A third outcome: `IsExcluded func(error) bool` for errors such as `context.Canceled`, which say
  nothing about the dependency. They count as neither success nor failure, but a probe must still
  give back its slot.
- Open on a failure rate instead of a run of failures: at least 20 calls in the last 10 s and at
  least 50 % of them failed (Hystrix's defaults). Hystrix and resilience4j work this way.
- Metrics: the current state as a gauge and rejected calls as a counter, labelled with the name of
  the dependency, in the Prometheus text format.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 39 hint 1
    go run ./cmd/peek 39 hint 2
    go run ./cmd/peek 39 hint 3
    go run ./cmd/peek 39 explanation
    go run ./cmd/peek 39 walkthrough
    go run ./cmd/peek 39 solution
