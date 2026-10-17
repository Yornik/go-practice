package broadcaster_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	broadcaster "github.com/yornik/go-practice/exercises/21-broadcaster"
)

var policies = []broadcaster.Policy{broadcaster.DropOldest, broadcaster.DropNewest, broadcaster.Disconnect}

// ---------------------------------------------------------------------------
// Step 1: Publish and Subscribe (go test -run TestLive)
// ---------------------------------------------------------------------------

func TestLivePublishNumbersEvents(t *testing.T) {
	b := newB[string](t, 0)
	for i, v := range []string{"a", "b", "c"} {
		if got, want := publish(t, b, v), uint64(i+1); got != want {
			t.Errorf("Publish(%q) = %d, want %d (Seq starts at 1 and grows by 1)", v, got, want)
		}
	}
}

func TestLiveSubscribe(t *testing.T) {
	t.Run("a subscriber receives every event in order", func(t *testing.T) {
		b := newB[string](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		values := []string{"deploy started", "pod ready", "deploy done"}
		for _, v := range values {
			publish(t, b, v)
		}
		got := recvN(t, sub, 3)
		for i, ev := range got {
			want := broadcaster.Event[string]{Seq: uint64(i + 1), Value: values[i]}
			if ev != want {
				t.Errorf("event %d = %+v, want %+v", i, ev, want)
			}
		}
		expectNothing(t, sub)
	})

	t.Run("every subscriber receives every event", func(t *testing.T) {
		b := newB[int](t, 0)
		var subs []*broadcaster.Subscription[int]
		for _, p := range policies {
			subs = append(subs, subscribe(t, b, 10, p))
		}
		publishN(t, b, 1, 5)
		for i, sub := range subs {
			got := recvN(t, sub, 5)
			for j, ev := range got {
				if ev.Seq != uint64(j+1) || ev.Value != j+1 {
					t.Errorf("subscriber %d (%v): event %d = %+v, want {Seq:%d Value:%d}", i, policies[i], j, ev, j+1, j+1)
				}
			}
			expectNothing(t, sub)
		}
	})

	t.Run("events published before Subscribe are not delivered", func(t *testing.T) {
		b := newB[int](t, 10) // even with history: Subscribe only gets live events
		publishN(t, b, 1, 2)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		publish(t, b, 3)
		if got := seqs(recvN(t, sub, 1)); !slices.Equal(got, []uint64{3}) {
			t.Errorf("received seqs %v, want [3]", got)
		}
		expectNothing(t, sub)
	})

	t.Run("an active subscription has Err nil and Dropped 0", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		publishN(t, b, 1, 3)
		if err := errOf(t, sub); err != nil {
			t.Errorf("Err() = %v, want nil while the subscription is active", err)
		}
		if n := dropped(t, sub); n != 0 {
			t.Errorf("Dropped() = %d, want 0", n)
		}
	})

	t.Run("Subscribers counts the active subscriptions", func(t *testing.T) {
		b := newB[int](t, 0)
		if n := subscribers(t, b); n != 0 {
			t.Errorf("Subscribers() = %d before any Subscribe, want 0", n)
		}
		for i := range 3 {
			subscribe(t, b, 1, broadcaster.DropOldest)
			if n := subscribers(t, b); n != i+1 {
				t.Errorf("Subscribers() = %d after %d calls of Subscribe, want %d", n, i+1, i+1)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Step 2: slow consumers, buffers and policies (go test -run TestSlow)
// ---------------------------------------------------------------------------

// In these tests the consumer does not read while events are published, so
// the buffer fills up in a known way.

func TestSlowBufferHoldsExactlyBuf(t *testing.T) {
	for _, p := range policies {
		t.Run(p.String(), func(t *testing.T) {
			b := newB[int](t, 0)
			sub := subscribe(t, b, 3, p)
			publishN(t, b, 1, 3) // fills the buffer, but nothing overflows yet
			if n := dropped(t, sub); n != 0 {
				t.Errorf("Dropped() = %d after 3 events into a buffer of 3, want 0", n)
			}
			if n := subscribers(t, b); n != 1 {
				t.Errorf("Subscribers() = %d after 3 events into a buffer of 3, want 1 (nothing overflowed)", n)
			}
			if got := seqs(recvN(t, sub, 3)); !slices.Equal(got, []uint64{1, 2, 3}) {
				t.Errorf("received seqs %v, want [1 2 3]", got)
			}
		})
	}
}

func TestSlowDropOldest(t *testing.T) {
	b := newB[int](t, 0)
	sub := subscribe(t, b, 3, broadcaster.DropOldest)
	publishN(t, b, 1, 10)
	// The policy is applied inside Publish, so Dropped is up to date at once.
	if n := dropped(t, sub); n != 7 {
		t.Errorf("Dropped() = %d after 10 events into a buffer of 3, want 7", n)
	}
	if got := seqs(recvN(t, sub, 3)); !slices.Equal(got, []uint64{8, 9, 10}) {
		t.Errorf("received seqs %v, want [8 9 10] (the 3 newest)", got)
	}
	expectNothing(t, sub)

	// The subscription is still active and receives new events.
	publish(t, b, 11)
	if got := seqs(recvN(t, sub, 1)); !slices.Equal(got, []uint64{11}) {
		t.Errorf("received seqs %v after the next Publish, want [11]", got)
	}
	if n := dropped(t, sub); n != 7 {
		t.Errorf("Dropped() = %d, want still 7", n)
	}
	if err := errOf(t, sub); err != nil {
		t.Errorf("Err() = %v, want nil (dropping does not end the subscription)", err)
	}
	if n := subscribers(t, b); n != 1 {
		t.Errorf("Subscribers() = %d, want 1", n)
	}
}

func TestSlowDropNewest(t *testing.T) {
	b := newB[int](t, 0)
	sub := subscribe(t, b, 3, broadcaster.DropNewest)
	publishN(t, b, 1, 10)
	if n := dropped(t, sub); n != 7 {
		t.Errorf("Dropped() = %d after 10 events into a buffer of 3, want 7", n)
	}
	if got := seqs(recvN(t, sub, 3)); !slices.Equal(got, []uint64{1, 2, 3}) {
		t.Errorf("received seqs %v, want [1 2 3] (the 3 oldest)", got)
	}
	expectNothing(t, sub)

	// Reading made room again.
	publish(t, b, 11)
	if got := seqs(recvN(t, sub, 1)); !slices.Equal(got, []uint64{11}) {
		t.Errorf("received seqs %v after the next Publish, want [11]", got)
	}
	if n := dropped(t, sub); n != 7 {
		t.Errorf("Dropped() = %d, want still 7", n)
	}
	if err := errOf(t, sub); err != nil {
		t.Errorf("Err() = %v, want nil (dropping does not end the subscription)", err)
	}
}

func TestSlowDisconnect(t *testing.T) {
	t.Run("a full buffer ends the subscription with ErrSlowConsumer", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 3, broadcaster.Disconnect)
		publishN(t, b, 1, 10)
		// The 4th Publish ended the subscription before it returned.
		if n := subscribers(t, b); n != 0 {
			t.Errorf("Subscribers() = %d after the buffer overflowed, want 0", n)
		}
		// The consumer still gets what was buffered, then C is closed.
		if got := seqs(drain(t, sub)); !slices.Equal(got, []uint64{1, 2, 3}) {
			t.Errorf("received seqs %v before C was closed, want [1 2 3]", got)
		}
		if err := errOf(t, sub); !errors.Is(err, broadcaster.ErrSlowConsumer) {
			t.Errorf("Err() = %v, want ErrSlowConsumer", err)
		}
		if n := dropped(t, sub); n != 0 {
			t.Errorf("Dropped() = %d, want 0 (Disconnect never drops)", n)
		}
		// The producer is not affected.
		if seq := publish(t, b, 11); seq != 11 {
			t.Errorf("Publish after the disconnect = %d, want 11", seq)
		}
	})

	t.Run("a consumer that keeps up is never disconnected", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 1, broadcaster.Disconnect)
		for i := 1; i <= 100; i++ {
			publish(t, b, i)
			if got := recvN(t, sub, 1); got[0].Seq != uint64(i) {
				t.Fatalf("received seq %d, want %d", got[0].Seq, i)
			}
		}
		if n := subscribers(t, b); n != 1 {
			t.Errorf("Subscribers() = %d, want 1", n)
		}
		if err := errOf(t, sub); err != nil {
			t.Errorf("Err() = %v, want nil", err)
		}
	})
}

func TestSlowBufferAtLeastOne(t *testing.T) {
	// buf < 1 counts as 1. An unbuffered channel would lose every event that
	// arrives while the consumer is not waiting in a receive.
	tests := []struct {
		policy broadcaster.Policy
		want   []uint64 // after publishing 1 and 2
	}{
		{broadcaster.DropOldest, []uint64{2}},
		{broadcaster.DropNewest, []uint64{1}},
		{broadcaster.Disconnect, []uint64{1}},
	}
	for _, buf := range []int{0, -1} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("buf %d %v", buf, tt.policy), func(t *testing.T) {
				b := newB[int](t, 0)
				sub := subscribe(t, b, buf, tt.policy)
				publishN(t, b, 1, 2)
				var got []uint64
				if tt.policy == broadcaster.Disconnect {
					got = seqs(drain(t, sub))
				} else {
					got = seqs(recvN(t, sub, 1))
					expectNothing(t, sub)
				}
				if !slices.Equal(got, tt.want) {
					t.Errorf("Subscribe(%d, %v), Publish 1 and 2: received seqs %v, want %v", buf, tt.policy, got, tt.want)
				}
			})
		}
	}
}

