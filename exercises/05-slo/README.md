# 05 · Error budgets and burn-rate alerts

**Tier 2 · about 1.5 h** · Go: float64 precision and tolerances, integer vs float division, half-open time intervals with `time.Time`, value receivers, passing `now` in · Problem solving: turn a domain formula into code; check the units; compute examples by hand

You own the checkout API. Its SLO: 99.9 % of requests succeed over a rolling 30 days, so the error
budget is 0.1 % of the requests. A classic "error rate above 1 % for 5 minutes" alert is wrong both
ways: it pages you at 3 am for a blip that spent 0.12 % of the month's budget, and it stays silent
while a steady 0.5 % error rate eats the whole budget in 6 days. The Google SRE Workbook replaces it
with burn-rate alerts over two windows: the long window proves that enough budget burned to matter,
the short one proves that it is still burning. Sloth and Pyrra generate these Prometheus rules.
Here you write the math behind them on per-minute request counts, so you can backtest the rules
against last week's incident and explain why the page fired at 03:09 and stopped at 03:35.

## Task

Implement the stubs in `slo.go`:

```go
type Bucket struct{ Start time.Time; Good, Total int64 }   // the request counts of one minute
type SLO struct{ Objective float64; Period time.Duration }  // {0.999, 30 * 24 * time.Hour}
var ErrInvalidSLO = errors.New("slo: invalid SLO")          // given

func (s SLO) Validate() error                                                     // nil, or wraps ErrInvalidSLO
func (s SLO) ErrorRatio(b []Bucket, now time.Time, window time.Duration) float64  // bad / total in the window
func (s SLO) BurnRate(b []Bucket, now time.Time, window time.Duration) float64    // error ratio / budget
func (s SLO) BudgetRemaining(b []Bucket, now time.Time) float64                   // over Period; may be < 0

type Rule struct{ Name, Severity string; Long, Short time.Duration; Factor float64 }
type Alert struct{ Rule, Severity string; LongBurn, ShortBurn float64 }
func DefaultRules() []Rule                                                        // the 4 rules below
func (s SLO) Evaluate(b []Bucket, now time.Time, rules []Rule) []Alert            // the rules that fire
func (s SLO) TimeToExhaustion(b []Bucket, now time.Time, window time.Duration) (time.Duration, bool)
```

Every method takes the buckets, then `now`, then what to compute. None of them reads the clock.

## Rules

### The formulas

| Quantity | Formula |
|---|---|
| bad requests of a bucket | `Total − Good` |
| error ratio | Σ bad / Σ Total over the window |
| error budget | `1 − Objective` (0.001 for 99.9 %) |
| burn rate | error ratio / error budget |
| budget remaining | `1 − burn rate over [now − Period, now)` |
| time to exhaustion | `remaining × Period / burn rate over window` |

A burn rate of 1 spends exactly the whole budget in one Period. A burn rate of 14.4 for 1 hour
spends 14.4 h / 720 h = 2 % of it.

### Windows and buckets

- A bucket is in a window when `now − window <= Start < now`: a bucket that starts exactly at
  `now − window` is in, a bucket that starts at `now` is out. Only `Start` decides, and `now` does
  not have to be a whole minute.
- Buckets come in any order. Buckets with `Start >= now` are ignored: a backtest passes a whole
  day of buckets and moves `now` through it.
- Several buckets can have the same `Start` (one per replica). All of them count.
- Sum first, then divide: Σ bad / Σ Total, not the mean of the per-bucket ratios.
- No traffic in the window (no buckets, or Σ Total = 0) gives error ratio 0 and burn rate 0, never
  NaN. No traffic in the period gives budget remaining 1.
- No method modifies the caller's buckets slice.
- In all tests `0 <= Good <= Total`. You do not have to check it.

### Validate

- Valid when `0 < Objective < 1` and `Period > 0`. Otherwise the error wraps `ErrInvalidSLO`
  (`errors.Is` works). The wording is yours.
- Invalid examples: the zero `SLO`, objective 0, 1, −0.5, 99.9 (a percentage, not a fraction),
  NaN and +Inf; period 0 or negative.
- The other methods are only called with valid SLOs.

### BudgetRemaining

- 1 when nothing is spent, 0 when the budget is spent exactly, negative when it is overspent
  (−1 means twice the budget). Do not clamp it.
- The period rolls: errors older than `now − Period` no longer count.

### DefaultRules

The 4 rules for a 30-day SLO from the SRE Workbook and Sloth, in this order:

| Name | Severity | Long | Short | Factor | Budget spent in Long when it fires |
|---|---|---|---|---|---|
| `1h/5m` | `page` | 1 h | 5 min | 14.4 | 2 % |
| `6h/30m` | `page` | 6 h | 30 min | 6 | 5 % |
| `1d/2h` | `ticket` | 24 h | 2 h | 3 | 10 % |
| `3d/6h` | `ticket` | 72 h | 6 h | 1 | 10 % |

- Every call returns a new slice: a caller that changes its result (to tune a Factor) does not
  change what the next call returns.

### Evaluate

- A rule fires when the burn rate over `Long` **and** the burn rate over `Short` both reach `Factor`.
- "Reach" includes a tolerance: `burn >= Factor × (1 − 1e-9)`. At run time `1 − 0.999` is
  `0.0010000000000000009`, so an error ratio of exactly 0.001 gives the burn rate
  `0.9999999999999991`, not 1, and a rule exactly at its Factor would not fire without it.
  A burn rate 0.1 % below Factor does not fire.
- One `Alert` per firing rule, in the order of `rules`. `Rule` is the rule's `Name`, `Severity`
  its `Severity`, `LongBurn` and `ShortBurn` the two burn rates.
