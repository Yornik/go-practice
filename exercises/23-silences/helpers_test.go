package silences_test

import (
	"bytes"
	"errors"
	"fmt"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	silences "github.com/yornik/go-practice/exercises/23-silences"
)

// timeout bounds every call into the store. A correct solution returns in
// microseconds; the timeout only catches a call that hangs.
const timeout = 5 * time.Second

// lockHang is the likely cause of a call that does not return.
const lockHang = "is a lock never released, or locked twice by the same goroutine?"

// runs is how many fresh stores a test of a tie or an order uses. Map
// order changes from store to store, so a result that is right only by
// luck fails on 1 of them.
const runs = 20

// t0 is where every fake clock starts.
var t0 = time.Date(2026, 12, 14, 4, 0, 0, 0, time.UTC)

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

// Advance moves the clock forward by d.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// since describes a time for failure messages: "t0+1h0m0s".
func since(t time.Time) string {
	if t.IsZero() {
		return "zero time"
	}
	d := t.Sub(t0)
	if d < 0 {
		return fmt.Sprintf("t0-%v", -d)
	}
	return fmt.Sprintf("t0+%v", d)
}

// sil is a short way to write a silence that ends at t0+until.
func sil(id, alert, target string, until time.Duration) silences.Silence {
	return silences.Silence{ID: id, Alert: alert, Target: target, Until: t0.Add(until)}
}

// show formats a silence for failure messages, with Until relative to t0.
func show(s silences.Silence) string {
	return fmt.Sprintf("{ID:%q Alert:%q Target:%q Until:%s}", s.ID, s.Alert, s.Target, since(s.Until))
}

func showAll(list []silences.Silence) string {
	parts := make([]string, len(list))
	for i, s := range list {
		parts[i] = show(s)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// same reports whether 2 silences are equal. Until is compared with Equal.
func same(a, b silences.Silence) bool {
	return a.ID == b.ID && a.Alert == b.Alert && a.Target == b.Target && a.Until.Equal(b.Until)
}

func sameAll(a, b []silences.Silence) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !same(a[i], b[i]) {
			return false
		}
	}
	return true
}

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics (for example with "TODO: implement"), or if f does not return
// within the timeout. Then it prints the stacks of all goroutines and cause,
// the likely reason.
func within(t *testing.T, what, cause string, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case v := <-done:
		if v != nil {
			t.Fatalf("%s panicked: %v", what, v)
		}
	case <-time.After(timeout):
		var stacks bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
		t.Fatalf("%s did not return within %v: %s\nStacks of all goroutines:\n\n%s", what, timeout, cause, stacks.String())
	}
}

// recorder records the calls of onExpire. Pass its OnExpire method to New.
// It is safe for concurrent use.
type recorder struct {
	mu  sync.Mutex
	got []silences.Silence
}

func (r *recorder) OnExpire(s silences.Silence) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, s)
}

// Take returns the calls recorded since the last Take.
func (r *recorder) Take() []silences.Silence {
	r.mu.Lock()
	defer r.mu.Unlock()
	got := r.got
	r.got = nil
	return got
}

// harness is a Store with a fake clock and a recorder as onExpire. Every
// call goes through within, and failure messages name the clock time.
type harness struct {
	t     *testing.T
	clock *fakeClock
	rec   *recorder
	s     *silences.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, clock: newClock(), rec: &recorder{}}
	within(t, "New(clock.Now, onExpire)", lockHang, func() { h.s = silences.New(h.clock.Now, h.rec.OnExpire) })
	if h.s == nil {
		t.Fatal("New(clock.Now, onExpire) = nil, want a store")
	}
	return h
}

// at describes the clock for failure messages: "at t0+1h0m0s".
func (h *harness) at() string { return "at " + since(h.clock.Now()) }

func (h *harness) add(s silences.Silence) error {
	h.t.Helper()
	var err error
	within(h.t, "Add("+show(s)+")", lockHang, func() { err = h.s.Add(s) })
	return err
}

// mustAdd adds every silence and fails the test if Add returns an error.
func (h *harness) mustAdd(list ...silences.Silence) {
	h.t.Helper()
	for _, s := range list {
		if err := h.add(s); err != nil {
			h.t.Fatalf("%s: Add(%s) = %v, want nil", h.at(), show(s), err)
		}
	}
}

// addInvalid calls Add and fails the test unless the error wraps
// ErrInvalid and contains the ID formatted with %q.
func (h *harness) addInvalid(s silences.Silence) {
	h.t.Helper()
	err := h.add(s)
	if err == nil {
		h.t.Fatalf("%s: Add(%s) = nil, want an error that wraps ErrInvalid", h.at(), show(s))
	}
	if !errors.Is(err, silences.ErrInvalid) {
		h.t.Errorf("%s: Add(%s) = %q, want an error that wraps ErrInvalid (errors.Is)", h.at(), show(s), err)
	}
	if q := fmt.Sprintf("%q", s.ID); !strings.Contains(err.Error(), q) {
		h.t.Errorf("%s: Add(%s) = %q, want the ID %s in the message", h.at(), show(s), err, q)
	}
}

func (h *harness) match(alert, target string) (silences.Silence, bool) {
	h.t.Helper()
	var got silences.Silence
	var ok bool
	within(h.t, fmt.Sprintf("Match(%q, %q)", alert, target), lockHang, func() { got, ok = h.s.Match(alert, target) })
	return got, ok
}

// wantMatch calls Match and fails the test unless it returns want and true.
func (h *harness) wantMatch(alert, target string, want silences.Silence) {
	h.t.Helper()
	got, ok := h.match(alert, target)
	if !ok || !same(got, want) {
		h.t.Errorf("%s: Match(%q, %q) = %s, %v; want %s, true", h.at(), alert, target, show(got), ok, show(want))
	}
}

// wantNoMatch calls Match and fails the test unless it returns the zero
// Silence and false.
func (h *harness) wantNoMatch(alert, target string) {
	h.t.Helper()
	got, ok := h.match(alert, target)
	if ok || got != (silences.Silence{}) {
		h.t.Errorf("%s: Match(%q, %q) = %s, %v; want the zero Silence, false", h.at(), alert, target, show(got), ok)
	}
}

func (h *harness) expire() []silences.Silence {
	h.t.Helper()
	var got []silences.Silence
	within(h.t, "Expire()", lockHang, func() { got = h.s.Expire() })
	return got
}

// wantExpire calls Expire and fails the test unless it returns want, in
// this order. An empty want accepts a nil or an empty slice.
func (h *harness) wantExpire(want ...silences.Silence) {
	h.t.Helper()
	if got := h.expire(); !sameAll(got, want) {
		h.t.Errorf("%s: Expire() = %s, want %s", h.at(), showAll(got), showAll(want))
	}
}

func (h *harness) length() int {
	h.t.Helper()
	var n int
	within(h.t, "Len()", lockHang, func() { n = h.s.Len() })
	return n
}

// wantLen calls Len and fails the test unless it returns want.
func (h *harness) wantLen(want int) {
	h.t.Helper()
	if got := h.length(); got != want {
		h.t.Errorf("%s: Len() = %d, want %d", h.at(), got, want)
	}
}

// wantCalls fails the test unless the onExpire calls since the last check
// are exactly want, in this order.
func (h *harness) wantCalls(after string, want ...silences.Silence) {
	h.t.Helper()
	if got := h.rec.Take(); !sameAll(got, want) {
		h.t.Errorf("%s: after %s: onExpire calls = %s, want %s", h.at(), after, showAll(got), showAll(want))
	}
}