func TestSlowSubscriberDoesNotHurtOthers(t *testing.T) {
	b := newB[int](t, 0)
	fast := subscribe(t, b, 100, broadcaster.DropOldest)
	slowDrop := subscribe(t, b, 2, broadcaster.DropNewest)
	slowDisc := subscribe(t, b, 2, broadcaster.Disconnect)
	publishN(t, b, 1, 10)

	if got := seqs(recvN(t, fast, 10)); !slices.Equal(got, seqRange(1, 10)) {
		t.Errorf("fast subscriber received seqs %v, want 1 to 10", got)
	}
	if n := dropped(t, fast); n != 0 {
		t.Errorf("fast subscriber: Dropped() = %d, want 0", n)
	}
	if got := seqs(recvN(t, slowDrop, 2)); !slices.Equal(got, []uint64{1, 2}) {
		t.Errorf("slow DropNewest subscriber received seqs %v, want [1 2]", got)
	}
	if n := dropped(t, slowDrop); n != 8 {
		t.Errorf("slow DropNewest subscriber: Dropped() = %d, want 8", n)
	}
	if got := seqs(drain(t, slowDisc)); !slices.Equal(got, []uint64{1, 2}) {
		t.Errorf("slow Disconnect subscriber received seqs %v, want [1 2]", got)
	}
	if n := subscribers(t, b); n != 2 {
		t.Errorf("Subscribers() = %d, want 2 (only the Disconnect one ended)", n)
	}
}

