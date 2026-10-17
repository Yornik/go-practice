package latency_test

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	latency "github.com/yornik/go-practice/exercises/03-latency"
)

// ms converts whole milliseconds to durations: ms(12, 15) is [12ms 15ms].
func ms(values ...int) []time.Duration {
	out := make([]time.Duration, len(values))
	for i, v := range values {
		out[i] = time.Duration(v) * time.Millisecond
	}
	return out
}

// tenRequests returns the README example, a fresh slice on every call.
// Sorted it is 11 12 12 13 13 14 15 16 90 250 ms.
func tenRequests() []time.Duration {
	return ms(12, 15, 11, 90, 13, 14, 16, 250, 13, 12)
}

// sevenRequests returns 10 20 30 40 50 60 70 ms in a shuffled order.
func sevenRequests() []time.Duration {
	return ms(70, 10, 60, 20, 50, 30, 40)
}

// shuffled returns 1*unit, 2*unit, ..., n*unit in a fixed pseudo-random order.
// The k-th smallest sample is k*unit, which makes expected values easy to read.
func shuffled(n int, unit time.Duration) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = time.Duration(i+1) * unit
	}
	r := rand.New(rand.NewPCG(3, 3))
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// show prints short sample slices in full and long ones as a count, so that
// failure messages stay readable.
func show(samples []time.Duration) string {
	if len(samples) > 12 {
		return fmt.Sprintf("<%d samples>", len(samples))
	}
	return fmt.Sprint(samples)
}

// ---- Percentile -----------------------------------------------------------

func TestPercentile(t *testing.T) {
	tests := []struct {
		name    string
		samples []time.Duration
		p       float64
		want    time.Duration
	}{
		// Sorted: 11 12 12 13 13 14 15 16 90 250 ms, N = 10.
		{"p50 of 10 is rank 5", tenRequests(), 50, 13 * time.Millisecond},
		{"p90 of 10 is rank 9", tenRequests(), 90, 90 * time.Millisecond},
		{"p99 of 10: rank 9.9 rounds up to 10", tenRequests(), 99, 250 * time.Millisecond},
		{"p95 of 10: rank 9.5 rounds up to 10", tenRequests(), 95, 250 * time.Millisecond},
		{"p10 of 10 is rank 1", tenRequests(), 10, 11 * time.Millisecond},
		{"p30 of 10 is rank 3, duplicates count one by one", tenRequests(), 30, 12 * time.Millisecond},
		{"p40 of 10 is rank 4", tenRequests(), 40, 13 * time.Millisecond},
		{"p100 is the maximum", tenRequests(), 100, 250 * time.Millisecond},
		{"tiny p is the minimum", tenRequests(), 0.001, 11 * time.Millisecond},

		// Sorted: 10 20 30 40 50 60 70 ms, N = 7.
		{"p50 of 7: rank 3.5 rounds up to 4", sevenRequests(), 50, 40 * time.Millisecond},
		{"p90 of 7: rank 6.3 rounds up to 7", sevenRequests(), 90, 70 * time.Millisecond},
		{"p10 of 7: rank 0.7 rounds up to 1", sevenRequests(), 10, 10 * time.Millisecond},
		{"p25 of 7: rank 1.75 rounds up to 2", sevenRequests(), 25, 20 * time.Millisecond},

		{"one sample, p50", ms(42), 50, 42 * time.Millisecond},
		{"one sample, p100", ms(42), 100, 42 * time.Millisecond},
		{"one sample, tiny p", ms(42), 0.1, 42 * time.Millisecond},
		{"all samples equal", ms(5, 5, 5, 5), 75, 5 * time.Millisecond},

		// 1 ms to 1000 ms in a shuffled order: the rank-k sample is k ms.
		{"p50 of 1000", shuffled(1000, time.Millisecond), 50, 500 * time.Millisecond},
		{"p90 of 1000", shuffled(1000, time.Millisecond), 90, 900 * time.Millisecond},
		{"p99 of 1000", shuffled(1000, time.Millisecond), 99, 990 * time.Millisecond},

		// float64 cannot hold 99.9 exactly. Computed naively, the rank comes
		// out a hair above the whole number and math.Ceil adds 1.
		{"p99.9 of 1000 is rank 999, not 1000", shuffled(1000, time.Millisecond), 99.9, 999 * time.Millisecond},
		{"p99.9 of 41000 is rank 40959, not 40960", shuffled(41000, time.Microsecond), 99.9, 40959 * time.Microsecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := latency.Percentile(tc.samples, tc.p)
			if err != nil {
				t.Fatalf("Percentile(%s, %v) error = %v, want nil", show(tc.samples), tc.p, err)
			}
			if got != tc.want {
				t.Errorf("Percentile(%s, %v) = %v, want %v", show(tc.samples), tc.p, got, tc.want)
			}
		})
	}
}

