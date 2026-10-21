# 22 · Do not page me for a squirrel

**Tier 5 · about 1 h** · Go: an `iota` state type with `String()`, a struct that owns its state behind methods, an injected clock (`func() time.Time`), `time.Duration` comparisons, `switch` on a state · Problem solving: find the exact evaluation at which each state changes; test 1 second before and at every boundary

On 3 December you carry the Owl, the ceramic owl that marks who is on call at Peckish, the
bird-feeder company. At 23:00 the motion-anomaly alert for `hq-window-01`, the demo feeder outside
the office window, pages you. At 23:01 it resolves. At 23:10 it pages again. The cause is Kevin,
the squirrel who lives in the tree next to the feeder: he visits for 40 seconds every 10 minutes,
all night. The monitoring stack evaluates the rule every 30 seconds. The rule fires on 1 true
evaluation and resolves on the next false one. By 07:00 you have 48 pages and 48 all-clears. The
postmortem template has a box "Was it Kevin?", and for once the logs say yes, 48 times. Ines
Okafor, the senior SRE, reads the alert history over coffee: "An alert must be true for a while
before it wakes a human, and quiet for a while before it says all clear." In this exercise you
track the state of 1 alert rule: it waits before it fires, and it waits before it resolves.

## Task

Implement the stubs in `alertstate.go`. The stub gives you the `State` constants, `Rule`,
`Transition` and `ErrInvalidRule`. You write `String`, the `Alert` fields and its functions.

```go
type State int // Inactive, Pending, Firing (the stub gives the constants)
func (s State) String() string // "inactive", "pending", "firing", or "State(N)"

type Rule struct {
	For           time.Duration // true this long before Pending becomes Firing
	KeepFiringFor time.Duration // stay Firing this long after the last true evaluation
}

type Transition struct {
	At       time.Time
	From, To State
}

var ErrInvalidRule = errors.New("alertstate: invalid rule") // in the stub

type Alert struct{ /* you design the fields */ }
func New(r Rule, now func() time.Time) (*Alert, error) // now nil means time.Now
func (a *Alert) Eval(active bool) State                // 1 evaluation at now()
func (a *Alert) State() State                          // the current state
func (a *Alert) ActiveAt() time.Time                   // start of the current episode; zero when Inactive
func (a *Alert) History() []Transition                 // every change, oldest first, a copy
```

The monitoring system calls `Eval` once per evaluation interval. `active` is the result of the
alert condition at that moment, for example "the motion rate is above 5 per minute".

## Rules

### State

- `String()` returns `inactive`, `pending` and `firing` for the 3 constants, and `State(N)` for
  any other value, for example `State(-1)` or `State(3)`. `fmt` uses it: `fmt.Sprint(Firing)` is
  `firing`.
- The given constants make `Inactive` the zero value of `State`.

### New

- `New` returns an `Alert` in the state `Inactive`, with the zero `ActiveAt()` and an empty
  `History()`, and a nil error. `For` 0 and `KeepFiringFor` 0 are valid.
- `now` is the clock that `Eval` reads. A nil `now` means `time.Now`.
- A negative `For` or `KeepFiringFor` returns a nil `*Alert` and an error that wraps
  `ErrInvalidRule` (`errors.Is` works).

### Eval

`Eval(active)` reads the clock once: call that time `now`. It changes the state by this table
and returns the new state. `State()` returns the same value until the next evaluation.

| State | `active` | New state |
|---|---|---|
| Inactive | false | Inactive |
| Inactive | true | Pending, and `ActiveAt` becomes `now`. When `For` is 0: Firing at once, and `ActiveAt` becomes `now`. |
| Pending | false | Inactive |
| Pending | true | Firing when `now − ActiveAt >= For`, else Pending |
| Firing | true | Firing |
| Firing | false | Inactive when `now − lastActive >= KeepFiringFor`, else Firing |

- `lastActive` is the time of the latest evaluation with `active` true, whatever the state was
  at that evaluation. The evaluation that makes an alert Firing is always one of them.
- Both comparisons use `>=`: with `For` 2m, an alert that became Pending at 00:30 is Firing at
  an evaluation at 02:30, and still Pending at 02:29. The same holds for `KeepFiringFor`.
- The state changes only at an evaluation. A Pending alert whose `For` has passed stays Pending
  until the next evaluation; if that one is false, it goes to Inactive, not to Firing.
- Firing with `active` true stays Firing: `For` does not count again while the alert fires.
- With `KeepFiringFor` 0 a Firing alert resolves at the first false evaluation.
- **A clock that moves backwards.** `Eval` ignores an evaluation whose `now` is before the
  latest evaluation that it did not ignore. An ignored evaluation changes nothing: not the
  state, not `ActiveAt`, not `lastActive`, not the history and not the time of the latest
  evaluation. It returns the current state. `Eval` never ignores the first evaluation, and it
  does not ignore an evaluation at the same time as the latest one.

### ActiveAt

- `ActiveAt()` is the time of the evaluation that moved the alert out of Inactive. It stays the
  same while the alert is Pending and Firing, also while it keeps firing after the condition
  cleared.
- While the alert is Inactive, `ActiveAt()` is the zero `time.Time`. A new episode gets a new
  `ActiveAt`.