func TestSlowPublishNeverBlocks(t *testing.T) {
	b := newB[int](t, 100)
	for _, p := range policies {
		subscribe(t, b, 1, p) // nobody ever reads from these
	}
	subscribe(t, b, 0, broadcaster.DropOldest)
	within(t, "10000 calls of Publish with subscribers that never read", func() {
		for i := 1; i <= 10000; i++ {
			if _, err := b.Publish(i); err != nil {
				panic(err)
			}
		}
	})
	if n := subscribers(t, b); n != 3 {
		t.Errorf("Subscribers() = %d, want 3 (the Disconnect one ended)", n)
	}
}

// ---------------------------------------------------------------------------
// Step 3: ending a subscription (go test -run TestEnd)
// ---------------------------------------------------------------------------

func TestEndUnsubscribe(t *testing.T) {
	t.Run("C is closed after the buffered events and Err is nil", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.Disconnect)
		publishN(t, b, 1, 2)
		unsubscribe(t, sub)
		if n := subscribers(t, b); n != 0 {
			t.Errorf("Subscribers() = %d after Unsubscribe, want 0", n)
		}
		if got := seqs(drain(t, sub)); !slices.Equal(got, []uint64{1, 2}) {
			t.Errorf("received seqs %v, want [1 2]", got)
		}
		if err := errOf(t, sub); err != nil {
			t.Errorf("Err() = %v after Unsubscribe, want nil", err)
		}
	})

	t.Run("no events arrive after Unsubscribe and Publish keeps working", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		publish(t, b, 1)
		unsubscribe(t, sub)
		publishN(t, b, 2, 3) // must not send on the closed channel
		if got := seqs(drain(t, sub)); !slices.Equal(got, []uint64{1}) {
			t.Errorf("received seqs %v, want [1]", got)
		}
	})

	t.Run("Unsubscribe is idempotent", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		unsubscribe(t, sub)
		unsubscribe(t, sub) // must not close the channel twice
		if got := drain(t, sub); len(got) != 0 {
			t.Errorf("received seqs %v, want none", seqs(got))
		}
		if n := subscribers(t, b); n != 0 {
			t.Errorf("Subscribers() = %d, want 0", n)
		}
	})

	t.Run("Unsubscribe wakes a consumer blocked on C", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		c := channel(t, sub)
		loopDone := make(chan struct{})
		go func() {
			defer close(loopDone)
			for range c { // the loop only waits for the close
			}
		}()
		time.Sleep(window) // give the consumer time to block in the receive
		unsubscribe(t, sub)
		select {
		case <-loopDone:
		case <-time.After(timeout):
			t.Fatalf("the consumer's range loop over C did not end within %v after Unsubscribe", timeout)
		}
	})

	t.Run("other subscriptions are not affected", func(t *testing.T) {
		b := newB[int](t, 0)
		gone := subscribe(t, b, 10, broadcaster.DropOldest)
		stays := subscribe(t, b, 10, broadcaster.DropOldest)
		unsubscribe(t, gone)
		publish(t, b, 1)
		if got := seqs(recvN(t, stays, 1)); !slices.Equal(got, []uint64{1}) {
			t.Errorf("remaining subscriber received seqs %v, want [1]", got)
		}
		if n := subscribers(t, b); n != 1 {
			t.Errorf("Subscribers() = %d, want 1", n)
		}
	})
}

