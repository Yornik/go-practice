package bisect_test

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strconv"
	"testing"
	"time"

	bisect "github.com/yornik/go-practice/exercises/14-bisect"
)

// hangLimit is how long guard lets a normal test run. A correct solution
// needs a few milliseconds.
const hangLimit = 5 * time.Second

// guard turns the 2 ways a wrong solution can break the test binary into
// readable failures. Use it as the first line of a test: defer guard(t)().
//
//   - A panic, such as the "TODO: implement" of a function you have not
//     written yet, becomes a failure of this test, and the other tests
//     still run.
//   - A loop that never ends would hang until the 10-minute test timeout.
//     After hangLimit, guard prints the stack of every goroutine, so you
//     can see where it is stuck, and stops the test binary.
func guard(t *testing.T) func() {
	t.Helper()
	return guardFor(t, hangLimit)
}

// guardFor is guard with its own limit, for the speed tests, which may run
// for up to 20 s before they give up on their own.
func guardFor(t *testing.T, limit time.Duration) func() {
	t.Helper()
	name := t.Name()
	watchdog := time.AfterFunc(limit, func() {
		var stacks bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
		fmt.Fprintf(os.Stderr, "\n%s is still running after %v: a call does not return "+
			"(does a loop never move lo or hi?).\nStacks of all goroutines:\n\n%s\n",
			name, limit, stacks.String())
		os.Exit(2)
	})
	return func() {
		t.Helper()
		watchdog.Stop()
		if v := recover(); v != nil {
			t.Fatalf("panicked: %v\n%s", v, debug.Stack())
		}
	}
}

// day0 is the date of the first build in the fixtures.
var day0 = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

// at returns day0 plus the given day, hour and minute, in UTC.
func at(day, hour, minute int) time.Time {
	return day0.Add(time.Duration(day)*24*time.Hour + time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
}

// cet and nyc are fixed zones, so no test depends on the tz database.
var (
	cet = time.FixedZone("CET", 1*60*60)
	nyc = time.FixedZone("EST", -5*60*60)
)

// fixture returns 5 builds, sorted by At. Each call returns a new slice, so
// a test may compare against a fresh copy.
func fixture() []bisect.Build {
	return []bisect.Build{
		{Version: "v2.14.0-b0001", At: at(0, 9, 0)},   // 2 Nov 09:00 UTC
		{Version: "v2.14.0-b0002", At: at(0, 11, 30)}, // 2 Nov 11:30 UTC
		{Version: "v2.14.0-b0003", At: at(1, 8, 15)},  // 3 Nov 08:15 UTC
		{Version: "v2.14.0-b0004", At: at(3, 14, 0)},  // 5 Nov 14:00 UTC
		{Version: "v2.14.0-b0005", At: at(3, 14, 1)},  // 5 Nov 14:01 UTC
	}
}

// versions lists the versions of builds, for comparisons and messages.
func versions(builds []bisect.Build) []string {
	out := []string{}
	for _, b := range builds {
		out = append(out, b.Version)
	}
	return out
}

// sameBuilds reports whether got and want hold the same versions with the
// same instants, in the same order.
func sameBuilds(got, want []bisect.Build) bool {
	return slices.EqualFunc(got, want, func(a, b bisect.Build) bool {
		return a.Version == b.Version && a.At.Equal(b.At)
	})
}

// manyBuilds returns n builds, 1 minute apart from day0 on.
func manyBuilds(n int) []bisect.Build {
	builds := make([]bisect.Build, n)
	for i := range builds {
		builds[i] = bisect.Build{
			Version: "b" + strconv.Itoa(i),
			At:      day0.Add(time.Duration(i) * time.Minute),
		}
	}
	return builds
}

// speedLimit is how long a speed test runs before it gives up. A binary
// search needs about 1 s, or a few seconds with -race.
const speedLimit = 20 * time.Second
