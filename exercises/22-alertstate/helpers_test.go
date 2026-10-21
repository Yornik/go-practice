package alertstate_test

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	alertstate "github.com/yornik/go-practice/exercises/22-alertstate"
)

// Short names for the states keep the scripts readable.
const (
	inactive = alertstate.Inactive
	pending  = alertstate.Pending
	firing   = alertstate.Firing
)

// hangLimit is how long guard lets a single test run. A correct solution
// needs well under a millisecond per test.
const hangLimit = 5 * time.Second

// t0 is where every fake clock starts.
var t0 = time.Date(2026, 12, 3, 23, 0, 0, 0, time.UTC)

// fakeClock is the clock the tests pass to New. Time stands still until the
// test moves it. It is safe for concurrent use.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock { return &fakeClock{now: t0} }

// Now is the method value the tests pass as the now argument: clock.Now.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d (backwards if d is negative).
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// since describes a time for failure messages: "t0+2m30s" or "t0-1m0s".
func since(t time.Time) string {
	d := t.Sub(t0)
	if d < 0 {
		return fmt.Sprintf("t0-%v", -d)
	}
	return fmt.Sprintf("t0+%v", d)
}

// guard turns the 2 ways a wrong solution can break the test binary into
// readable failures. Use it as the first line of a test or subtest:
// defer guard(t)().
//
//   - A panic, such as the "TODO: implement" of a method you have not
//     written yet, becomes a failure of this test, and the other tests
//     still run.
//   - A call that never returns, for example because a method locks a
//     sync.Mutex that the same goroutine already holds, would hang until the
//     10-minute test timeout. After hangLimit, guard prints the stack of
//     every goroutine, so you can see where it is stuck, and stops the test
//     binary.
func guard(t *testing.T) func() {
	t.Helper()
	name := t.Name()
	watchdog := time.AfterFunc(hangLimit, func() {
		var stacks bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
		fmt.Fprintf(os.Stderr, "\n%s is still running after %v: a call is blocked "+
			"(does a method lock a mutex that its goroutine already holds?).\n"+
			"Stacks of all goroutines:\n\n%s\n", name, hangLimit, stacks.String())
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

// harness is an Alert driven by a fake clock. Its methods call the Alert and
// fail the test, with the time and the rule in the message, if the result
// is wrong.
type harness struct {
	t     *testing.T
	rule  alertstate.Rule
	clock *fakeClock
	a     *alertstate.Alert
}

func newHarness(t *testing.T, rule alertstate.Rule) *harness {
	t.Helper()
	clock := newClock()
	a, err := alertstate.New(rule, clock.Now)
	if err != nil || a == nil {
		t.Fatalf("New(%+v, clock) = %v, %v; want an Alert and nil", rule, a, err)
	}
	return &harness{t: t, rule: rule, clock: clock, a: a}
}

// eval moves the clock by after, calls Eval(active) and expects want, both
// from Eval and from State afterwards.
func (h *harness) eval(after time.Duration, active bool, want alertstate.State) {
	h.t.Helper()
	h.clock.Advance(after)
	at := since(h.clock.Now())
	got := h.a.Eval(active)
	if got != want {
		h.t.Fatalf("rule %+v at %s: Eval(%v) = %v, want %v", h.rule, at, active, got, want)
	}
	if s := h.a.State(); s != got {
		h.t.Fatalf("rule %+v at %s: Eval(%v) returned %v, but State() = %v afterwards", h.rule, at, active, got, s)
	}
}

// activeAt expects ActiveAt() to be t0 plus d.
func (h *harness) activeAt(d time.Duration) {
	h.t.Helper()
	want := t0.Add(d)
	if got := h.a.ActiveAt(); !got.Equal(want) {
		h.t.Errorf("rule %+v at %s in state %v: ActiveAt() = %s, want %s",
			h.rule, since(h.clock.Now()), h.a.State(), describe(got), since(want))
	}
}

// noActiveAt expects ActiveAt() to be the zero time.
func (h *harness) noActiveAt() {
	h.t.Helper()
	if got := h.a.ActiveAt(); !got.IsZero() {
		h.t.Errorf("rule %+v at %s in state %v: ActiveAt() = %s, want the zero time",
			h.rule, since(h.clock.Now()), h.a.State(), describe(got))
	}
}

// describe prints a time relative to t0, or "the zero time".
func describe(t time.Time) string {
	if t.IsZero() {
		return "the zero time"
	}
	return since(t)
}

// step is 1 evaluation of a script: move the clock by after, call
// Eval(active), expect want.
type step struct {
	after  time.Duration
	active bool
	want   alertstate.State
}

type scriptCase struct {
	name  string
	rule  alertstate.Rule
	steps []step
}

// runScripts runs each script on a new Alert whose clock starts at t0.
func runScripts(t *testing.T, cases []scriptCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer guard(t)()
			h := newHarness(t, tc.rule)
			for _, s := range tc.steps {
				h.eval(s.after, s.active, s.want)
			}
		})
	}
}

// tr builds the Transition at t0 plus d.
func tr(d time.Duration, from, to alertstate.State) alertstate.Transition {
	return alertstate.Transition{At: t0.Add(d), From: from, To: to}
}

// sameHistory compares 2 histories. It compares times with Time.Equal.
func sameHistory(got, want []alertstate.Transition) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !got[i].At.Equal(want[i].At) || got[i].From != want[i].From || got[i].To != want[i].To {
			return false
		}
	}
	return true
}

// formatHistory prints a history with times relative to t0.
func formatHistory(h []alertstate.Transition) string {
	if len(h) == 0 {
		return "[]"
	}
	var b bytes.Buffer
	b.WriteString("[")
	for i, x := range h {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "{%s %v→%v}", since(x.At), x.From, x.To)
	}
	b.WriteString("]")
	return b.String()
}