func TestEndClose(t *testing.T) {
	t.Run("every subscription ends with ErrClosed after its buffered events", func(t *testing.T) {
		b := newB[int](t, 0)
		var subs []*broadcaster.Subscription[int]
		for _, p := range policies {
			subs = append(subs, subscribe(t, b, 10, p))
		}
		publishN(t, b, 1, 2)
		closeB(t, b)
		if n := subscribers(t, b); n != 0 {
			t.Errorf("Subscribers() = %d after Close, want 0", n)
		}
		for i, sub := range subs {
			if got := seqs(drain(t, sub)); !slices.Equal(got, []uint64{1, 2}) {
				t.Errorf("%v subscriber received seqs %v, want [1 2]", policies[i], got)
			}
			if err := errOf(t, sub); !errors.Is(err, broadcaster.ErrClosed) {
				t.Errorf("%v subscriber: Err() = %v, want ErrClosed", policies[i], err)
			}
		}
	})

	t.Run("Publish after Close returns ErrClosed", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		publish(t, b, 1)
		closeB(t, b)
		var seq uint64
		var err error
		within(t, "Publish after Close", func() { seq, err = b.Publish(2) })
		if seq != 0 || !errors.Is(err, broadcaster.ErrClosed) {
			t.Errorf("Publish(2) after Close = %d, %v, want 0, ErrClosed", seq, err)
		}
		if got := seqs(drain(t, sub)); !slices.Equal(got, []uint64{1}) {
			t.Errorf("received seqs %v, want [1]", got)
		}
	})

	t.Run("Close is idempotent", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		closeB(t, b)
		closeB(t, b) // must not close any channel twice
		drain(t, sub)
		if err := errOf(t, sub); !errors.Is(err, broadcaster.ErrClosed) {
			t.Errorf("Err() = %v, want ErrClosed", err)
		}
	})

	t.Run("Close wakes a consumer blocked on C", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		c := channel(t, sub)
		loopDone := make(chan struct{})
		go func() {
			defer close(loopDone)
			for range c { // the loop only waits for the close
			}
		}()
		time.Sleep(window) // give the consumer time to block in the receive
		closeB(t, b)
		select {
		case <-loopDone:
		case <-time.After(timeout):
			t.Fatalf("the consumer's range loop over C did not end within %v after Close", timeout)
		}
	})

	t.Run("Subscribe after Close returns an ended subscription", func(t *testing.T) {
		b := newB[int](t, 0)
		closeB(t, b)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		if got := drain(t, sub); len(got) != 0 {
			t.Errorf("received seqs %v, want none", seqs(got))
		}
		if err := errOf(t, sub); !errors.Is(err, broadcaster.ErrClosed) {
			t.Errorf("Err() = %v, want ErrClosed", err)
		}
		if n := subscribers(t, b); n != 0 {
			t.Errorf("Subscribers() = %d, want 0", n)
		}
	})

	t.Run("Unsubscribe after Close is fine and Err stays ErrClosed", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		closeB(t, b)
		unsubscribe(t, sub)
		drain(t, sub)
		if err := errOf(t, sub); !errors.Is(err, broadcaster.ErrClosed) {
			t.Errorf("Err() = %v, want ErrClosed (the first reason wins)", err)
		}
	})
}

