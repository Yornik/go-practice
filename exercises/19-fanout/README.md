# 19 · Check all 12 at once

**Tier 4 · about 1 h** · Go: goroutines, `sync.WaitGroup`, loop variables in closures (per iteration since Go 1.22), writing to separate slice elements, `defer` and `recover`, a channel that you close, `-race` · Problem solving: decide who waits for whom; make the result independent of the finish order

Seun Adebayo, an SRE on your team, unclips the Key from his ring at 18:00 and clips it onto
yours: the swap board gave you his night, so that he can see his daughter's school play. It is
the third night of Launch Week. At 21:14 `orders`, `payments` and `gateway` hang at the same
time. The status page at `status.vesper-labs.example` gets its colours from a checker written
when Vesper Labs had 3 services. It now probes 12 in sequence, each with a 5 s timeout. The 3 hung
probes wait 5 s each, so every update of the page comes 15 s late. Marisol Quispe Mamani, the
support lead, has 6 tickets that say "I cannot pay" before the page turns red. At 21:20 HUSH, the
on-call AI that the lab sells, answers the probe of its own `hush-api` with `200 OK` and an empty
body. The probe panics with `index out of range [0] with length 0` and takes the whole checker
down, so the page stays red long after `payments` recovers. In this exercise you write the part of
the checker that runs every probe at once, collects every result, and survives a probe that panics.

## Task

Implement the stubs in `fanout.go`. The types and `ErrPanic` are given.

```go
type Check struct {
	Name string       // names need not be unique
	Run  func() error // returns nil when the check passes
}

type Result struct {
	Name string // the Name of the check
	Err  error  // what Run returned, or an error that wraps ErrPanic
}

var ErrPanic = errors.New("fanout: check panicked") // given

func RunAll(checks []Check) []Result      // run all at once, wait, results in the order of checks
func Stream(checks []Check) <-chan Result // run all at once, send each result when it is ready
```

Suggested order: `RunAll` first, then `Stream`.

## Rules

### RunAll

- `RunAll` calls the `Run` function of every check exactly once, each in its own goroutine.
- All checks run at the same time: `RunAll` starts every check before it waits for any of them.
  There is no limit on the number of checks that run at once. The tests start 200.
- `RunAll` returns when every `Run` has returned.
- It returns 1 `Result` per check, in the order of `checks`: result `i` belongs to `checks[i]`,
  whatever order the checks finish in.
- `Result.Name` is the `Name` of the check. `Result.Err` is the error that `Run` returned,
  unchanged: the tests compare it with `==`. It is `nil` when the check passed.
- Names need not be unique. 2 checks with the same name get 2 results, each at its own position.
- For no checks (a nil or an empty slice) `RunAll` returns a slice of length 0. Both nil and an
  empty slice are fine.

### A check that panics

- A panic in `Run` does not stop the program, and `RunAll` does not panic. The panic becomes the
  `Err` of that check.
- That error wraps `ErrPanic`, so `errors.Is(err, ErrPanic)` is true. Its message is
  `fanout: check panicked: ` followed by the panic value formatted with `%v`:

  | `Run` does | `Err.Error()` |
  |---|---|
  | `panic("empty body")` | `fanout: check panicked: empty body` |
  | `panic(io.ErrUnexpectedEOF)` | `fanout: check panicked: unexpected EOF` |
  | `panic(42)` | `fanout: check panicked: 42` |
  | `body[0]` on an empty `body` | `fanout: check panicked: runtime error: index out of range [0] with length 0` |

- A check whose `Run` is nil counts as a check that panics: calling a nil function panics. Its
  `Err` wraps `ErrPanic` and starts with `fanout: check panicked: `. The tests do not check the
  rest of the message.
- The result keeps the `Name` of the check that panicked.
- The other checks are not affected. Their results are the same as without the panic.
- An error that `Run` returns is never changed: it wraps `ErrPanic` only if `Run` wrapped it.

### Stream

- `Stream` starts the checks like `RunAll`: each `Run` exactly once, in its own goroutine, all at
  the same time.
- `Stream` returns the channel at once, while the checks still run. It never returns a nil
  channel.
- It sends the `Result` of each check on the channel when that check's `Run` returns. So the
  results arrive in the order in which the checks finish, not in the order of `checks`. `Name`,
  `Err` and panics follow the rules above.
- The channel has room for every result, so no check waits for a reader. If nobody reads, every
  check still finishes.
- After the last result `Stream` closes the channel, so `for r := range ch` ends after
  `len(checks)` results. For no checks `Stream` closes the channel at once.

### Goroutines

- When `RunAll` returns, every goroutine it started has ended or is about to end. The same holds
  for `Stream` once it has closed the channel, also when nobody has read from the channel yet.
- The tests check this with `runtime.NumGoroutine`: they wait up to 5 s for the count to come back
  to what it was before the call.
- Your code must be free of data races. Run the tests with `-race`. The race detector reports 2
  goroutines that access the same memory without synchronisation when at least 1 of them writes.

### What this exercise leaves out

