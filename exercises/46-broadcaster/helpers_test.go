package broadcaster_test

import (
	"fmt"
	"testing"
	"time"

	broadcaster "github.com/yornik/go-practice/exercises/46-broadcaster"
)

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// window is how long a test watches for something that must NOT happen, such
// as an extra event arriving. A correct solution never does it, so the window
// only decides how likely it is that a wrong solution gets caught. It never
// makes a correct solution fail.
const window = 50 * time.Millisecond

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics or does not return within the timeout. Use it for every call that
// a wrong solution could block or panic in.
func within(t *testing.T, what string, f func()) {
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
		t.Fatalf("%s did not return within %v (is it blocked?)", what, timeout)
	}
}

// newB calls broadcaster.New and fails the test if it panics.
func newB[T any](t *testing.T, history int) *broadcaster.Broadcaster[T] {
	t.Helper()
	var b *broadcaster.Broadcaster[T]
	within(t, fmt.Sprintf("New(%d)", history), func() { b = broadcaster.New[T](history) })
	return b
}

// publish calls b.Publish(v) and fails the test if it returns an error,
// panics or blocks. It returns the sequence number.
func publish[T any](t *testing.T, b *broadcaster.Broadcaster[T], v T) uint64 {
	t.Helper()
	var seq uint64
	var err error
	within(t, fmt.Sprintf("Publish(%v)", v), func() { seq, err = b.Publish(v) })
	if err != nil {
		t.Fatalf("Publish(%v) = %d, %v, want no error", v, seq, err)
	}
	return seq
}

// publishN publishes the values from..to (inclusive) in order.
func publishN(t *testing.T, b *broadcaster.Broadcaster[int], from, to int) {
	t.Helper()
	for v := from; v <= to; v++ {
		publish(t, b, v)
	}
}

// subscribe calls b.Subscribe and fails the test if it panics or blocks.
func subscribe[T any](t *testing.T, b *broadcaster.Broadcaster[T], buf int, p broadcaster.Policy) *broadcaster.Subscription[T] {
	t.Helper()
	var sub *broadcaster.Subscription[T]
	within(t, fmt.Sprintf("Subscribe(%d, %v)", buf, p), func() { sub = b.Subscribe(buf, p) })
	if sub == nil {
		t.Fatalf("Subscribe(%d, %v) = nil, want a subscription", buf, p)
	}
	return sub
}

// subscribeFrom calls b.SubscribeFrom and fails the test if it panics or
// blocks. It returns what SubscribeFrom returned.
func subscribeFrom[T any](t *testing.T, b *broadcaster.Broadcaster[T], after uint64, buf int, p broadcaster.Policy) (*broadcaster.Subscription[T], error) {
	t.Helper()
	var sub *broadcaster.Subscription[T]
	var err error
	within(t, fmt.Sprintf("SubscribeFrom(%d, %d, %v)", after, buf, p), func() { sub, err = b.SubscribeFrom(after, buf, p) })
	return sub, err
}

// mustSubscribeFrom is subscribeFrom for calls that must succeed.
func mustSubscribeFrom[T any](t *testing.T, b *broadcaster.Broadcaster[T], after uint64, buf int, p broadcaster.Policy) *broadcaster.Subscription[T] {
	t.Helper()
	sub, err := subscribeFrom(t, b, after, buf, p)
	if err != nil || sub == nil {
		t.Fatalf("SubscribeFrom(%d, %d, %v) = %v, %v, want a subscription and no error", after, buf, p, sub, err)
	}
	return sub
}

// unsubscribe calls sub.Unsubscribe and fails the test if it panics or blocks.
func unsubscribe[T any](t *testing.T, sub *broadcaster.Subscription[T]) {
	t.Helper()
	within(t, "Unsubscribe()", sub.Unsubscribe)
}

// closeB calls b.Close and fails the test if it panics or blocks.
func closeB[T any](t *testing.T, b *broadcaster.Broadcaster[T]) {
	t.Helper()
	within(t, "Close()", b.Close)
}

// subscribers calls b.Subscribers and fails the test if it panics or blocks.
func subscribers[T any](t *testing.T, b *broadcaster.Broadcaster[T]) int {
	t.Helper()
	var n int
	within(t, "Subscribers()", func() { n = b.Subscribers() })
	return n
}

// errOf calls sub.Err and fails the test if it panics or blocks.
func errOf[T any](t *testing.T, sub *broadcaster.Subscription[T]) error {
	t.Helper()
	var err error
	within(t, "Err()", func() { err = sub.Err() })
	return err
}

// dropped calls sub.Dropped and fails the test if it panics or blocks.
func dropped[T any](t *testing.T, sub *broadcaster.Subscription[T]) uint64 {
	t.Helper()
	var n uint64
	within(t, "Dropped()", func() { n = sub.Dropped() })
	return n
}

// channel calls sub.C and fails the test if it panics or returns nil.
func channel[T any](t *testing.T, sub *broadcaster.Subscription[T]) <-chan broadcaster.Event[T] {
	t.Helper()
	var c <-chan broadcaster.Event[T]
	within(t, "C()", func() { c = sub.C() })
	if c == nil {
		t.Fatalf("C() = nil, want a channel")
	}
	return c
}

// recvN receives exactly n events from sub.C(). It fails the test if C is
// closed before that or an event does not arrive within the timeout.
func recvN[T any](t *testing.T, sub *broadcaster.Subscription[T], n int) []broadcaster.Event[T] {
	t.Helper()
	c := channel(t, sub)
	var got []broadcaster.Event[T]
	for len(got) < n {
		select {
		case ev, ok := <-c:
			if !ok {
				t.Fatalf("C was closed after %d events (seqs %v), want %d events", len(got), seqs(got), n)
			}
			got = append(got, ev)
		case <-time.After(timeout):
			t.Fatalf("received %d events (seqs %v) within %v, want %d", len(got), seqs(got), timeout, n)
		}
	}
	return got
}

// expectNothing fails the test if an event arrives on sub.C(), or C is
// closed, within the window.
func expectNothing[T any](t *testing.T, sub *broadcaster.Subscription[T]) {
	t.Helper()
	c := channel(t, sub)
	select {
	case ev, ok := <-c:
		if !ok {
			t.Fatalf("C was closed, want it open with nothing to receive")
		}
		t.Fatalf("received event %d (%v), want nothing", ev.Seq, ev.Value)
	case <-time.After(window):
	}
}

// drain receives from sub.C() until it is closed and returns every event it
// received. It fails the test if C is not closed within the timeout.
func drain[T any](t *testing.T, sub *broadcaster.Subscription[T]) []broadcaster.Event[T] {
	t.Helper()
	c := channel(t, sub)
	var got []broadcaster.Event[T]
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-c:
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-deadline:
			t.Fatalf("C was not closed within %v (received seqs %v so far)", timeout, seqs(got))
		}
	}
}

// seqs returns the sequence numbers of events.
func seqs[T any](events []broadcaster.Event[T]) []uint64 {
	out := make([]uint64, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Seq)
	}
	return out
}

// seqRange returns []uint64{from, from+1, ..., to}, or an empty slice if
// from > to.
func seqRange(from, to uint64) []uint64 {
	out := []uint64{}
	for s := from; s <= to; s++ {
		out = append(out, s)
	}
	return out
}