- No rule fires, or `rules` is empty: return nil or an empty slice.

### TimeToExhaustion

- The result is `remaining × Period / burn` and `true`, with `burn` the burn rate over `window`
  and `remaining` from `BudgetRemaining`. It is `(0, false)` when `burn` is 0 or `remaining <= 0`.
- If the result does not fit in a `time.Duration` (2^63 ns or more, about 292 years), return
  `math.MaxInt64` and `true`. Converting a float64 that is too large to an integer does not fail
  in Go: the spec says the result is implementation-dependent. On amd64 it is negative. On arm64
  it happens to be `math.MaxInt64`, so there the test passes without the check: write it anyway.

### Precision in the tests

Floats are compared with a tolerance of 1e-9 (relative above 1) and durations within 1 µs, so
the last bits of your arithmetic do not matter.

## Examples

```text
SLO{Objective: 0.999, Period: 30 days}, now = 12:00

Buckets 11:55 to 11:59, 1000 requests each, 2 bad in the 11:58 bucket:
  ErrorRatio(window 5m) = 2 / 5000 = 0.0004
  BurnRate(window 5m)   = 0.0004 / 0.001 = 0.4

Buckets 11:58 {Total 100, 50 bad} and 11:59 {Total 900, 0 bad}:
  ErrorRatio(window 5m) = 50 / 1000 = 0.05      (not (0.5 + 0) / 2 = 0.25)
```

```text
30 days at 5000 requests per minute: 216 million requests, a budget of 216 000 bad ones.
108 000 bad so far:                       BudgetRemaining = 0.5
Last hour: 72 of 5000 fail every minute:  BurnRate(window 1h) = 0.0144 / 0.001 = 14.4
TimeToExhaustion(window 1h)             = 0.5 × 720h / 14.4 = 25h, true
TimeToExhaustion(window 6h)             = burn rate 2.4 over 6h: 0.5 × 720h / 2.4 = 150h, true
```

An incident replay with `DefaultRules()` (this is `TestEvaluateIncidentReplay`). Traffic is 1000
requests per minute for 3 days. From t0 to t0+30m, 100 of them fail each minute (10 %, burn rate
100). A minute with 100 bad requests adds 1.67 to the 1h burn rate, 20 to the 5m one, 0.28 to the
6h one and 3.33 to the 30m one:

```text
now      1h burn  5m burn  6h burn  30m burn   fires
t0+8m      13.3    100       2.2      26.7     -
t0+9m      15.0    100       2.5      30.0     1h/5m
t0+22m     36.7    100       6.1      73.3     1h/5m, 6h/30m
t0+34m     50.0     20       8.3      86.7     1h/5m, 6h/30m
t0+35m     50.0      0       8.3      83.3     6h/30m
t0+58m     50.0      0       8.3       6.7     6h/30m
t0+59m     50.0      0       8.3       3.3     -
```

The tickets never fire: 3000 bad requests are a burn rate of 2.1 over 1 day and 0.7 over 3 days.
Without the 5m window, the 1h/5m page would keep firing until t0+82m, 52 minutes after the
incident ended.

## Run the tests

    go test ./exercises/05-slo/
    go test -run TestValidate -v ./exercises/05-slo/                        # step 1
    go test -run 'TestErrorRatio|TestBurnRate' -v ./exercises/05-slo/        # step 2
    go test -run TestBudgetRemaining -v ./exercises/05-slo/                  # step 3
    go test -run 'TestDefaultRules|TestEvaluate' -v ./exercises/05-slo/      # step 4
    go test -run TestTimeToExhaustion -v ./exercises/05-slo/                 # step 5
    go test -run 'TestErrorRatio/unsorted' -v ./exercises/05-slo/            # one case

The step filters do not overlap, and a "TODO: implement" panic shows up as a failing test, not a
crash. There is no concurrency, so `-race` is not needed.

## Docs you can use

- [`time`](https://pkg.go.dev/time): [`Time.Add`](https://pkg.go.dev/time#Time.Add),
  [`Time.Before`](https://pkg.go.dev/time#Time.Before), [`Duration`](https://pkg.go.dev/time#Duration)
- [`math`](https://pkg.go.dev/math): [`IsNaN`](https://pkg.go.dev/math#IsNaN) and the
  [constants](https://pkg.go.dev/math#pkg-constants) such as `MaxInt64`
- [`errors.Is`](https://pkg.go.dev/errors#Is) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`
- The Go spec: [Arithmetic operators](https://go.dev/ref/spec#Arithmetic_operators),
  [Conversions](https://go.dev/ref/spec#Conversions), [Constant expressions](https://go.dev/ref/spec#Constant_expressions)
- Go FAQ: [Should I define methods on values or pointers?](https://go.dev/doc/faq#methods_on_values_or_pointers)
- [Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/), Google SRE Workbook

## Stretch goals

- Derive the factor instead of hard-coding it: `Factor = budget fraction × Period / Long`, so the
  rules also work for a 7-day or 28-day SLO.
- Make `Evaluate` fast for 30 days of buckets and many rules: sort once, build prefix sums, find
  the window edges with `sort.Search`. Measure with a benchmark.
- A latency SLO ("99 % of requests faster than 300 ms"): count good requests from histogram
  buckets (see 03-latency) and reuse the rest.
- Generate the Prometheus recording and alerting rules for an SLO as YAML, the way Sloth does.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 05 hint 1
    go run ./cmd/peek 05 hint 2
    go run ./cmd/peek 05 hint 3
    go run ./cmd/peek 05 explanation
    go run ./cmd/peek 05 walkthrough
    go run ./cmd/peek 05 solution