- A check that never returns makes `RunAll` wait forever and keeps the channel of `Stream` open. A
  timeout per check needs `context` (21-rollout); 40-healthcheck adds it.
- There is no limit on parallelism and no early stop at the first failure. 37-workerpool adds both.

## Examples

```go
checks := []fanout.Check{
	{Name: "checkout", Run: func() error { return nil }},
	{Name: "orders", Run: func() error { return errors.New("GET /healthz: 503 Service Unavailable") }},
	{Name: "hush-api", Run: func() error { panic("empty body") }},
}
for _, r := range fanout.RunAll(checks) {
	if r.Err != nil {
		fmt.Printf("%-12s FAIL %v\n", r.Name, r.Err)
		continue
	}
	fmt.Printf("%-12s ok\n", r.Name)
}
```

```text
checkout     ok
orders       FAIL GET /healthz: 503 Service Unavailable
hush-api     FAIL fanout: check panicked: empty body
```

This is `ExampleRunAll`. The order is always the order of `checks`, even if `hush-api`
finishes first.

`Stream` with 3 checks: `payments` takes 4 s, `orders` hangs for 2 s and then fails, `checkout`
takes 10 ms.

```text
ch := fanout.Stream(checks)   returns at once
<-ch after 10 ms              {checkout <nil>}
<-ch after 2 s                {orders GET /healthz: 503 Service Unavailable}
<-ch after 4 s                {payments <nil>}
<-ch                          the channel is closed: a range loop ends here
```

## Run the tests

    go test -race ./exercises/19-fanout/
    go test -race -run TestRunAllOrder -v ./exercises/19-fanout/        # step 1: results in the order of the checks
    go test -race -run TestRunAllConcurrent -v ./exercises/19-fanout/   # step 2: all at the same time, no goroutine left
    go test -race -run TestRunAllPanic -v ./exercises/19-fanout/        # step 3: a check that panics
    go test -race -run TestStream -v ./exercises/19-fanout/             # step 4: Stream

Run one step at a time; the 4 filters do not overlap. Every test calls `RunAll` or `Stream` in a
goroutine of its own, with a recover and a 5 s timeout. So a "TODO: implement" panic fails 1 test
at once, with a message such as `RunAll panicked: TODO: implement`. A call that never returns
fails 1 test after 5 s. Neither crashes nor hangs the run. The checks in the tests wait on
channels that the test controls. So the test decides the order in which the checks finish, and
nothing depends on the speed of your machine.

Step 1 already needs every check to run at the same time: `TestRunAllOrder` lets the last check
finish first and the first check last. If step 2 fails, its message says how many checks were
running at once when a check waited for the others.

A panic inside the goroutine of a check is different. No recover in the test can catch it, so it
stops the whole test binary until your code recovers it. Step 3 is where you fix that.
`ExampleRunAll` runs only in a plain `go test`. An example cannot recover a panic, so a plain run
crashes until `RunAll` works.

## Docs you can use

- A Tour of Go: [Goroutines](https://go.dev/tour/concurrency/1),
  [Buffered Channels](https://go.dev/tour/concurrency/3) and
  [Range and Close](https://go.dev/tour/concurrency/4)
- [`sync.WaitGroup`](https://pkg.go.dev/sync#WaitGroup), and its example
- [Effective Go: Goroutines](https://go.dev/doc/effective_go#goroutines),
  [Channels](https://go.dev/doc/effective_go#channels) and
  [Recover](https://go.dev/doc/effective_go#recover)
- [Defer, Panic, and Recover](https://go.dev/blog/defer-panic-and-recover) (Go blog)
- [Fixing For Loops in Go 1.22](https://go.dev/blog/loopvar-preview) (Go blog): why each loop
  iteration has its own variable
- The Go spec: [Handling panics](https://go.dev/ref/spec#Handling_panics),
  [Close](https://go.dev/ref/spec#Close),
  [Making slices, maps and channels](https://go.dev/ref/spec#Making_slices_maps_and_channels),
  [Variables](https://go.dev/ref/spec#Variables)
- [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines) (Go blog),
  the sections on fan-out and fan-in
- [`runtime.NumGoroutine`](https://pkg.go.dev/runtime#NumGoroutine)

## Stretch goals

- Add a `Duration` to `Result`. Measure it with an injected clock (`now func() time.Time`), and
  print the 3 slowest checks.
- Turn it into a command. It prints each result as it arrives with `for r := range Stream(checks)`
  and then 1 summary line. Like the check of 15-checkdisk, it exits with a non-zero code if a
  check failed.
- Go 1.25 added `sync.WaitGroup.Go`. Rewrite `RunAll` with it and compare.
- Add the stack of the panic to the error with `runtime/debug.Stack`. 37-workerpool's `PanicError`
  does this.
- Write the generic form: `Map[T, R any](in []T, f func(T) R) []R`. 37-workerpool adds a limit and
  an early stop to it.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 19 hint 1
    go run ./cmd/peek 19 hint 2
    go run ./cmd/peek 19 hint 3
    go run ./cmd/peek 19 explanation
    go run ./cmd/peek 19 walkthrough
    go run ./cmd/peek 19 solution