func TestPercentileErrors(t *testing.T) {
	badP := []float64{0, -1, -50, 100.000001, 101, 1000, math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, p := range badP {
		t.Run(fmt.Sprintf("p=%v", p), func(t *testing.T) {
			_, err := latency.Percentile(tenRequests(), p)
			if !errors.Is(err, latency.ErrInvalidPercentile) {
				t.Errorf("Percentile(samples, %v) error = %v, want an error that wraps ErrInvalidPercentile", p, err)
			}
		})
	}

	noSamples := map[string][]time.Duration{"nil": nil, "empty": {}}
	for name, samples := range noSamples {
		t.Run(name+" samples", func(t *testing.T) {
			_, err := latency.Percentile(samples, 50)
			if !errors.Is(err, latency.ErrNoSamples) {
				t.Errorf("Percentile(%#v, 50) error = %v, want ErrNoSamples", samples, err)
			}
		})
	}
}

// The caller may still need its samples in their original order, for example
// to plot them over time.
func TestPercentileDoesNotModifyInput(t *testing.T) {
	samples := tenRequests()
	before := slices.Clone(samples)
	for _, p := range []float64{50, 99, 100} {
		if _, err := latency.Percentile(samples, p); err != nil {
			t.Fatalf("Percentile(%v, %v) error = %v, want nil", before, p, err)
		}
		if !slices.Equal(samples, before) {
			t.Fatalf("Percentile(samples, %v) modified its input:\n got  %v\n want %v", p, samples, before)
		}
	}
}

// TestPercentileDefinition checks nearest rank without the formula, on random
// samples with many duplicates: the result v is one of the samples, at least
// p % of the samples are <= v, and fewer than p % of the samples are < v.
// With an integer p, "at least p % of N" is 100*count >= p*N, which is exact.
func TestPercentileDefinition(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 200 {
		n := 1 + r.IntN(60)
		samples := make([]time.Duration, n)
		for i := range samples {
			samples[i] = time.Duration(1+r.IntN(20)) * time.Millisecond
		}
		for p := 1; p <= 100; p++ {
			got, err := latency.Percentile(samples, float64(p))
			if err != nil {
				t.Fatalf("Percentile(%v, %d) error = %v, want nil", samples, p, err)
			}
			atMost, below := 0, 0
			for _, s := range samples {
				if s <= got {
					atMost++
				}
				if s < got {
					below++
				}
			}
			switch {
			case !slices.Contains(samples, got):
				t.Fatalf("Percentile(%v, %d) = %v, which is not one of the samples", samples, p, got)
			case 100*atMost < p*n:
				t.Fatalf("Percentile(%v, %d) = %v, but only %d of %d samples are <= it, want at least %d %%",
					samples, p, got, atMost, n, p)
			case 100*below >= p*n:
				t.Fatalf("Percentile(%v, %d) = %v, but %d of %d samples are < it, want fewer than %d %% (a smaller sample is the answer)",
					samples, p, got, below, n, p)
			}
		}
	}
}

// ---- Summarize --------------------------------------------------------------

func TestSummarize(t *testing.T) {
	tests := []struct {
		name    string
		samples []time.Duration
		want    latency.Summary
	}{
		{
			name:    "ten requests",
			samples: tenRequests(),
			want: latency.Summary{
				Count: 10,
				Min:   11 * time.Millisecond,
				Max:   250 * time.Millisecond,
				Mean:  44600 * time.Microsecond, // 446 ms / 10
				P50:   13 * time.Millisecond,
				P90:   90 * time.Millisecond,
				P99:   250 * time.Millisecond,
			},
		},
		{
			name:    "one sample",
			samples: ms(42),
			want: latency.Summary{
				Count: 1,
				Min:   42 * time.Millisecond, Max: 42 * time.Millisecond, Mean: 42 * time.Millisecond,
				P50: 42 * time.Millisecond, P90: 42 * time.Millisecond, P99: 42 * time.Millisecond,
			},
		},
		{
			name:    "1 to 1000 ms shuffled",
			samples: shuffled(1000, time.Millisecond),
			want: latency.Summary{
				Count: 1000,
				Min:   1 * time.Millisecond,
				Max:   1000 * time.Millisecond,
				Mean:  500500 * time.Microsecond, // 500500 ms / 1000
				P50:   500 * time.Millisecond,
				P90:   900 * time.Millisecond,
				P99:   990 * time.Millisecond,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := latency.Summarize(tc.samples)
			if err != nil {
				t.Fatalf("Summarize(%s) error = %v, want nil", show(tc.samples), err)
			}
			if got != tc.want {
				t.Errorf("Summarize(%s)\n got  %+v\n want %+v", show(tc.samples), got, tc.want)
			}
		})
	}
}

// Mean is the integer division sum / Count of time.Duration values: the
// fraction of a nanosecond is dropped, not rounded.
func TestSummarizeMean(t *testing.T) {
	tests := []struct {
		name    string
		samples []time.Duration
		want    time.Duration
	}{
		{"1ns and 2ns", []time.Duration{1, 2}, 1},
		{"1ms, 2ms and 2ms", ms(1, 2, 2), 1666666}, // 5 ms / 3 = 1666666.67 ns
		{"exact", ms(10, 20, 30, 40), 25 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := latency.Summarize(tc.samples)
			if err != nil {
				t.Fatalf("Summarize(%v) error = %v, want nil", tc.samples, err)
			}
			if got.Mean != tc.want {
				t.Errorf("Summarize(%v).Mean = %v (%d ns), want %v (%d ns)",
					tc.samples, got.Mean, int64(got.Mean), tc.want, int64(tc.want))
			}
		})
	}
}

func TestSummarizeErrors(t *testing.T) {
	noSamples := map[string][]time.Duration{"nil": nil, "empty": {}}
	for name, samples := range noSamples {
		t.Run(name, func(t *testing.T) {
			_, err := latency.Summarize(samples)
			if !errors.Is(err, latency.ErrNoSamples) {
				t.Errorf("Summarize(%#v) error = %v, want ErrNoSamples", samples, err)
			}
		})
	}
}

func TestSummarizeDoesNotModifyInput(t *testing.T) {
	samples := tenRequests()
	before := slices.Clone(samples)
	if _, err := latency.Summarize(samples); err != nil {
		t.Fatalf("Summarize(%v) error = %v, want nil", before, err)
	}
	if !slices.Equal(samples, before) {
		t.Fatalf("Summarize modified its input:\n got  %v\n want %v", samples, before)
	}
}
