package counters_test

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	counters "github.com/yornik/go-practice/exercises/20-counters"
)

// timeout bounds every wait in these tests: for a call to return, for
// goroutines to finish. A correct solution needs microseconds per call; the
// timeout only catches a lock that is never released.
const timeout = 5 * time.Second

// workTimeout bounds the wait for the goroutines of parallel, which do a lot
// of work. A correct solution needs about 1 s under -race; the limit only
// catches a hang.
const workTimeout = 30 * time.Second

// hangLimit is how long guard lets a whole test run.
const hangLimit = 60 * time.Second

// guard turns 2 ways a wrong solution can break the test binary into
// readable failures. Use it as the first line of a test: defer guard(t)().
//
//   - A panic in the test's own goroutine, such as "TODO: implement",
//     becomes a failure of this test with the stack, and the other tests
//     still run.
//   - If the test is still running after hangLimit, guard prints the stack
//     of every goroutine, so you can see where it is stuck, and stops the
//     test binary.
func guard(t *testing.T) func() {
	t.Helper()
	name := t.Name()
	watchdog := time.AfterFunc(hangLimit, func() {
		fmt.Fprintf(os.Stderr, "\n%s is still running after %v: a call is blocked.\n"+
			"Stacks of all goroutines:\n\n%s\n", name, hangLimit, stacks())
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

// stacks returns the stacks of all goroutines, grouped.
func stacks() string {
	var b bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&b, 1)
	return b.String()
}

// async runs f in a new goroutine. The channel receives the value f panicked
// with, or nil when f returns normally.
func async(f func()) <-chan any {
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	return done
}

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics or does not return within the timeout.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	select {
	case v := <-async(f):
		if v != nil {
			t.Fatalf("%s panicked: %v", what, v)
		}
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v (is a lock held, or never released?). "+
			"Stacks of all goroutines:\n%s", what, timeout, stacks())
	}
}

// newRegistry calls NewRegistry and fails the test if it returns nil.
func newRegistry(t *testing.T) *counters.Registry {
	t.Helper()
	var r *counters.Registry
	within(t, "NewRegistry()", func() { r = counters.NewRegistry() })
	if r == nil {
		t.Fatalf("NewRegistry() = nil, want a registry")
	}
	return r
}

// counter calls r.Counter(name) and fails the test if it returns nil.
func counter(t *testing.T, r *counters.Registry, name string) *counters.Counter {
	t.Helper()
	var c *counters.Counter
	within(t, fmt.Sprintf("Counter(%q)", name), func() { c = r.Counter(name) })
	if c == nil {
		t.Fatalf("Counter(%q) = nil, want a counter", name)
	}
	return c
}

func add(t *testing.T, c *counters.Counter, n int64) {
	t.Helper()
	within(t, fmt.Sprintf("Add(%d)", n), func() { c.Add(n) })
}

func value(t *testing.T, c *counters.Counter) int64 {
	t.Helper()
	var v int64
	within(t, "Value()", func() { v = c.Value() })
	return v
}

func snapshot(t *testing.T, r *counters.Registry) []counters.Sample {
	t.Helper()
	var s []counters.Sample
	within(t, "Snapshot()", func() { s = r.Snapshot() })
	return s
}

func drain(t *testing.T, r *counters.Registry) []counters.Sample {
	t.Helper()
	var s []counters.Sample
	within(t, "Drain()", func() { s = r.Drain() })
	return s
}

// write calls r.Write with a bytes.Buffer and returns the text and the error.
func write(t *testing.T, r *counters.Registry) (string, error) {
	t.Helper()
	var b bytes.Buffer
	var err error
	within(t, "Write(w)", func() { err = r.Write(&b) })
	return b.String(), err
}

// wantValue fails the test unless c.Value() returns want.
func wantValue(t *testing.T, c *counters.Counter, name string, want int64, after string) {
	t.Helper()
	if got := value(t, c); got != want {
		t.Errorf("after %s: Counter(%q).Value() = %d, want %d", after, name, got, want)
	}
}

// wantSamples fails the test unless got equals want, in this order.
func wantSamples(t *testing.T, call string, got []counters.Sample, want ...counters.Sample) {
	t.Helper()
	if !equalSamples(got, want) {
		t.Errorf("%s = %s,\nwant %s", call, show(got), show(want))
	}
}

func equalSamples(a, b []counters.Sample) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// s is a short way to write an expected Sample.
func s(name string, value int64) counters.Sample {
	return counters.Sample{Name: name, Value: value}
}

// show formats samples for failure messages: [{"a" 1} {"b" 2}].
func show(samples []counters.Sample) string {
	parts := make([]string, len(samples))
	for i, x := range samples {
		parts[i] = fmt.Sprintf("{%q %d}", x.Name, x.Value)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// minProcs is the GOMAXPROCS that parallel sets at least. Some concurrency
// bugs are not data races, so -race cannot report them; only goroutines that
// really run at the same time show them. With GOMAXPROCS=1, as on a 1-CPU
// container or with -cpu 1, the goroutines would take turns instead.
const minProcs = 4

// parallel runs f(0), ..., f(n-1) in n goroutines, released at the same
// moment by 1 closed channel so that they really overlap, and waits for all
// of them. It raises GOMAXPROCS to at least minProcs for the rest of the
// test. It fails the test if one of them panics or if they do not all return
// within workTimeout.
func parallel(t *testing.T, n int, f func(i int)) {
	t.Helper()
	if old := runtime.GOMAXPROCS(0); old < minProcs {
		runtime.GOMAXPROCS(minProcs)
		t.Cleanup(func() { runtime.GOMAXPROCS(old) })
	}
	start := make(chan struct{})
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		panics []any
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					mu.Lock()
					panics = append(panics, v)
					mu.Unlock()
				}
			}()
			<-start
			f(i)
		}()
	}
	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(workTimeout):
		t.Fatalf("%d goroutines did not all return within %v: a call is blocked. "+
			"Stacks of all goroutines:\n%s", n, workTimeout, stacks())
	}
	if len(panics) > 0 {
		t.Fatalf("%d of %d goroutines panicked, for example with: %v", len(panics), n, panics[0])
	}
}
