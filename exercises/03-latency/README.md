# 03 · Percentiles and histogram quantiles

**Tier 1 · about 1.5 h** · Go: slices share backing arrays (`slices.Clone`, `slices.Sort`), binary search (`sort.Search`), `math.Ceil`, `NaN` and `Inf`, float64 rounding, `time.Duration` arithmetic · Problem solving: pin down a definition precisely; compute small examples by hand

After a deploy, Grafana shows a p99 latency of 235 ms for the checkout API. The load test tool says
250 ms. A colleague says the mean is 45 ms, so all is fine. All three are "right", and you have to
explain why in the incident channel. The load test keeps every sample and computes an exact
percentile. Prometheus does not keep samples: a histogram only counts how many requests fell into
fixed buckets (`le="0.1"`, `le="0.25"`, ...). That is cheap and can be summed across 50 pods, and
`histogram_quantile` estimates the percentile by assuming the requests are spread evenly inside a
bucket. The mean hides the slow requests behind the fast ones. Here you write the exact and the
estimated side, so you know where the difference comes from and how large it can get.

## Task

Implement the stubs in `latency.go` and `histogram.go`:

```go
var ErrNoSamples = errors.New("latency: no samples")                       // given
var ErrInvalidPercentile = errors.New("latency: percentile out of range")  // given
var ErrInvalidBounds = errors.New("latency: invalid bucket bounds")        // given

type Summary struct {
	Count         int
	Min, Max      time.Duration
	Mean          time.Duration
	P50, P90, P99 time.Duration
}
func Summarize(samples []time.Duration) (Summary, error)                   // exact statistics
func Percentile(samples []time.Duration, p float64) (time.Duration, error) // nearest rank, 0 < p <= 100

type Bucket struct {
	UpperBound float64 // seconds; math.Inf(1) for the last bucket
	Count      uint64  // cumulative: observations <= UpperBound
}
type Histogram struct{ /* your fields */ }
func NewHistogram(upperBounds []float64) (*Histogram, error) // finite, strictly increasing, in seconds
func (h *Histogram) Observe(seconds float64)                // count one observation
func (h *Histogram) Buckets() []Bucket                      // like the <name>_bucket{le="..."} series
func (h *Histogram) Quantile(q float64) float64             // like PromQL histogram_quantile
```

## Rules

### Percentile: nearest rank

- Sort the N samples in ascending order. The rank is ⌈p/100 × N⌉ (round **up**), and the result is
  the sample at that 1-based rank, `sorted[rank-1]`. No interpolation.
- The same rule without the formula: the result v is one of the samples, at least p % of the
  samples are `<= v`, and fewer than p % are `< v`. `TestPercentileDefinition` checks this.
- The samples can be in any order. Duplicates count one by one.
- Valid p: `0 < p <= 100`. p = 100 is the maximum; a tiny p such as 0.001 is rank 1, the minimum.
  Any other p, including NaN and ±Inf: the error wraps `ErrInvalidPercentile`.
- No samples (nil or empty): `ErrNoSamples`.
- The rank must be exact for p as written in decimal. float64 cannot hold 99.9, so at run time
  `99.9 / 100 * 1000` is `999.0000000000001` and `math.Ceil` makes it 1000, not 999. The tests
  check p = 99.9 with N = 1000 (rank 999) and N = 41000 (rank 40959); the second case is there
  because reordering the arithmetic only moves the error to other N. Valid p in the tests have at
  most 3 decimals. How you make the rank exact is up to you.
- The caller's slice is not modified: same elements, same order.

### Summarize

- `Mean` is the sum divided by `Count` as `time.Duration` integer division, so the fraction of a
  nanosecond is dropped: the mean of 1 ms, 2 ms and 2 ms is 1666666 ns.
- `P50`, `P90`, `P99` are what `Percentile` returns for 50, 90 and 99.
- No samples: `ErrNoSamples`. The caller's slice is not modified.

### NewHistogram, Observe, Buckets

- Bounds are in seconds, like Prometheus `le` labels. Valid: at least 1 bound, all finite (no NaN,
  no ±Inf), strictly increasing. Zero and negative bounds are allowed. Otherwise the error wraps
  `ErrInvalidBounds`. Do not pass +Inf: the histogram adds the +Inf bucket itself.
- The histogram keeps its own copy of the bounds: changing the caller's slice later changes nothing.
- `Observe(v)` counts v in the first bucket whose bound is `>= v` (`le` is "less than or equal"):
  0.1 goes into the 0.1 bucket, 0.10001 into the next. Values below the first bound go into the
  first bucket (with positive bounds, that includes 0 and negative values); values above the
  highest bound only into +Inf.
- `Buckets()` returns `len(upperBounds)+1` buckets in increasing order; the last has `UpperBound`
  `math.Inf(1)`. `Count` is **cumulative** (observations `<=` the bound), so the counts never
  decrease and the +Inf count is the total.
- `Buckets()` returns a new slice on every call. Changing it does not change the histogram, and a
  slice from an earlier call does not change when you observe more.
- A `Histogram` is not safe for concurrent use. The tests use one goroutine.

### Quantile

The checks on q come first, in this order:

| Condition | Result |
|---|---|
| q is NaN | NaN |
| q < 0 | -Inf |
| q > 1 | +Inf |
| no observations | NaN |

