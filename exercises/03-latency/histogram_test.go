package latency_test

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	latency "github.com/yornik/go-practice/exercises/03-latency"
)

// defBuckets are the default buckets of the Prometheus Go client, in seconds.
var defBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

var inf = math.Inf(1)

// newHistogram calls NewHistogram and stops the test if it fails.
func newHistogram(t *testing.T, bounds ...float64) *latency.Histogram {
	t.Helper()
	h, err := latency.NewHistogram(bounds)
	if err != nil {
		t.Fatalf("NewHistogram(%v) error = %v, want nil", bounds, err)
	}
	if h == nil {
		t.Fatalf("NewHistogram(%v) = nil, want a histogram", bounds)
	}
	return h
}

// observe calls h.Observe once for each value.
func observe(h *latency.Histogram, values ...float64) {
	for _, v := range values {
		h.Observe(v)
	}
}

// observeN calls h.Observe(v) n times.
func observeN(h *latency.Histogram, n int, v float64) {
	for range n {
		h.Observe(v)
	}
}

// buckets builds the expected Buckets() result: the bounds plus +Inf, with
// the given cumulative counts.
func buckets(bounds []float64, counts ...uint64) []latency.Bucket {
	if len(counts) != len(bounds)+1 {
		panic("buckets: need one count per bound plus one for +Inf")
	}
	out := make([]latency.Bucket, 0, len(counts))
	for i, c := range counts {
		upper := inf
		if i < len(bounds) {
			upper = bounds[i]
		}
		out = append(out, latency.Bucket{UpperBound: upper, Count: c})
	}
	return out
}

// checkBuckets compares h.Buckets() with want.
func checkBuckets(t *testing.T, h *latency.Histogram, want []latency.Bucket) {
	t.Helper()
	if got := h.Buckets(); !slices.Equal(got, want) {
		t.Errorf("Buckets()\n got  %v\n want %v", got, want)
	}
}

// ---- NewHistogram, Observe, Buckets -----------------------------------------

func TestHistogramNew(t *testing.T) {
	tests := []struct {
		name   string
		bounds []float64
	}{
		{"one bound", []float64{1}},
		{"Prometheus default buckets", defBuckets},
		{"zero as the first bound", []float64{0, 0.5, 1}},
		{"negative bounds", []float64{-0.01, 0, 0.01}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHistogram(t, tc.bounds...)
			// A new histogram has every bound plus +Inf, all with count 0.
			checkBuckets(t, h, buckets(tc.bounds, make([]uint64, len(tc.bounds)+1)...))
		})
	}
}

func TestHistogramNewInvalid(t *testing.T) {
	tests := []struct {
		name   string
		bounds []float64
	}{
		{"nil", nil},
		{"empty", []float64{}},
		{"NaN", []float64{0.1, math.NaN(), 1}},
		{"only NaN", []float64{math.NaN()}},
		{"+Inf", []float64{0.1, 1, math.Inf(1)}},
		{"-Inf", []float64{math.Inf(-1), 0.1}},
		{"duplicate bound", []float64{0.1, 0.25, 0.25, 0.5}},
		{"decreasing", []float64{1, 0.5}},
		{"one bound out of order", []float64{0.1, 0.5, 0.25, 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := latency.NewHistogram(tc.bounds)
			if !errors.Is(err, latency.ErrInvalidBounds) {
				t.Errorf("NewHistogram(%v) error = %v, want an error that wraps ErrInvalidBounds", tc.bounds, err)
			}
		})
	}
}

