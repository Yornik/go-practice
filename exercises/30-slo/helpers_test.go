package slo_test

import (
	"math"
	"slices"
	"testing"
	"time"

	slo "github.com/yornik/go-practice/exercises/30-slo"
)

const (
	day   = 24 * time.Hour
	month = 30 * day // the SLO period in most tests
)

// t0 is "now" in most tests. It is a whole minute, so per-minute buckets line
// up with it: the bucket that starts at t0-1m is the last complete minute.
var t0 = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// slo999 is the SLO of most tests: 99.9 % of requests are good over 30 days.
// Its error budget is 0.1 % of the requests.
var slo999 = slo.SLO{Objective: 0.999, Period: month}

// bucket returns one bucket with total requests, of which bad failed.
func bucket(start time.Time, total, bad int64) slo.Bucket {
	return slo.Bucket{Start: start, Good: total - bad, Total: total}
}

// minutes returns n buckets, one per minute, the first one starting at from.
// Each has total requests, of which bad failed.
func minutes(from time.Time, n int, total, bad int64) []slo.Bucket {
	out := make([]slo.Bucket, n)
	for i := range n {
		out[i] = bucket(from.Add(time.Duration(i)*time.Minute), total, bad)
	}
	return out
}

// approxEqual compares floats with a tolerance of 1e-9 (relative for values
// above 1). Float results depend on the order of the operations, so the last
// bits may differ from the number on paper. NaN is never equal to anything.
func approxEqual(got, want float64) bool {
	return math.Abs(got-want) <= 1e-9*math.Max(1, math.Abs(want))
}

// durationClose reports whether got is within 1 µs of want. It compares in
// float64 so that a wildly wrong negative got cannot wrap around.
func durationClose(got, want time.Duration) bool {
	return math.Abs(float64(got)-float64(want)) <= float64(time.Microsecond)
}

// catchPanic turns a panic in the test's own goroutine, such as the
// "TODO: implement" of a method you have not written yet, into a test
// failure, so the other tests still run. Use it as: defer catchPanic(t).
func catchPanic(t *testing.T) {
	t.Helper()
	if v := recover(); v != nil {
		t.Fatalf("panicked: %v", v)
	}
}

// checkUnchanged fails the test if the code under test modified the caller's
// buckets, for example by sorting them in place.
func checkUnchanged(t *testing.T, got, before []slo.Bucket) {
	t.Helper()
	if !slices.Equal(got, before) {
		t.Errorf("the buckets slice was modified by the call; the caller's data must stay as it was")
	}
}