Otherwise, with the cumulative buckets:

1. `total` is the +Inf count, and `rank = q × total` (a float64, it can have a fraction).
2. Find the first bucket whose cumulative count is `>= rank`.
3. If that is the +Inf bucket, return the highest finite bound.
4. If that is the first bucket and its bound is `<= 0`, return that bound.
5. Otherwise interpolate linearly inside the bucket: `lower` is the previous bucket's bound (0 for
   the first bucket), `upper` its own bound, `below` the previous bucket's cumulative count (0 for
   the first bucket), and `in` the count of this bucket alone. The result is
   `lower + (upper − lower) × (rank − below) / in`.

- This is what Prometheus does for classic histograms. The tests compare with a tolerance of 1e-9.
- Not tested: q = 0 with an empty first bucket (step 5 divides 0 by 0; Prometheus returns NaN).

## Examples

10 requests, sorted: 11 12 12 13 13 14 15 16 90 250 ms.

```text
Percentile(samples, 50)   rank ⌈5⌉   = 5   → 13ms
Percentile(samples, 99)   rank ⌈9.9⌉ = 10  → 250ms
Percentile(samples, 0)    → error wrapping ErrInvalidPercentile
Summarize(samples)        → {Count:10 Min:11ms Max:250ms Mean:44.6ms P50:13ms P90:90ms P99:250ms}
```

The same requests in a histogram with the Prometheus default bounds 0.005, 0.01, 0.025, 0.05, 0.1,
0.25, 0.5, 1, 2.5, 5, 10. The 250 ms request is exactly on the 0.25 bound and counts there.

```text
Buckets()       le=0.005:0 le=0.01:0 le=0.025:8 le=0.05:8 le=0.1:9 le=0.25:10 ... le=+Inf:10
Quantile(0.5)   rank 5 in le=0.025:   0.01 + 0.015 × 5/8   = 0.019375  (exact p50: 13 ms)
Quantile(0.9)   rank 9 in le=0.1:     0.05 + 0.05 × 1/1    = 0.1       (exact p90: 90 ms)
Quantile(0.99)  rank 9.9 in le=0.25:  0.1 + 0.15 × 0.9/1   = 0.235     (exact p99: 250 ms)
```

Bounds 0.1, 0.25, 0.5, 1 with cumulative counts 50, 80, 95, 99 and 100 (+Inf):

```text
Quantile(0.9)    rank 90 in le=0.5:        0.25 + 0.25 × 10/15 = 0.41667
Quantile(0.995)  rank 99.5 is past le=1:   +Inf bucket → 1
```

## Run the tests

    go test ./exercises/03-latency/
    go test -run TestPercentile -v ./exercises/03-latency/              # step 1: Percentile
    go test -run TestSummarize -v ./exercises/03-latency/               # step 2: Summarize
    go test -run TestHistogram -v ./exercises/03-latency/               # step 3: NewHistogram, Observe, Buckets
    go test -run 'TestQuantile|Example' -v ./exercises/03-latency/      # step 4: Quantile
    go test -run TestQuantileEdgeCases -v ./exercises/03-latency/       # one test

A stub that panics stops the whole test binary, so run only the tests of your current step. The 4
filters do not overlap. `Example_histogramVersusExact` prints the exact and the estimated
percentiles side by side.

## Docs you can use

- [`slices`](https://pkg.go.dev/slices): `Clone`, `Sort`
- [`sort.Search`](https://pkg.go.dev/sort#Search) and [`sort.SearchFloat64s`](https://pkg.go.dev/sort#SearchFloat64s)
- [`math`](https://pkg.go.dev/math): `Ceil`, `Round`, `Abs`, `NaN`, `IsNaN`, `Inf`, `IsInf`
- [`time.Duration`](https://pkg.go.dev/time#Duration) and [`Duration.Seconds`](https://pkg.go.dev/time#Duration.Seconds)
- [Go Slices: usage and internals](https://go.dev/blog/slices-intro) (Go blog)
- [The Go spec: Constant expressions](https://go.dev/ref/spec#Constant_expressions): why `99.9 / 100 * 1000` in a quick test program is exact
- [Prometheus: Histograms and summaries](https://prometheus.io/docs/practices/histograms/)
- [PromQL `histogram_quantile`](https://prometheus.io/docs/prometheus/latest/querying/functions/#histogram_quantile)
- [Percentile, nearest-rank method](https://en.wikipedia.org/wiki/Percentile) (Wikipedia)

## Stretch goals

- Make `Histogram` safe for concurrent use: first with a `sync.Mutex`, then with `sync/atomic`
  counters as the Prometheus Go client does. Test with many goroutines and `-race`.
- Add `Merge(other *Histogram) error` for equal bounds; that is what `sum by (le)` does across pods.
  Then try the same for `Summary` and see why you cannot: the p99 of 2 pods is not the mean of
  their p99s.
- Add `FractionAtMost(bound float64) (float64, bool)`: the exact share of requests `<= bound` when
  bound is a bucket bound. With a bound on your SLO threshold (300 ms) the SLI is exact, not
  estimated.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 03 hint 1
    go run ./cmd/peek 03 hint 2
    go run ./cmd/peek 03 hint 3
    go run ./cmd/peek 03 explanation
    go run ./cmd/peek 03 walkthrough
    go run ./cmd/peek 03 solution
