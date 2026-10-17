// Package latency computes latency statistics in two ways: exactly from raw
// samples (Summarize, Percentile), and approximately from a Prometheus-style
// bucketed histogram (Histogram).
package latency

import (
	"errors"
	"time"
)

// ErrNoSamples is returned by Summarize and Percentile when there are no
// samples. Check for it with errors.Is.
var ErrNoSamples = errors.New("latency: no samples")

// ErrInvalidPercentile is wrapped by the error Percentile returns when p is
// outside 0 < p <= 100 (NaN included). Check for it with errors.Is.
var ErrInvalidPercentile = errors.New("latency: percentile out of range")

// Summary describes a set of latency samples.
type Summary struct {
	Count int           // number of samples
	Min   time.Duration // smallest sample
	Max   time.Duration // largest sample
	Mean  time.Duration // sum / Count, truncated to whole nanoseconds

	// The nearest-rank percentiles, exactly as Percentile computes them.
	P50, P90, P99 time.Duration
}

// Summarize returns the Summary of samples. The samples can be in any order.
//
// It returns ErrNoSamples for a nil or empty slice. It does not modify
// samples: the caller's slice has the same elements in the same order after
// the call.
func Summarize(samples []time.Duration) (Summary, error) {
	panic("TODO: implement")
}

// Percentile returns the p-th percentile of samples by the nearest-rank
// method: the sample at rank ceil(p/100 * N) in ascending order (1-based).
// The result is always one of the samples. Valid p: 0 < p <= 100.
//
// The rank is exact for p as written in decimal: p = 99.9 with 1000 samples
// is rank 999, although p/100*N in float64 is a hair above 999.
//
// It returns an error that wraps ErrInvalidPercentile for any other p
// (including NaN and the infinities), and ErrNoSamples for a nil or empty
// slice. It does not modify samples.
func Percentile(samples []time.Duration, p float64) (time.Duration, error) {
	panic("TODO: implement")
}
