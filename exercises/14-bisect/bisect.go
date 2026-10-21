// Package bisect finds the first bad firmware build with few tests, and the
// build that was running at a given time.
package bisect

import "time"

// FirstBad returns the index of the first bad build among builds 0 to n-1,
// or n when none is bad.
//
// isBad(i) reports whether build i is bad. All good builds come before all
// bad ones: if build i is bad, every build after i is bad too.
//
// FirstBad calls isBad only with 0 <= i < n, and at most bits.Len(uint(n))
// times: 12 times for 2048 builds. For n <= 0 it returns 0 without a call.
func FirstBad(n int, isBad func(i int) bool) int {
	panic("TODO: implement")
}

// FirstBadErr is FirstBad for a test that can fail. It follows the same
// rules and returns the same index, with a nil error, while test returns no
// error.
//
// When test(i) returns an error, FirstBadErr calls test no more and returns
// -1 and an error that contains "build <i>" and wraps the error of test, so
// that errors.Is finds it. For example:
// "build 1536: flash failed: feeder offline".
func FirstBadErr(n int, test func(i int) (bool, error)) (int, error) {
	panic("TODO: implement")
}

// Build is 1 firmware build and the time it was rolled out.
type Build struct {
	Version string
	At      time.Time // rollout time; builds are sorted by At, strictly increasing
}

// Running returns the newest build with At <= t, and true. It returns the
// zero Build and false when builds is empty or t is before the first build.
//
// builds is sorted by At, strictly increasing. Running compares instants:
// t may be in another time zone than the builds. It takes O(log n) time.
func Running(builds []Build, t time.Time) (Build, bool) {
	panic("TODO: implement")
}

// Between returns the builds with from <= At < to, in order, in a new slice
// that never shares memory with builds. For to <= from it returns an empty
// result.
//
// builds is sorted by At, strictly increasing. Between takes O(log n) time
// to find the window, plus the time to copy the builds it returns.
func Between(builds []Build, from, to time.Time) []Build {
	panic("TODO: implement")
}
