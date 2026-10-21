# 21 · sleep 300 is not a rollout check

**Tier 5 · about 1.5 h** · Go: channels and `select`, receiving from a closed channel, `context.Context` (`Done`, `Err`, `WithCancel`, `WithDeadline`), `errors.Is` with `context.Canceled` and `context.DeadlineExceeded`, a tick channel as a parameter · Problem solving: list every way the loop can end; apply the checks after each poll in a fixed order; give each ending its own error

`sleep 300`, then `echo deployed`. The deploy pipeline of Vesper Labs, the AI lab in the Spire, has
checked every rollout that way since `inference-api` had 2 pods. It has 24 now. On the first Monday
of December, with sleet on the glass, `hush-bot` shipped a new build. It crashed on start with
`missing env CKPT_STORE_URL`. The pipeline slept for 5 minutes, got exit code 0 from `sleep`, posted
`inference-api deployed`, and everybody went home. At 09:12 the next morning, next to the last of
the release cake that her wife Emiko had sent, Hana Kobayashi, the database engineer, found 6 pods
on the new version, all 6 in `CrashLoopBackOff`; 18 old pods had carried the evening peak alone. The
pipeline counts seconds, not ready replicas, and every build of HUSH, the lab's AI, has gone green
that way for a year. Noor Haddad, the principal SRE, wants the pipeline to watch the rollout, not
the clock. In this exercise you write the wait step: it ends when the rollout is done or stops
moving, when the API keeps failing, or when somebody stops it.

A `Status` holds 3 numbers that a Kubernetes Deployment reports. `Desired` is how many replicas
the new version must have. `Updated` is how many replicas run the new version. `Ready` is how many
replicas pass their readiness probe, old and new version together. `kubectl rollout status` waits
on similar numbers.

## Task

Implement `Wait` in `rollout.go`. The stub gives you `Status`, `Options` and the 3 errors.

```go
type Status struct{ Desired, Updated, Ready int }

var (
	ErrStalled = errors.New("rollout: no progress")
	ErrStopped = errors.New("rollout: ticks stopped")
	ErrInvalid = errors.New("rollout: invalid options")
)

type Options struct {
	Poll       func(ctx context.Context) (Status, error) // required
	Ticks      <-chan time.Time                          // required; 1 poll per tick
	StallTicks int                                       // polls in a row without progress that end Wait; 0: never
	MaxErrors  int                                       // failed polls in a row that Wait tolerates
}

// Wait polls right away and then once per tick, until the rollout is done or Wait gives up.
func Wait(ctx context.Context, opt Options) (Status, error)
```

In production, `Ticks` is the channel of a ticker:

```go
ticker := time.NewTicker(5 * time.Second)
defer ticker.Stop()
status, err := rollout.Wait(ctx, rollout.Options{Poll: readDeployment, Ticks: ticker.C, StallTicks: 60, MaxErrors: 3})
```

The tests send every tick themselves, so they run in microseconds. 32-retry adds a real timer.

## Rules

### Options

- `Poll` and `Ticks` must not be nil. `StallTicks` and `MaxErrors` must not be negative.
- Wait checks the options before anything else. Invalid options return the zero `Status` and an
  error that wraps `ErrInvalid`. Wait does not call `Poll` then.

### The loop

- Wait calls `Poll` right away, without waiting for a tick. After that it waits for a tick, and
  every tick it receives causes exactly 1 poll.
- Wait passes its `ctx` to `Poll` (the same context, or one derived from it).
- A poll **fails** when `Poll` returns a non-nil error. Wait then ignores the `Status` that came
  with the error: that status is never done, never progress and never returned.
- The value of a tick does not matter.

### Progress

- Wait remembers the highest `Updated` and the highest `Ready` of all successful polls so far.
- A successful poll **makes progress** when its `Updated` is higher than the highest `Updated` so
  far, or its `Ready` is higher than the highest `Ready` so far. The first successful poll always
  makes progress.
- A failed poll never makes progress.
- A drop is not progress, and neither is a return to an earlier highest value: `Ready` 2, then 1,
  then 2 again is 1 poll with progress and 2 polls without.
- `Desired` plays no part in progress. A change of `Desired` alone is not progress.

### After each poll

Wait runs these 3 checks in this order. The first one that applies ends Wait.

1. **Errors.** If more than `MaxErrors` polls in a row failed, Wait returns an error that wraps
   the error of the last poll. A successful poll resets this count to 0. With `MaxErrors` 0 the
   first failed poll ends Wait.
2. **Done.** If the poll succeeded with `Updated == Desired` and `Ready == Desired`, Wait returns
   that status and nil. This holds for `Desired == 0` too.
3. **Stall.** If `StallTicks > 0` and the last `StallTicks` polls in a row made no progress, Wait
   returns an error that wraps `ErrStalled`. With `StallTicks` 3, the 3rd poll in a row without
   progress ends Wait, not the 2nd. A poll with progress resets this count to 0; a successful
   poll without progress does not. With `StallTicks` 0, Wait never stalls.

If no check applies, Wait waits for the next tick.

### The context

- Wait checks `ctx` before every poll, the first one included. A `ctx` that is already done, such
  as one whose deadline has passed, ends Wait without a poll.
- While Wait waits for a tick, it also watches `ctx`: a cancel ends the wait at once, without a
  tick.
- **Wait never calls `Poll` once `ctx` is done.** A `select` with a tick that is ready and a
  closed `ctx.Done()` picks either one, so receiving a tick does not mean that `ctx` is still
  live.
- When `ctx` ends Wait, the error wraps `ctx.Err()`: `errors.Is(err, context.Canceled)` or
  `errors.Is(err, context.DeadlineExceeded)` is true.

