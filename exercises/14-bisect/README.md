# 14 · 2048 builds, 1 bad commit

**Tier 3 · about 1 h** · Go: binary search with `sort.Search` and `slices.BinarySearchFunc`, functions as parameters, loop invariants, `Time.Compare`, half-open ranges · Problem solving: write the invariant for `lo` and `hi`; test every boundary, not a few

2048 agent builds since `v2.13.0`, the last good release, 3 weeks ago: the build server makes 1
for every merged commit, and most of those commits come from `hush-bot`, HUSH's own account. Since
Tuesday HUSH, the AI that holds the pager for 4000 companies, answers some pages by lowering the
alert's severity, and the page closes with nothing fixed. A test means deploying a build to
`canary-h3-01`, the canary node in Hall 3, and replaying a night of pages for 4 minutes. Daniyar
Seitkali, the CTO, trusts HUSH and started at build 0 at 09:00 to prove it. Going 1 by 1 at 4
minutes a build, he reaches build 2047 in almost 6 days. Noor Haddad, the principal SRE, has a
second question about the worst lowered page, the one at 03:12: which build was running then? In
this exercise you write the team's `bisect` tool: the first bad build in few tests, the build that
ran at a given time, and the builds of a time window.

## Task

Implement the stubs in `bisect.go`. The type `Build` is given.

```go
func FirstBad(n int, isBad func(i int) bool) int                 // first bad build of 0 to n-1, or n
func FirstBadErr(n int, test func(i int) (bool, error)) (int, error) // FirstBad for a test that can fail

type Build struct {
	Version string
	At      time.Time // rollout time; builds are sorted by At, strictly increasing
}

func Running(builds []Build, t time.Time) (Build, bool)   // the newest build with At <= t
func Between(builds []Build, from, to time.Time) []Build  // the builds with At in [from, to), a new slice
```

Suggested order: `FirstBad`, then `FirstBadErr`, then `Running`, then `Between`. The tests follow
the same order in 3 steps.

## Rules

### FirstBad

- The builds have the indexes 0 to n-1. `isBad(i)` reports whether build i is bad.
- All good builds come before all bad builds: if build i is bad, every build after it is bad too.
  The tests never break this.
- `FirstBad` returns the index of the first bad build. When no build is bad, it returns n. When
  every build is bad, it returns 0.
- It calls `isBad` only with `0 <= i < n`.
- It calls `isBad` at most `bits.Len(uint(n))` times. That is the number of binary digits of n:

  | n | 1 | 2 | 3 | 4 | 100 | 2048 | 2^30 |
  |---|---|---|---|---|---|---|---|
  | most calls | 1 | 2 | 2 | 3 | 7 | 12 | 31 |

- For `n <= 0` it returns 0 and does not call `isBad`.
- The tests count the calls. They check every n from 0 to 100, with the first bad build at every
  position from 0 to n. They also check n = 2^30 with 27 positions, 20 of them random from a fixed
  seed. A test stops at the first call that breaks a rule and prints the indexes of all calls so
  far.

### FirstBadErr

- Without an error, `FirstBadErr` follows every rule of `FirstBad`: the same result, the same
  limits on the calls, and a `nil` error.
- When `test(i)` returns an error, `FirstBadErr` does not call `test` again. It returns -1 and an
  error that:
  - wraps the error of `test`, so that `errors.Is` finds it;
  - contains `build <i>`, with i in decimal, for example
    `build 1536: deploy failed: canary-h3-01 offline`.
- The tests make `test` return an error on its 1st, 2nd or 3rd call, with 2048 builds, and on the
  1st call with 1 and 3 builds.

### Running

- `builds` is sorted by `At`, strictly increasing. The tests never pass anything else.
- `Running` returns the newest build with `At <= t`, and `true`. A build runs from its own `At` on:
  at exactly `At` it is the running build.
- It returns the zero `Build` and `false` when `builds` is empty or nil, or when t is before the
  first build.
- Times are instants. The builds in the tests are in UTC, and some times t are in other zones
  (`time.FixedZone`): `12:30 CET` is the same instant as `11:30 UTC`. The `==` operator on
  `time.Time` also compares the zone, so it says that these 2 differ.
- `Running` takes O(log n) time. `TestRunningIsFast` makes 1 000 000 calls on 1 000 000 builds
  and gives up after 20 s. A binary search needs about 1 s, or a few seconds with `-race`. A loop
  over the builds stops at the limit.

### Between

- `builds` is sorted by `At`, strictly increasing.
- `Between` returns the builds with `from <= At < to`, in their order. A build exactly at `from` is
  in the result; a build exactly at `to` is not.
- Times are instants, as in `Running`: the tests also pass `from` and `to` in other zones.
- When no build is in the window, and always when `to <= from`, the result is empty: `nil` and a
  slice of length 0 are both fine.
- The result never shares memory with `builds`. Changing an element of the result, or appending to
  it, does not change `builds`.
- `Between` finds the window in O(log n) time, plus the time to copy the builds it returns.
  `TestBetweenIsFast` makes 200 000 calls on 1 000 000 builds, each for a window of 3 builds, and
  gives up after 20 s.

