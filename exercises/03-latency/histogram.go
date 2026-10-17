package latency

import "errors"

// ErrInvalidBounds is wrapped by the error NewHistogram returns for bucket
// bounds it cannot use. Check for it with errors.Is.
var ErrInvalidBounds = errors.New("latency: invalid bucket bounds")

// Bucket is one bucket of a histogram, as Prometheus exposes it in the series
// <name>_bucket{le="<UpperBound>"}.
type Bucket struct {
	UpperBound float64 // in seconds; math.Inf(1) for the last bucket
	Count      uint64  // cumulative: the number of observations <= UpperBound
}

// Histogram counts observations in buckets with fixed upper bounds, like a
// Prometheus classic histogram. Create one with NewHistogram; the zero value
// is not usable.
//
// A Histogram is not safe for concurrent use.
type Histogram struct {
	// TODO: add fields.
}

// NewHistogram returns an empty histogram with the given upper bounds, in
// seconds. The bounds must be non-empty, finite (no NaN, no +Inf or -Inf) and
// strictly increasing; zero and negative bounds are allowed. Otherwise
// NewHistogram returns an error that wraps ErrInvalidBounds.
//
// Do not pass +Inf: the histogram always adds the +Inf bucket itself. The
// histogram keeps its own copy of the bounds, so the caller may reuse the
// slice afterwards.
func NewHistogram(upperBounds []float64) (*Histogram, error) {
	panic("TODO: implement")
}

// Observe counts one observation, in seconds, in the first bucket whose upper
// bound is >= seconds. A value above the highest bound counts only in the
// +Inf bucket.
func (h *Histogram) Observe(seconds float64) {
	panic("TODO: implement")
}

// Buckets returns one Bucket per upper bound plus the +Inf bucket, in
// increasing order of UpperBound, with cumulative counts. The Count of the
// +Inf bucket is the total number of observations. Every call returns a new
// slice that the caller may modify.
func (h *Histogram) Buckets() []Bucket {
	panic("TODO: implement")
}

// Quantile estimates the q-quantile (0 <= q <= 1) of the observations the way
// PromQL histogram_quantile does: it finds the bucket that holds rank
// q * total and interpolates linearly inside it. The README lists the exact
// steps and the special cases (NaN, -Inf, +Inf).
func (h *Histogram) Quantile(q float64) float64 {
	panic("TODO: implement")
}