func TestHistogramObserve(t *testing.T) {
	bounds := []float64{0.1, 0.25, 0.5, 1}
	tests := []struct {
		name    string
		observe []float64
		want    []uint64 // cumulative counts for le 0.1, 0.25, 0.5, 1, +Inf
	}{
		{"nothing observed", nil, []uint64{0, 0, 0, 0, 0}},
		// le means "less than or equal".
		{"a value on a bound goes into that bucket", []float64{0.1}, []uint64{1, 1, 1, 1, 1}},
		{"a value just above a bound goes into the next bucket", []float64{0.10001}, []uint64{0, 1, 1, 1, 1}},
		{"the highest bound is a finite bucket", []float64{1}, []uint64{0, 0, 0, 1, 1}},
		{"values below the first bound, zero and negative", []float64{0.05, 0, -0.2}, []uint64{3, 3, 3, 3, 3}},
		{"values above the highest bound count only in +Inf", []float64{1.5, 30}, []uint64{0, 0, 0, 0, 2}},
		// Per bucket 1, 2, 1, 2, 1 (0.25 is on a bound); cumulative 1, 3, 4, 6, 7.
		{"mixed", []float64{0.05, 0.2, 0.25, 0.3, 0.7, 0.9, 5}, []uint64{1, 3, 4, 6, 7}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHistogram(t, bounds...)
			observe(h, tc.observe...)
			if got, want := h.Buckets(), buckets(bounds, tc.want...); !slices.Equal(got, want) {
				t.Errorf("after Observe of %v, Buckets()\n got  %v\n want %v", tc.observe, got, want)
			}
		})
	}
}

// The README example: the ten requests with the Prometheus default buckets.
func TestHistogramObserveDurations(t *testing.T) {
	h := newHistogram(t, defBuckets...)
	for _, d := range tenRequests() {
		h.Observe(d.Seconds())
	}
	// 11..16 ms are in le=0.025, 90 ms in le=0.1, 250 ms exactly on le=0.25.
	checkBuckets(t, h, buckets(defBuckets, 0, 0, 8, 8, 9, 10, 10, 10, 10, 10, 10, 10))
}

// A caller that builds bounds in a slice and reuses that slice afterwards
// must not change the histogram.
func TestHistogramNewCopiesBounds(t *testing.T) {
	bounds := []float64{0.1, 0.25, 0.5}
	h, err := latency.NewHistogram(bounds)
	if err != nil {
		t.Fatalf("NewHistogram(%v) error = %v, want nil", bounds, err)
	}
	bounds[0], bounds[1], bounds[2] = 7, 8, 9

	h.Observe(0.2)
	checkBuckets(t, h, buckets([]float64{0.1, 0.25, 0.5}, 0, 1, 1, 1))
}

// Buckets returns a new slice on every call. A caller may modify it, and a
// snapshot taken earlier stays as it was: a scraper computes the increase
// between two snapshots, like rate() does.
func TestHistogramBucketsIsACopy(t *testing.T) {
	bounds := []float64{0.1, 0.5}

	t.Run("modifying the result does not change the histogram", func(t *testing.T) {
		h := newHistogram(t, bounds...)
		h.Observe(0.05)
		got := h.Buckets()
		for i := range got {
			got[i].UpperBound = 42
			got[i].Count = 99
		}
		checkBuckets(t, h, buckets(bounds, 1, 1, 1))
	})

	t.Run("an earlier snapshot does not change", func(t *testing.T) {
		h := newHistogram(t, bounds...)
		h.Observe(0.05)
		before := h.Buckets()
		h.Observe(0.3)
		h.Observe(2)
		after := h.Buckets()
		if want := buckets(bounds, 1, 1, 1); !slices.Equal(before, want) {
			t.Errorf("snapshot taken before 2 more Observe calls changed to\n got  %v\n want %v", before, want)
		}
		if want := buckets(bounds, 1, 2, 3); !slices.Equal(after, want) {
			t.Errorf("Buckets() after 3 observations\n got  %v\n want %v", after, want)
		}
	})
}

// ---- Quantile ---------------------------------------------------------------