### How Wait ends

The **last good status** is the `Status` of the last successful poll, or the zero `Status` if no
poll succeeded.

| Ending | Status returned | Error |
|---|---|---|
| done | the status of that poll | nil |
| more than `MaxErrors` failed polls in a row | the last good status | wraps the error of the last poll |
| `StallTicks` polls in a row without progress | the last good status | wraps `ErrStalled` |
| `Ticks` is closed | the last good status | wraps `ErrStopped` |
| `ctx` is done | the last good status | wraps `ctx.Err()` |
| invalid options | the zero `Status` | wraps `ErrInvalid` |

- Wait notices a closed `Ticks` only while it waits for a tick. If the first poll is done, Wait
  returns nil, closed channel or not.
- The wording of the error messages is yours. The tests use only `errors.Is`. A good message
  says what happened, for example `rollout: no progress in 60 polls: 6 of 24 updated, 18 ready`.
- Wait runs in the goroutine of its caller and starts no goroutine that outlives it.

## Examples

`StallTicks` 3, `MaxErrors` 1. The table shows each poll and the state after it:

```text
poll  Poll returns                          highest U/R  failed in a row  without progress
1     {Desired:4 Updated:1 Ready:0}         1/0          0                0     the first good poll is progress
2     {Desired:4 Updated:2 Ready:1}         2/1          0                0     progress
3     error "connection refused"            2/1          1                1     1 failure is tolerated
4     {Desired:4 Updated:2 Ready:0}         2/1          0                2     Ready dropped: no progress
5     {Desired:4 Updated:2 Ready:1}         2/1          0                3     back to 1: no progress
→ Wait returns {Desired:4 Updated:2 Ready:1} and an error that wraps ErrStalled
```

The same options, other polls:

```text
{4 1 0}, {4 3 2}, {4 4 4}                    → {4 4 4}, nil (2 ticks, 3 polls)
{4 1 0}, error A, error B                    → {4 1 0}, an error that wraps B (2 failures in a row)
{4 3 3}, then ctx is canceled before a tick  → {4 3 3}, an error that wraps context.Canceled
a ctx whose deadline is in 1970              → {0 0 0}, an error that wraps context.DeadlineExceeded, no poll
{4 1 0}, then Ticks is closed                → {4 1 0}, an error that wraps ErrStopped
Options{Poll: nil, ...}                      → {0 0 0}, an error that wraps ErrInvalid
```

## Run the tests

    go test ./exercises/21-rollout/
    go test -race ./exercises/21-rollout/
    go test -run TestWaitDone -v ./exercises/21-rollout/        # step 1: poll until done
    go test -run TestWaitStalled -v ./exercises/21-rollout/     # step 2: progress and StallTicks
    go test -run TestWaitErrors -v ./exercises/21-rollout/      # step 3: failed polls and MaxErrors
    go test -run TestWaitContext -v ./exercises/21-rollout/     # step 4: the context
    go test -run TestWaitEnd -v ./exercises/21-rollout/         # step 5: closed ticks, invalid options

Run 1 step at a time; the 5 filters do not overlap. Every test calls Wait in its own goroutine,
with a recover and a 5 s timeout. A "TODO: implement" panic or a Wait that never returns then
fails 1 test with a message, instead of crashing or hanging the run. On a hang, the message shows
where every goroutine is stuck.

The tests script every poll. They send every tick on a channel without a buffer, so a send
succeeds only when Wait receives it. That lets them count the ticks Wait took and the polls it
made, and nothing depends on the speed of your machine. `TestWaitContextCanceledDuringPoll` puts
ticks in a buffered channel instead, so that a tick is ready when `ctx` ends. It repeats that
scenario 20 times.

## Docs you can use

- [The Go spec: Select statements](https://go.dev/ref/spec#Select_statements) and
  [Receive operator](https://go.dev/ref/spec#Receive_operator) (the `v, ok := <-ch` form on a closed
  channel)
- [Effective Go: Channels](https://go.dev/doc/effective_go#channels)
- [`context`](https://pkg.go.dev/context): `Context`, `WithCancel`, `WithDeadline`, `Canceled`,
  `DeadlineExceeded`
- [Go Concurrency Patterns: Context](https://go.dev/blog/context) (Go blog)
- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines) (Go blog)
- [`time.NewTicker`](https://pkg.go.dev/time#NewTicker)
- [`errors.Is`](https://pkg.go.dev/errors#Is) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf)
  with `%w`
- [`kubectl rollout status`](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_rollout/kubectl_rollout_status/)
  and the Deployment's
  [progress deadline](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#progress-deadline-seconds)

## Stretch goals

- Make it a command: `rolloutwait -deployment inference-api -timeout 10m`. Read the status with
  `exec.Command("kubectl", "get", "deployment", ...)` and `-o json`. Use `context.WithTimeout`
  for `-timeout` and `signal.NotifyContext` for Ctrl-C. Exit with 1 on any error.
- Log every change: an `OnPoll func(Status, error)` option that the pipeline uses to print
  `inference-api: 12 of 24 updated, 6 ready`, once per change, not once per tick.
- Measure the stall in time instead of in ticks, as `progressDeadlineSeconds` does: pass a clock
  in, as in 22-alertstate.
- Read which other fields `kubectl rollout status` checks: `.status.observedGeneration`,
  `.status.replicas` (old pods that still run) and `.status.availableReplicas`. Add them to
  `Status` and to the done rule.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 21 hint 1
    go run ./cmd/peek 21 hint 2
    go run ./cmd/peek 21 hint 3
    go run ./cmd/peek 21 explanation
    go run ./cmd/peek 21 walkthrough
    go run ./cmd/peek 21 solution