### History

- Every change of state appends 1 `Transition` with the time of the evaluation (`At`), the old
  state (`From`) and the new state (`To`). Inactive to Firing with `For` 0 is 1 transition,
  without Pending in between.
- An evaluation that does not change the state adds nothing, and neither does an ignored one.
- `History()` returns the transitions oldest first. The caller may change or append to the
  returned slice; the next `History()` call returns the same transitions as before.

### Concurrency

- An `Alert` is not safe for concurrent use, and the tests use it from 1 goroutine. The
  evaluation loop of a monitoring system owns it.

## Examples

`Rule{For: 2 * time.Minute, KeepFiringFor: 5 * time.Minute}`, times as minutes:seconds after the
first evaluation:

```text
time   Eval(active)   returns    ActiveAt()   History() gets
00:00  false          inactive   zero
00:30  true           pending    00:30        {00:30 inactive pending}
01:30  true           pending    00:30        (1m after 00:30: For has not passed)
02:30  true           firing     00:30        {02:30 pending firing}
03:00  false          firing     00:30        (30s after the last true evaluation at 02:30)
07:00  true           firing     00:30        (the last true evaluation is now 07:00)
11:59  false          firing     00:30        (4m59s after 07:00)
12:00  false          inactive   zero         {12:00 firing inactive}
11:00  true           inactive   zero         (before 12:00: ignored)
12:30  true           pending    12:30        {12:30 inactive pending}
```

A night of short spikes: an evaluation every 30 s for 8 h (960 evaluations, the first at
00:00). The condition is true when the time since the first evaluation, modulo 10 minutes, is
under 40 s: at 0 s and 30 s of every 10 minutes.

```text
Rule                   History()                                     last Eval returns
{}                     96 transitions: fires 48 times, resolves 48   inactive
{For: 2m}              96 transitions: pending and back, 48 times    inactive
{KeepFiringFor: 10m}   1 transition: fires at 00:00, never resolves  firing
```

With `For` 2m the night pages nobody: each spike is 2 true evaluations, 30 s apart.
`KeepFiringFor` alone does not help: it turns 48 pages into 1 page that lasts all night.

## Run the tests

    go test ./exercises/22-alertstate/
    go test -run TestState -v ./exercises/22-alertstate/              # step 1: String, New, the getters of a new Alert
    go test -run TestEvalFor -v ./exercises/22-alertstate/            # step 2: Inactive, Pending, For
    go test -run TestEvalKeepFiring -v ./exercises/22-alertstate/     # step 3: Firing, KeepFiringFor
    go test -run TestEvalScenarios -v ./exercises/22-alertstate/      # step 4: the 2 examples, a clock that moves back, History

Run 1 step at a time; the 4 filters do not overlap. Every test recovers a panic, so a
"TODO: implement" fails only that test and the test binary goes on. A call that blocks for 5 s
stops the run and prints the stacks of all goroutines. The tests drive a fake clock: the time
moves only when a test moves it. A failure message names the rule, the time after `t0` (the
first evaluation) and the call, for example
`rule {For:2m0s KeepFiringFor:0s} at t0+2m0s: Eval(true) = pending, want firing`.

## Docs you can use

- [The Go spec: Iota](https://go.dev/ref/spec#Iota) and [The zero value](https://go.dev/ref/spec#The_zero_value)
- [The Go spec: Switch statements](https://go.dev/ref/spec#Switch_statements)
- [`fmt.Stringer`](https://pkg.go.dev/fmt#Stringer)
- [Effective Go: Pointers vs. Values](https://go.dev/doc/effective_go#pointers_vs_values), on methods that change their receiver
- [`time.Time.Sub`](https://pkg.go.dev/time#Time.Sub), [`time.Time.Before`](https://pkg.go.dev/time#Time.Before), [`time.Time.IsZero`](https://pkg.go.dev/time#Time.IsZero) and [`time.Duration`](https://pkg.go.dev/time#Duration)
- [`slices.Clone`](https://pkg.go.dev/slices#Clone)
- [`errors.Is`](https://pkg.go.dev/errors#Is) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`
- [Prometheus: Alerting rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/), the fields `for` and `keep_firing_for`

## Stretch goals

- Add `FiredAt()` and `ResolvedAt()`, and keep a resolved alert visible for 15 minutes, so that a
  notifier can still send "resolved" after a restart. Prometheus does this for every alert.
- Survive a restart: write `ActiveAt` and the state to a file, and restore them in `New`, so a
  restart does not reset the `For` timer of an alert that was about to fire.
- Make it a tool: `alertreplay -for 2m -keep-firing-for 5m motion.csv` reads `time,value` lines,
  applies a threshold, and prints every transition. Tune `for` on last week's data before you
  change the real rule.
- Many alerts from 1 rule: 1 `Alert` per feeder, created on the first true evaluation and
  dropped when it is Inactive again. 44-alertgroup groups such alerts into notifications.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 22 hint 1
    go run ./cmd/peek 22 hint 2
    go run ./cmd/peek 22 hint 3
    go run ./cmd/peek 22 explanation
    go run ./cmd/peek 22 walkthrough
    go run ./cmd/peek 22 solution
