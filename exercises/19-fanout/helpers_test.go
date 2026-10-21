package fanout_test

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sync/atomic"
	"testing"
	"time"

	fanout "github.com/yornik/go-practice/exercises/19-fanout"
)

// timeout bounds every wait in these tests: for RunAll or Stream to return,
// for a check to finish, for a result on the channel, for goroutines to end.
// A correct solution needs microseconds; the timeout only catches a hang.
const timeout = 5 * time.Second

// hangLimit is how long guard lets a whole test run.
const hangLimit = 30 * time.Second

// services are the names the checks in these tests use.
var services = []string{
	"checkout", "orders", "payments", "photo-upload", "species-labeller", "push",
	"feeder-gateway", "seed-subscriptions", "search", "auth", "cdn", "status-db",
}

// guard turns 2 ways a wrong solution can break the test binary into
// readable failures. Use it as the first line of a test: defer guard(t)().
//
//   - A panic in the test's own goroutine becomes a failure of this test
//     with the stack, and the other tests still run.
//   - If the test is still running after hangLimit, guard prints the stack
//     of every goroutine, so you can see where it is stuck, and stops the
//     test binary.
func guard(t *testing.T) func() {
	t.Helper()
	name := t.Name()
	watchdog := time.AfterFunc(hangLimit, func() {
		fmt.Fprintf(os.Stderr, "\n%s is still running after %v.\nStacks of all goroutines:\n\n%s\n",
			name, hangLimit, stacks())
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

// runAllCall is 1 call of RunAll in a goroutine of its own. done is closed
// once RunAll has returned or panicked; then results and panicked hold the
// outcome. A closed channel can be watched by several selects, so a test can
// notice an early return or a panic while it waits for something else.
type runAllCall struct {
	done     chan struct{}
	results  []fanout.Result
	panicked any
}

// startRunAll calls RunAll in a new goroutine.
func startRunAll(checks []fanout.Check) *runAllCall {
	c := &runAllCall{done: make(chan struct{})}
	go func() {
		defer func() {
			c.panicked = recover()
			close(c.done)
		}()
		c.results = fanout.RunAll(checks)
	}()
	return c
}

// awaitRunAll waits for the outcome of startRunAll. It fails the test if
// RunAll panicked or did not return within the timeout.
func awaitRunAll(t *testing.T, c *runAllCall) []fanout.Result {
	t.Helper()
	select {
	case <-c.done:
		if c.panicked != nil {
			t.Fatalf("RunAll panicked: %v", c.panicked)
		}
		return c.results
	case <-time.After(timeout):
		t.Fatalf("RunAll did not return within %v. Stacks of all goroutines:\n%s", timeout, stacks())
		return nil
	}
}

// runAll calls RunAll and fails the test if it panics or hangs.
func runAll(t *testing.T, checks []fanout.Check) []fanout.Result {
	t.Helper()
	return awaitRunAll(t, startRunAll(checks))
}

// stream calls Stream and fails the test if it panics, does not return
// within the timeout, or returns a nil channel.
func stream(t *testing.T, checks []fanout.Check) <-chan fanout.Result {
	t.Helper()
	type outcome struct {
		ch       <-chan fanout.Result
		panicked any
	}
	done := make(chan outcome, 1)
	go func() {
		var o outcome
		defer func() {
			o.panicked = recover()
			done <- o
		}()
		o.ch = fanout.Stream(checks)
	}()
	select {
	case o := <-done:
		if o.panicked != nil {
			t.Fatalf("Stream panicked: %v", o.panicked)
		}
		if o.ch == nil {
			t.Fatalf("Stream returned a nil channel; a receive from a nil channel blocks forever")
		}
		return o.ch
	case <-time.After(timeout):
		t.Fatalf("Stream did not return within %v while the checks were still running: "+
			"Stream must return at once, before the checks finish", timeout)
		return nil
	}
}

// receive receives 1 value from ch. It fails the test if nothing arrives
// within the timeout. ok is false when ch is closed.
func receive(t *testing.T, ch <-chan fanout.Result, what string) (r fanout.Result, ok bool) {
	t.Helper()
	select {
	case r, ok = <-ch:
		return r, ok
	case <-time.After(timeout):
		t.Fatalf("nothing arrived on the channel within %v while waiting for %s", timeout, what)
		return fanout.Result{}, false
	}
}

// receiveAll receives n results and then expects the channel to be closed.
func receiveAll(t *testing.T, ch <-chan fanout.Result, n int) []fanout.Result {
	t.Helper()
	var got []fanout.Result
	for len(got) < n {
		r, ok := receive(t, ch, fmt.Sprintf("result %d of %d", len(got)+1, n))
		if !ok {
			t.Fatalf("the channel was closed after %d results, want %d results", len(got), n)
		}
		got = append(got, r)
	}
	expectClosed(t, ch, n)
	return got
}

// expectClosed fails the test unless ch is closed and has no more values.
// n is the number of results received so far.
func expectClosed(t *testing.T, ch <-chan fanout.Result, n int) {
	t.Helper()
	select {
	case r, ok := <-ch:
		if ok {
			t.Fatalf("after %d results the channel sent 1 more: {%q, %v}; want the channel closed", n, r.Name, r.Err)
		}
	case <-time.After(timeout):
		t.Fatalf("the channel was not closed within %v after %d results; "+
			"Stream must close it after the last result, so that a range loop over it ends", timeout, n)
	}
}

// waitGoroutines waits until runtime.NumGoroutine() is back at most at
// before. Goroutines that sent their last value may need a moment to exit,
// which is why it polls. It fails the test, with the stack of every
// goroutine, if the count stays higher than before for the whole timeout.
func waitGoroutines(t *testing.T, before int, after string) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for runtime.NumGoroutine() > before {
		select {
		case <-deadline.C:
			t.Fatalf("%v %s, %d goroutines were running; before the call there were %d. "+
				"A goroutine started by the package did not end. Stacks of all goroutines:\n%s",
				timeout, after, runtime.NumGoroutine(), before, stacks())
		default:
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
	}
}

// probes is a set of checks that the test controls. Run i counts its call,
// waits until the test releases it, records that it returned, and returns
// errs[i]. The test releases the checks one by one, so it decides the order
// in which they finish. When the test ends, every check that is still
// waiting is released, so that a wrong solution cannot hang the next test.
type probes struct {
	checks   []fanout.Check
	errs     []error
	gates    []chan struct{}
	returned []chan struct{}
	calls    []atomic.Int32
	abort    chan struct{}
}

// newProbes creates 1 check per name. errs[i] is what check i returns.
func newProbes(t *testing.T, names []string, errs []error) *probes {
	t.Helper()
	n := len(names)
	p := &probes{
		checks:   make([]fanout.Check, n),
		errs:     errs,
		gates:    make([]chan struct{}, n),
		returned: make([]chan struct{}, n),
		calls:    make([]atomic.Int32, n),
		abort:    make(chan struct{}),
	}
	for i, name := range names {
		p.gates[i] = make(chan struct{})
		p.returned[i] = make(chan struct{}, 1)
		p.checks[i] = fanout.Check{Name: name, Run: func() error {
			p.calls[i].Add(1)
			select {
			case <-p.gates[i]:
			case <-p.abort:
			}
			select {
			case p.returned[i] <- struct{}{}:
			default:
			}
			return p.errs[i]
		}}
	}
	t.Cleanup(func() { close(p.abort) })
	return p
}

// release lets check i return.
func (p *probes) release(i int) { close(p.gates[i]) }

// waitReturned waits until Run of check i has returned. It fails the test
// if RunAll panics or returns before that, or if it takes longer than the
// timeout.
func (p *probes) waitReturned(t *testing.T, i int, call *runAllCall) {
	t.Helper()
	select {
	case <-p.returned[i]:
	case <-call.done:
		if call.panicked != nil {
			t.Fatalf("RunAll panicked: %v", call.panicked)
		}
		// When i is the last check to finish, RunAll may return at the same
		// moment. Run sends on returned[i] before it returns, so that value
		// is already there in a correct solution.
		select {
		case <-p.returned[i]:
		default:
			t.Fatalf("RunAll returned while check %d (%q) was still running; "+
				"RunAll must wait until every Run has returned", i, p.checks[i].Name)
		}
	case <-time.After(timeout):
		t.Fatalf("check %d (%q) was released but did not return within %v (Run was called %d times so far). "+
			"The test releases the checks in an order of its own; this hangs when the checks run "+
			"one after the other instead of all at the same time", i, p.checks[i].Name, timeout, p.calls[i].Load())
	}
}

// wantCalledOnce fails the test unless every Run was called exactly once.
func (p *probes) wantCalledOnce(t *testing.T) {
	t.Helper()
	for i := range p.checks {
		if got := p.calls[i].Load(); got != 1 {
			t.Errorf("Run of check %d (%q) was called %d times, want 1", i, p.checks[i].Name, got)
		}
	}
}

// failures returns an error for every third check, nil for the others.
func failures(names []string) []error {
	errs := make([]error, len(names))
	for i, name := range names {
		if i%3 == 1 {
			errs[i] = fmt.Errorf("%s: GET /healthz: 503 Service Unavailable", name)
		}
	}
	return errs
}

// quick returns checks that return errs[i] at once, and the counters of
// their calls.
func quick(names []string, errs []error) ([]fanout.Check, []atomic.Int32) {
	checks := make([]fanout.Check, len(names))
	calls := make([]atomic.Int32, len(names))
	for i, name := range names {
		checks[i] = fanout.Check{Name: name, Run: func() error {
			calls[i].Add(1)
			return errs[i]
		}}
	}
	return checks, calls
}

// numbered returns n names: "feeder-gateway-1", "feeder-gateway-2", ...
func numbered(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("feeder-gateway-%d", i+1)
	}
	return names
}

// show formats a result for failure messages.
func show(r fanout.Result) string {
	if r.Err == nil {
		return fmt.Sprintf("{%q, nil}", r.Name)
	}
	return fmt.Sprintf("{%q, %q}", r.Name, r.Err)
}
