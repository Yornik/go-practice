# 13 · The refund call at 10:00

**Tier 3 · about 1.5 h** · Go: `float64` and integer vs float division, `strconv.ParseFloat`, `NaN` and `±Inf` (`math.IsNaN`, `math.IsInf`), converting between `float64` and `time.Duration`, `Duration.Round`, half-open intervals with `time.Time` · Problem solving: draw the outages on a time line; keep durations as durations until the last step

"Your contract promises 99.95% uptime. You were down 47 minutes in October. We want a refund."
Meridian Lightrail, the city's tram operator with 300 edge nodes, sends it at 09:20 with a call
invitation, and Marisol Quispe Mamani, the support lead, accepts for the whole team. Lightrail added
up every incident on `status.vesper-labs.example`, where 2 monitors post: the external uptime check
and the load balancer's health check. Both saw the same 2 outages, so the page lists 4 incidents,
and every minute that both saw counts twice. Daniyar Seitkali, the CTO, says that 99.95% "is about
an hour a month". HUSH, the on-call AI that Vesper Labs sells, says 100.00% in its own report for
October. The call is at 10:00, captioned like every call at Vesper Labs, and Noor Haddad, the
principal SRE, asks you who is right, with 1 number for each side. In this exercise you write the
arithmetic behind the answer: read an objective, turn it into allowed downtime, and measure how long
the service was really down.

## Task

Implement the stubs in `nines.go`. `ErrInvalid`, `Outage` and `Report` are given.

```go
var ErrInvalid = errors.New("nines: invalid input") // every error wraps it

func ParseObjective(s string) (float64, error)                  // "99.95%" or "99.95" → 0.9995
func Budget(objective float64, window time.Duration) time.Duration // window × (1 − objective), to the second

type Outage struct{ Start, End time.Time }                      // half-open: [Start, End)
func Downtime(outages []Outage, from, to time.Time) time.Duration  // time in [from, to) that at least 1 outage covers
func Availability(down, window time.Duration) float64            // 1 − down/window; NaN when window <= 0

type Report struct {
	Window       time.Duration // to − from
	Downtime     time.Duration // Downtime(outages, from, to)
	Availability float64       // Availability(Downtime, Window)
	Budget       time.Duration // Budget(objective, Window)
	Remaining    time.Duration // Budget − Downtime; negative when Downtime exceeds Budget
	Met          bool          // Downtime <= Budget
}
func Check(objective float64, from, to time.Time, outages []Outage) (Report, error)
```

An objective is a fraction everywhere except in `ParseObjective`'s input: `0.9995`, not `99.95`.

## Rules

### ParseObjective

- `ParseObjective` ignores the whitespace around the input that `strings.TrimSpace` removes:
  `" 99.5% "` and `"\t99.99%\n"` are fine.
- 1 `%` may follow the number directly. `99.5 %` (a space before the `%`), `99.5%%` and `%99.5`
  are errors.
- `strconv.ParseFloat` reads the number, so `9.995e1` is fine. Anything it cannot read is an
  error: `abc`, `""`, `%`.
- The number must be above 0 and below 100. `0`, `0%`, `-1`, `100` and `100%` are errors.
- NaN and the infinities are errors, in every spelling that `ParseFloat` accepts: `NaN`, `nan`,
  `Inf`, `+Inf`, `-Inf`, `infinity`. Watch out: a range check written as
  `if v <= 0 || v >= 100 { error }` lets NaN through, because every comparison with NaN is false.
- The result is the number divided by 100: `99.95%` gives `0.9995`.
- On error the value is 0, and the error wraps `ErrInvalid` (`errors.Is` works). The message
  contains the input as the caller passed it, before trimming, formatted with `%q`: `"99.5 %"` or
  `"   "`. The rest of the message is yours.

### Budget

- The result is `window × (1 − objective)`, rounded to the nearest second with
  `Duration.Round(time.Second)`.
- A window of 0 gives 0. `Budget` does not check its input: callers pass an objective above 0 and
  below 1.

### Downtime

- An outage is half-open: it includes `Start` and ends just before `End`. `[03:10, 03:20)` lasts
  10 minutes. The window `[from, to)` is half-open too.
- The result is how much of the window at least 1 outage covers. Time that several outages cover
  counts once: overlapping, nested and duplicate outages do not add up.
- Touching outages, `[a, b)` and `[b, c)`, cover `[a, c)` and count `c − a`.
- Only the part of an outage inside the window counts. An outage that ends at `from` or starts at
  `to` counts 0.
- An outage whose `End` is not after its `Start` covers nothing and counts 0.
- The outages may come in any order.
- `Downtime` does not change the caller's slice: not the order, not the times in it.
- If `to` is not after `from`, the result is 0.

### Availability

- The result is `1 − down/window` as a `float64`: 30 minutes down in 1 hour gives `0.5`.
- If `window` is 0 or negative, the result is NaN (`math.NaN()`). Watch out: in floats
  `5m / 0` is `+Inf` and `0 / 0` is NaN, so the division alone gives 2 different answers.
- It does not clamp: callers pass a `down` between 0 and `window`.

### Check

- It returns an error that wraps `ErrInvalid`, together with the zero `Report`, when:
  - `objective` is NaN, or not above 0 and below 1 (`0`, `1`, `-0.5`, `+Inf`, and `99.95`, a
    percentage passed by mistake);
  - `to` is not after `from`;
  - an outage ends before it starts (`End` before `Start`). The message contains `outage <i>`,
    with the 0-based index of the first such outage in the slice, for example `outage 1`.