// hundredRequests returns the README example: 100 observations with bounds
// 0.1, 0.25, 0.5 and 1: 50 in (0, 0.1], 30 in (0.1, 0.25], 15 in (0.25, 0.5],
// 4 in (0.5, 1] and 1 above 1. Cumulative counts: 50, 80, 95, 99, 100.
func hundredRequests(t *testing.T) *latency.Histogram {
	t.Helper()
	h := newHistogram(t, 0.1, 0.25, 0.5, 1)
	observeN(h, 50, 0.07)
	observeN(h, 30, 0.2)
	observeN(h, 15, 0.3)
	observeN(h, 4, 0.6)
	observeN(h, 1, 3)
	return h
}

// sameFloat reports whether got and want are equal within 1e-9, treating two
// NaNs as equal and infinities as equal only to themselves.
func sameFloat(got, want float64) bool {
	switch {
	case math.IsNaN(want):
		return math.IsNaN(got)
	case math.IsInf(want, 0):
		return got == want
	default:
		return math.Abs(got-want) <= 1e-9
	}
}

type quantileCase struct {
	name string
	q    float64
	want float64
}

func runQuantileCases(t *testing.T, h *latency.Histogram, cases []quantileCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.Quantile(tc.q); !sameFloat(got, tc.want) {
				t.Errorf("Quantile(%v) = %v, want %v\n buckets: %v", tc.q, got, tc.want, h.Buckets())
			}
		})
	}
}

func TestQuantile(t *testing.T) {
	// Cumulative counts: le=0.1: 50, le=0.25: 80, le=0.5: 95, le=1: 99, +Inf: 100.
	runQuantileCases(t, hundredRequests(t), []quantileCase{
		// rank 0 is in the first bucket: 0 + 0.1 * 0/50.
		{"q=0", 0, 0},
		// rank 25 in le=0.1: 0 + 0.1 * 25/50.
		{"q=0.25", 0.25, 0.05},
		// rank 50 in le=0.1 (count 50 >= 50): 0 + 0.1 * 50/50.
		{"q=0.5, rank on the top of a bucket", 0.5, 0.1},
		// rank 75 in le=0.25: 0.1 + 0.15 * (75-50)/30.
		{"q=0.75", 0.75, 0.225},
		// rank 90 in le=0.5: 0.25 + 0.25 * (90-80)/15.
		{"q=0.9", 0.9, 0.25 + 0.25*10.0/15.0},
		// rank 95 in le=0.5: 0.25 + 0.25 * (95-80)/15.
		{"q=0.95", 0.95, 0.5},
		// rank 99 in le=1: 0.5 + 0.5 * (99-95)/4.
		{"q=0.99", 0.99, 1},
		// rank 99.5 is past le=1 (99), so it is in +Inf: the highest finite bound.
		{"q=0.995, rank in the +Inf bucket", 0.995, 1},
		{"q=1, rank in the +Inf bucket", 1, 1},
	})
}

func TestQuantileSpecialValues(t *testing.T) {
	t.Run("with observations", func(t *testing.T) {
		runQuantileCases(t, hundredRequests(t), []quantileCase{
			{"q is NaN", math.NaN(), math.NaN()},
			{"q below 0", -0.1, math.Inf(-1)},
			{"q above 1", 1.01, math.Inf(1)},
		})
	})
	// The checks on q come before the check for "no observations".
	t.Run("empty histogram", func(t *testing.T) {
		runQuantileCases(t, newHistogram(t, 0.1, 1), []quantileCase{
			{"q=0.5", 0.5, math.NaN()},
			{"q=0", 0, math.NaN()},
			{"q=1", 1, math.NaN()},
			{"q is NaN", math.NaN(), math.NaN()},
			{"q below 0", -1, math.Inf(-1)},
			{"q above 1", 2, math.Inf(1)},
		})
	})
}

func TestQuantileEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		bounds  []float64
		observe []float64
		q       float64
		want    float64
	}{
		{
			// Cumulative 2, 2, 4, 4. Rank 2 is reached first by le=0.1, so
			// the answer is 0.1, not 0.2 from the empty bucket after it.
			name:    "rank on a bucket boundary with an empty bucket after it",
			bounds:  []float64{0.1, 0.2, 0.4},
			observe: []float64{0.05, 0.05, 0.3, 0.3},
			q:       0.5,
			want:    0.1,
		},
		{
			// Cumulative 1, 2, 2, 2, 2. Rank 2 is reached first by le=0.25.
			name:    "q=1 with an empty +Inf bucket gives the top of the highest non-empty bucket",
			bounds:  []float64{0.1, 0.25, 0.5, 1},
			observe: []float64{0.05, 0.2},
			q:       1,
			want:    0.25,
		},
		{
			name:    "all observations above the highest bound",
			bounds:  []float64{0.1, 0.5},
			observe: []float64{1, 2, 3},
			q:       0.5,
			want:    0.5,
		},
		{
			// The histogram does not know where in a bucket the values were.
			// All 4 are at 0.11, near the bottom, yet rank 2 of 4 interpolates
			// to the middle: 0.1 + 0.15 * 2/4.
			name:    "only the counts matter, not the values",
			bounds:  []float64{0.1, 0.25},
			observe: []float64{0.11, 0.11, 0.11, 0.11},
			q:       0.5,
			want:    0.175,
		},
		{
			name:    "single bound",
			bounds:  []float64{1},
			observe: []float64{0.5, 0.5, 0.5, 0.5},
			q:       0.5,
			want:    0.5,
		},
		// Negative bounds, for example for the clock offset between two
		// hosts in seconds. Cumulative 2, 4, 4, 4.
		{
			// Rank 1 in le=-0.01, the first bucket: no interpolation from 0.
			name:    "first bound <= 0: a rank in the first bucket gives that bound",
			bounds:  []float64{-0.01, 0, 0.01},
			observe: []float64{-0.02, -0.015, -0.005, -0.005},
			q:       0.25,
			want:    -0.01,
		},
		{
			// Rank 3 in le=0, the second bucket: -0.01 + 0.01 * (3-2)/2.
			// Only the first bucket is special; this one interpolates from
			// the previous bound, which is negative.
			name:    "first bound <= 0: later buckets interpolate as usual",
			bounds:  []float64{-0.01, 0, 0.01},
			observe: []float64{-0.02, -0.015, -0.005, -0.005},
			q:       0.75,
			want:    -0.005,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHistogram(t, tc.bounds...)
			observe(h, tc.observe...)
			if got := h.Quantile(tc.q); !sameFloat(got, tc.want) {
				t.Errorf("Quantile(%v) = %v, want %v\n buckets: %v", tc.q, got, tc.want, h.Buckets())
			}
		})
	}
}

// The ten requests of the README, measured exactly and with a histogram with
// the Prometheus default buckets. The histogram knows only that 8 requests
// took between 10 and 25 ms, so its p50 lands in the middle of that range.
func Example_histogramVersusExact() {
	samples := []time.Duration{
		12 * time.Millisecond, 15 * time.Millisecond, 11 * time.Millisecond, 90 * time.Millisecond,
		13 * time.Millisecond, 14 * time.Millisecond, 16 * time.Millisecond, 250 * time.Millisecond,
		13 * time.Millisecond, 12 * time.Millisecond,
	}
	s, err := latency.Summarize(samples)
	if err != nil {
		fmt.Println(err)
		return
	}
	h, err := latency.NewHistogram(defBuckets)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, d := range samples {
		h.Observe(d.Seconds())
	}

	fmt.Printf("mean %v\n", s.Mean)
	fmt.Printf("p50  exact %-6v histogram %.1fms\n", s.P50, h.Quantile(0.5)*1000)
	fmt.Printf("p90  exact %-6v histogram %.1fms\n", s.P90, h.Quantile(0.9)*1000)
	fmt.Printf("p99  exact %-6v histogram %.1fms\n", s.P99, h.Quantile(0.99)*1000)
	// Output:
	// mean 44.6ms
	// p50  exact 13ms   histogram 19.4ms
	// p90  exact 90ms   histogram 100.0ms
	// p99  exact 250ms  histogram 235.0ms
}