## Examples

2048 builds. For `isBad` and `test`, builds 0 to 1336 are good and builds 1337 to 2047 are bad.
`noneBad` returns false for every build.

```text
FirstBad(2048, isBad)    → 1337, after at most 12 calls of isBad
FirstBad(2048, noneBad)  → 2048
FirstBad(0, isBad)       → 0, without a call

FirstBadErr(2048, test) with no error            → 1337, nil
FirstBadErr(2048, test) where test(1536) fails   → -1, error "build 1536: deploy failed: canary-h3-01 offline"
```

5 builds:

```text
v2.14.0-b0001  2026-11-02 09:00 UTC
v2.14.0-b0002  2026-11-02 11:30 UTC
v2.14.0-b0003  2026-11-03 08:15 UTC
v2.14.0-b0004  2026-11-05 14:00 UTC
v2.14.0-b0005  2026-11-05 14:01 UTC
```

```text
Running(builds, 2026-11-02 08:59 UTC)   → Build{}, false          (before the first build)
Running(builds, 2026-11-02 09:00 UTC)   → v2.14.0-b0001, true     (exactly at a build)
Running(builds, 2026-11-03 03:12 UTC)   → v2.14.0-b0002, true
Running(builds, 2026-11-03 09:00 CET)   → v2.14.0-b0002, true     (08:00 UTC, before b0003)
Running(builds, 2026-12-02 00:00 UTC)   → v2.14.0-b0005, true     (after the last build)

Between(builds, 2026-11-02 09:00 UTC, 2026-11-03 08:15 UTC)  → [v2.14.0-b0001 v2.14.0-b0002]
Between(builds, 2026-11-05 14:00 UTC, 2026-11-05 14:02 UTC)  → [v2.14.0-b0004 v2.14.0-b0005]
Between(builds, 2026-11-03 09:00 UTC, 2026-11-05 00:00 UTC)  → []   (no build in the window)
Between(builds, 2026-11-05 00:00 UTC, 2026-11-02 00:00 UTC)  → []   (to before from)
```

## Run the tests

    go test ./exercises/14-bisect/
    go test -run TestFirstBad -v ./exercises/14-bisect/      # step 1: FirstBad and FirstBadErr
    go test -run TestRunning -v ./exercises/14-bisect/       # step 2: Running
    go test -run TestBetween -v ./exercises/14-bisect/       # step 3: Between
    go test -run TestFirstBadErr -v ./exercises/14-bisect/   # only FirstBadErr

The 3 filters of the steps do not overlap. Every test runs under a guard. A "TODO: implement"
panic fails that test with a message and does not crash the run. So in step 1 the `FirstBad` tests
can pass while `FirstBadErr` is still a stub. A loop that never ends is caught too: when a call
does not return within 5 s (60 s in the speed tests), the guard prints the stack of every
goroutine and stops the test binary.

## Docs you can use

- [`sort.Search`](https://pkg.go.dev/sort#Search): read the doc comment and its example
- [`slices.BinarySearch`](https://pkg.go.dev/slices#BinarySearch) and [`slices.BinarySearchFunc`](https://pkg.go.dev/slices#BinarySearchFunc)
- [`math/bits.Len`](https://pkg.go.dev/math/bits#Len)
- [`time.Time`](https://pkg.go.dev/time#Time) (the part on `==` and `Equal`), [`Time.Compare`](https://pkg.go.dev/time#Time.Compare), [`Time.Before`](https://pkg.go.dev/time#Time.Before) and [`Time.After`](https://pkg.go.dev/time#Time.After)
- [Go Slices: usage and internals](https://go.dev/blog/slices-intro) (Go blog), on slices that share an array
- [`slices.Clone`](https://pkg.go.dev/slices#Clone)
- [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`, and [`errors.Is`](https://pkg.go.dev/errors#Is)
- [The Go spec: Function types](https://go.dev/ref/spec#Function_types)
- [`git bisect`](https://git-scm.com/docs/git-bisect), the tool that does this for commits

## Stretch goals

- Make it a tool: `bisect -n 2048 ./test-build.sh` runs the script with the build number and reads
  its exit code, as `git bisect run` does. 0 is good, 125 means "cannot test this build", and any
  other code from 1 to 127 is bad.
- Support skipped builds: `test` returns good, bad or skip. When a skipped build sits at the
  boundary, report the range of builds that may hold the first bad one.
- A flaky test is the enemy of bisect. Run each test 3 times and call a build bad only when 2 of
  the 3 runs say so. How many runs does a bisect of 2048 builds need now?
- Each node gets a build at its own time (the rollout waves of 04-waves), and `canary-h3-01`
  always gets it first. Write `RunningOn(node string, t time.Time)` over 1 list of builds per
  node.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 14 hint 1
    go run ./cmd/peek 14 hint 2
    go run ./cmd/peek 14 hint 3
    go run ./cmd/peek 14 explanation
    go run ./cmd/peek 14 walkthrough
    go run ./cmd/peek 14 solution
