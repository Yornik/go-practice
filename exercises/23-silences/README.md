# 23 · A silence until 2036

**Tier 5 · about 1.5 h** · Go: `sync.Mutex` together with an injected clock, deleting from a map while you range over it, callbacks outside a lock, sorting results, a fake clock in tests · Problem solving: decide what each method does with an entry that has already ended; call other code only after you let go of the lock

"Until the migration is done": in November, that was the whole comment on the silence for the
disk alert of `chkpt-store-3`, 1 of the 4 disks in the Undercroft that hold the model checkpoints
of Vesper Labs, the AI lab at the top of the Spire. The silence tool asked for an alert name and a
comment, but not for an end, so `hush-bot`, the chat account of HUSH, the lab's own AI, filled one
in: 87600 hours. The migration took 2 days. On 14 December at 04:40, with sleet on the harbour,
the disk fills up, and the alert fires for 3 hours straight into the silence. Nobody is paged.
That evening Adaeze Nwosu, the CEO, revokes HUSH's write access to silences. Noor Haddad, the
principal SRE, decides that every silence ends and that the on-call channel hears when it does.
In this exercise you write the store behind the silence tool: it matches alerts against silences,
removes the silences that have ended, and reports each one.

## Task

Implement the stubs in `silences.go`. `ErrInvalid` and `Silence` are given. You write the `Store`
fields, `New` and the 4 methods.

```go
var ErrInvalid = errors.New("silences: invalid silence") // given

type Silence struct {
	ID     string    // unique in the store
	Alert  string    // alert name; "" matches every alert
	Target string    // node or host; "" matches every target
	Until  time.Time // the silence is active while now < Until
} // given

type Store struct{ /* you design the fields */ }
func New(now func() time.Time, onExpire func(s Silence)) *Store // now nil means time.Now; onExpire may be nil
func (s *Store) Add(sil Silence) error                         // adds, or replaces the silence with the same ID
func (s *Store) Match(alert, target string) (Silence, bool)    // the active silence for this alert, if any
func (s *Store) Expire() []Silence                             // removes and returns the silences that ended
func (s *Store) Len() int                                      // silences held, ended ones included until Expire
```

The alerting system calls `Match` before it pages anyone: a match means "do not page". A job
calls `Expire` once a minute, and `onExpire` posts "silence disk-2 has ended" to the on-call
channel, with the button "extend 1 h". That button calls `Add` with the same ID and a new `Until`.

Later exercises remove entries in 2 other ways. 33-ratelimit removes a rate limiter that nobody
used for a while. 34-cache gives entries a time to live, also removes them when the cache is
full, and reports a reason for each removal. Here every silence has its own end time, set by
whoever created it. Only `Add` changes it.

## Rules

### New

- `New` returns an empty store. `now` is the store's clock. A nil `now` means `time.Now`.
- `onExpire` may be nil. Then `Expire` calls nothing.

### Add

- `Add` rejects a silence when:
  - its `ID` is empty,
  - its `Alert` and its `Target` are both empty (a silence for every alert on every target), or
  - its `Until` is not after `now()`. This includes `Until` equal to `now()`. A silence without
    an end has the zero `time.Time` as `Until`, which is long before any clock, so `Add` rejects
    it too.
- For a rejected silence, `Add` returns an error that wraps `ErrInvalid` (`errors.Is` works) and
  contains the `ID` formatted with `%q`, for example `silences: invalid silence "": empty ID`.
  The rest of the wording is yours. The store does not change: a rejected silence with the ID of
  a silence in the store leaves that silence as it was.
- Otherwise `Add` stores the silence and returns nil. If the store holds a silence with the same
  `ID`, the new silence replaces it, whether the old one has ended or not. The old silence is
  gone: `Match` and `Expire` never return it again, and `onExpire` never gets it. This is how
  "extend 1 h" works.

### Match

- `Match` reads the clock once. A silence is **active** while `now < Until`. At `Until` it has
  ended. `Match` ignores a silence that has ended, also when `Expire` has not removed it yet.
- An active silence matches when its `Alert` equals `alert` or is `""`, and its `Target` equals
  `target` or is `""`.
- Of all active silences that match, `Match` returns the one with the latest `Until`, and true.
  How exact a silence is does not matter: a silence for every target that ends later wins over a
  silence for this target.
- A tie on `Until` goes to the smallest `ID`, compared as strings with `<`.
- With no match, `Match` returns the zero `Silence` and false.
- `Match` does not change the store.

### Expire

- `Expire` reads the clock once (call it `now`) and removes every silence with `now >= Until`.
- It returns the removed silences sorted by `Until`, earliest first, and a tie by `ID`, compared
  as strings with `<`. When no silence has ended, it returns an empty result (nil or an empty
  slice; both are fine) and calls nothing.
- Then it calls `onExpire` once for each removed silence, in the order of the result. It makes
  the calls in the goroutine that called `Expire`, before it returns.
- When `onExpire` runs, `Expire` has already removed every silence of its result, and it holds no
  lock. So `onExpire` may call `Add`, `Match`, `Expire` and `Len` on the same store, and other
  goroutines can use the store while `onExpire` runs.

### Len

- `Len` returns the number of silences in the store, including silences that have ended but
  that `Expire` has not removed yet.

### Concurrency