- An outage whose `End` equals its `Start` is not an error. It counts 0.
- Otherwise the error is nil. `Window` is `to − from`. `Downtime`, `Availability` and `Budget`
  come from the 3 functions above. `Remaining` is `Budget − Downtime`, and `Met` is
  `Downtime <= Budget`.
- A downtime that uses exactly the whole budget meets the objective: `Met` is true and
  `Remaining` is 0.

### Floats in the tests

The tests compare every `float64` result with a tolerance of `1e-9`, so the rounding of your
formula does not matter. They compare durations exactly, to the nanosecond.

## Examples

```text
ParseObjective("99.95%")   → 0.9995, nil
ParseObjective(" 99.9 ")   → 0.999, nil
ParseObjective("100%")     → 0, error that wraps ErrInvalid and contains "100%" in quotes
ParseObjective("NaN")      → 0, error

Budget(0.999,  30 days)    → 43m12s     (43.2 minutes)
Budget(0.9995, 30 days)    → 21m36s
Budget(0.9995, 31 days)    → 22m19s     (22m19.2s rounds down)
Budget(0.9999, 365 days)   → 52m34s     (52m33.6s rounds up)
Budget(0.999,  7 days)     → 10m5s      (10m4.8s rounds up)
```

Outages as minutes after 03:00, window `[03:00, 04:00)`:

```text
[10,20) [30,35)            → 15m
[10,20) [15,30)            → 20m   (overlap counts once)
[10,40) [15,20)            → 30m   (nested)
[10,20) [20,30)            → 20m   (touching)
[-10,5) [55,70)            → 10m   (clipped to the window)
[30,35) [10,20) [15,25)    → 20m   (any order)
[30,25) [10,15)            → 5m    (End before Start counts 0)
```

October, 2 monitors, objective 99.95%:

```text
external uptime check:   7 Oct 03:12–03:31 (19m)   21 Oct 14:02–14:06 (4m)
load balancer check:     7 Oct 03:14–03:33 (19m)   21 Oct 14:03–14:08 (5m)
sum of the reports:      47m

Check(0.9995, 1 Oct 00:00 UTC, 1 Nov 00:00 UTC, the 4 reports) →
  Window       744h0m0s
  Downtime     27m0s        (03:12–03:33 and 14:02–14:08)
  Availability 0.99939516…  (1 − 27/44 640)
  Budget       22m19s
  Remaining    -4m41s
  Met          false
```

## Run the tests

    go test ./exercises/13-nines/
    go test -run TestParseObjective -v ./exercises/13-nines/   # step 1: ParseObjective
    go test -run TestBudget -v ./exercises/13-nines/           # step 2: Budget
    go test -run TestDowntime -v ./exercises/13-nines/         # step 3: Downtime
    go test -run TestAvailability -v ./exercises/13-nines/     # step 4: Availability
    go test -run TestCheck -v ./exercises/13-nines/            # step 5: Check
    go test -run TestDowntimeKeepsInput -v ./exercises/13-nines/   # one test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The test names are chosen so that the 5 filters above do not overlap. Step 5
also uses the functions of steps 2 to 4. `ExampleCheck` runs the October example above. It runs
only in a plain `go test`; an example cannot recover a panic, so a plain run crashes until all 5
steps work.

## Docs you can use

- [`strconv.ParseFloat`](https://pkg.go.dev/strconv#ParseFloat), on the spellings of NaN and the infinities it accepts
- [`strings`](https://pkg.go.dev/strings): `TrimSpace`, `CutSuffix`
- [`math`](https://pkg.go.dev/math): `NaN`, `IsNaN`, `Inf`, `IsInf`
- [`time.Duration`](https://pkg.go.dev/time#Duration) and [`Duration.Round`](https://pkg.go.dev/time#Duration.Round)
- [`time.Time`](https://pkg.go.dev/time#Time): `Before`, `After`, `Compare`, `Sub`
- [`slices`](https://pkg.go.dev/slices): `SortFunc`, `Clone`
- [The Go spec: Arithmetic operators](https://go.dev/ref/spec#Arithmetic_operators), on integer division, and [Conversions between numeric types](https://go.dev/ref/spec#Conversions_between_numeric_types)
- [The Go spec: Comparison operators](https://go.dev/ref/spec#Comparison_operators)
- [Google SRE book: Embracing Risk](https://sre.google/sre-book/embracing-risk/), with the table of allowed downtime per number of nines
- [Google SRE workbook: Implementing SLOs](https://sre.google/workbook/implementing-slos/)

## Stretch goals

- Make it a tool: `nines -objective 99.95 outages.csv`, with 1 outage per line as 2 RFC 3339
  times. Print the report, and exit with status 1 when the window misses the objective. Marisol
  wants it before the next refund call.
- Show each monitor on its own: the downtime that each one reports, and the union. A big gap
  between 2 monitors is a finding of its own.
- Use calendar months in a local zone. In `Europe/Amsterdam` October 2026 has 745 hours, because
  the clocks go back 1 hour. Build the window with `time.LoadLocation` and `AddDate(0, 1, 0)`.
- Weigh partial outages: an outage in which 30% of the requests failed counts 0.3 of its length.
  That is the step from time-based to request-based availability, which 30-slo takes.
- Compute the mean and the lowest availability of 100 customers, 99 at 100% and 1 at 95%. Which
  of the 2 numbers belongs in a report to the 100th?

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 13 hint 1
    go run ./cmd/peek 13 hint 2
    go run ./cmd/peek 13 hint 3
    go run ./cmd/peek 13 explanation
    go run ./cmd/peek 13 walkthrough
    go run ./cmd/peek 13 solution
