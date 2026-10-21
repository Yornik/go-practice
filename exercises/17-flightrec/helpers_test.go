package flightrec_test

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"testing"
	"time"

	flightrec "github.com/yornik/go-practice/exercises/17-flightrec"
)

// hangLimit is how long guard lets a test run. A correct solution needs a
// few milliseconds per test.
const hangLimit = 5 * time.Second

// guard turns the 2 ways a wrong solution can break the test binary into
// readable failures. Use it as the first line of a test or subtest:
// defer guard(t)().
//
//   - A panic, such as the "TODO: implement" of a method you have not
//     written yet, becomes a failure of this test with the stack, and the
//     other tests still run.
//   - A loop that never ends would hang until the 10-minute test timeout.
//     After hangLimit, guard prints the stack of every goroutine, so you
//     can see where it is stuck, and stops the test binary.
func guard(t *testing.T) func() {
	t.Helper()
	return guardFor(t, hangLimit)
}

// guardFor is guard with a different time limit.
func guardFor(t *testing.T, limit time.Duration) func() {
	t.Helper()
	name := t.Name()
	watchdog := time.AfterFunc(limit, func() {
		var stacks bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
		fmt.Fprintf(os.Stderr, "\n%s is still running after %v: a call does not return "+
			"(does a loop never end?).\nStacks of all goroutines:\n\n%s\n", name, limit, stacks.String())
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

// panicValue calls f and returns what f panicked with, or nil.
func panicValue(f func()) (v any) {
	defer func() { v = recover() }()
	f()
	return nil
}

// newRing returns NewRing[T](size) with the values added in order.
func newRing[T any](size int, values ...T) *flightrec.Ring[T] {
	r := flightrec.NewRing[T](size)
	for _, v := range values {
		r.Add(v)
	}
	return r
}

// seq returns the ints from to through to, in order.
func seq(from, to int) []int {
	var s []int
	for i := from; i <= to; i++ {
		s = append(s, i)
	}
	return s
}