- Every method is safe for concurrent use.
- Take a silence that `Add` accepted and that no later `Add` replaced. Exactly 1 `Expire` call
  returns it, and `onExpire` gets it exactly once. This also holds when 2 goroutines call
  `Expire` at the same time.

## Examples

A store with a fake clock. Its `onExpire` extends `disk-2` once, to 06:00.

```text
time   call                                    result
04:00  Add(disk-2: "disk_full", "chkpt-store-2", Until 05:00)    nil
04:00  Add(upgrade: "", "chkpt-store-2", Until 06:00)            nil
04:00  Add(until-done: "disk_full", "", no Until)                error: wraps ErrInvalid, contains the ID as %q: "until-done"
04:00  Match("disk_full", "chkpt-store-2")     upgrade, true      (06:00 is later than 05:00)
04:00  Match("disk_full", "chkpt-store-1")     {}, false
04:59  Match("high_latency", "chkpt-store-2")  upgrade, true      (Alert "" matches every alert)
05:00  Len()                                   2                  (disk-2 has ended; Expire has not run)
05:00  Match("disk_full", "chkpt-store-2")     upgrade, true      (disk-2 has ended)
05:00  Expire()                                [disk-2]
         onExpire(disk-2) calls Add(disk-2: "disk_full", "chkpt-store-2", Until 06:00)   nil
05:00  Match("disk_full", "chkpt-store-2")     disk-2, true       (both end at 06:00; "disk-2" < "upgrade")
05:30  Expire()                                []
06:00  Expire()                                [disk-2 upgrade]   (both end at 06:00: sorted by ID)
06:00  Len()                                   0
```

## Run the tests

    go test ./exercises/23-silences/
    go test -race ./exercises/23-silences/
    go test -run TestAdd -v ./exercises/23-silences/                  # step 1: New, Add, Len
    go test -run TestMatch -v ./exercises/23-silences/                # step 2: Match
    go test -run TestExpireBoundary -v ./exercises/23-silences/       # step 3: Expire and Len
    go test -run TestExpireCallback -v ./exercises/23-silences/       # step 4: onExpire
    go test -race -run TestConcurrent -v ./exercises/23-silences/     # step 5: goroutines

Run 1 step at a time. The 5 filters do not overlap. In steps 1 to 4 every call into the store
runs with a 5 s timeout and a recover. So a "TODO: implement" panic fails 1 test with a message
and does not crash the run. A call that does not return within 5 s fails the test. The message
shows the stacks of all goroutines and the likely cause, for example
`Expire() with an onExpire that calls Add did not return within 5s: onExpire was called while Expire held its lock`.
The tests drive a fake clock: the time moves only when a test moves it. Failure messages show it
as an offset from `t0`, for example `at t0+1h0m0s`. The tests of `Match` and of the order of
`Expire` run each case on 20 fresh stores, so a result that is right only by luck fails. Run
step 5 with `-race`: the race detector fails it on any access to the store's state that the lock
does not cover. On Linux and Windows `-race` needs cgo and a C compiler.

## Docs you can use

- [`sync.Mutex`](https://pkg.go.dev/sync#Mutex) and [Effective Go: Defer](https://go.dev/doc/effective_go#defer)
- [The Go spec: For statements with range clause](https://go.dev/ref/spec#For_range), on what happens when you delete a map entry during the range
- [The Go spec: Deletion of map elements](https://go.dev/ref/spec#Deletion_of_map_elements)
- [`slices.SortFunc`](https://pkg.go.dev/slices#SortFunc), [`cmp.Compare`](https://pkg.go.dev/cmp#Compare) and [`cmp.Or`](https://pkg.go.dev/cmp#Or)
- [`time.Time.Before`](https://pkg.go.dev/time#Time.Before), [`time.Time.After`](https://pkg.go.dev/time#Time.After), [`time.Time.Equal`](https://pkg.go.dev/time#Time.Equal) and [`time.Time.Compare`](https://pkg.go.dev/time#Time.Compare)
- [`errors.Is`](https://pkg.go.dev/errors#Is) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`
- [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [Alertmanager: Silences](https://prometheus.io/docs/alerting/latest/alertmanager/#silences)

## Stretch goals

- Warn before the end: `EndingWithin(d time.Duration) []Silence` lists the active silences that
  end within `d`, so the bot can post "disk-2 ends in 15 minutes" while there is still time to
  extend it.
- Keep the past: hold removed silences for 5 days in a second map, so the tool can show "recently
  ended". Alertmanager keeps expired silences for its `--data.retention` (120 h by default).
- Run `Expire` once a minute in a goroutine with a `time.Ticker`, and stop it with a `context`,
  as in 21-rollout. Make sure the goroutine has ended when your stop function returns.
- Match on labels instead of 2 fields: a silence holds `map[string]string` matchers, and an alert
  is a label set. 44-alertgroup works with label sets.
- Cap the length: `hush-bot` once set a silence for 10 years. Give the store a maximum length,
  and make `Add` reject a silence that ends more than that maximum after `now()`, with an error
  that wraps `ErrInvalid`.
- Make it a tool: `silence add disk_full chkpt-store-2 2d`. 24-durations parses the `2d`.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 23 hint 1
    go run ./cmd/peek 23 hint 2
    go run ./cmd/peek 23 hint 3
    go run ./cmd/peek 23 explanation
    go run ./cmd/peek 23 walkthrough
    go run ./cmd/peek 23 solution