func TestEndFirstReasonWins(t *testing.T) {
	// disconnected returns a subscription that a full buffer has just ended.
	disconnected := func(t *testing.T) (*broadcaster.Broadcaster[int], *broadcaster.Subscription[int]) {
		t.Helper()
		b := newB[int](t, 0)
		sub := subscribe(t, b, 1, broadcaster.Disconnect)
		publishN(t, b, 1, 2)
		return b, sub
	}

	t.Run("disconnected, then Close", func(t *testing.T) {
		b, sub := disconnected(t)
		closeB(t, b)
		drain(t, sub)
		if err := errOf(t, sub); !errors.Is(err, broadcaster.ErrSlowConsumer) {
			t.Errorf("Err() = %v, want ErrSlowConsumer", err)
		}
	})

	t.Run("disconnected, then Unsubscribe", func(t *testing.T) {
		_, sub := disconnected(t)
		unsubscribe(t, sub)
		drain(t, sub)
		if err := errOf(t, sub); !errors.Is(err, broadcaster.ErrSlowConsumer) {
			t.Errorf("Err() = %v, want ErrSlowConsumer", err)
		}
	})

	t.Run("Unsubscribe, then Close", func(t *testing.T) {
		b := newB[int](t, 0)
		sub := subscribe(t, b, 10, broadcaster.DropOldest)
		unsubscribe(t, sub)
		closeB(t, b)
		drain(t, sub)
		if err := errOf(t, sub); err != nil {
			t.Errorf("Err() = %v, want nil", err)
		}
	})
}
