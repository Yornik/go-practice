package broadcaster_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	broadcaster "github.com/yornik/go-practice/exercises/46-broadcaster"
)

// ---------------------------------------------------------------------------
// Step 4: history and SubscribeFrom (go test -run TestResume)
// ---------------------------------------------------------------------------

func TestResumeReplay(t *testing.T) {
	t.Run("replays the kept events after `after`, then live events", func(t *testing.T) {
		b := newB[string](t, 5)
		for _, v := range []string{"a", "b", "c"} {
			publish(t, b, v)
		}
		sub := mustSubscribeFrom(t, b, 1, 10, broadcaster.Disconnect)
		got := recvN(t, sub, 2)
		want := []broadcaster.Event[string]{{Seq: 2, Value: "b"}, {Seq: 3, Value: "c"}}
		if !slices.Equal(got, want) {
			t.Errorf("SubscribeFrom(1) replayed %+v, want %+v", got, want)
		}
		expectNothing(t, sub)
		publish(t, b, "d")
		if got := recvN(t, sub, 1); got[0] != (broadcaster.Event[string]{Seq: 4, Value: "d"}) {
			t.Errorf("then received %+v, want {Seq:4 Value:d}", got[0])
		}
		if n := subscribers(t, b); n != 1 {
			t.Errorf("Subscribers() = %d, want 1", n)
		}
	})

	t.Run("after 0 replays from the first event", func(t *testing.T) {
		b := newB[int](t, 5)
		publishN(t, b, 1, 3)
		sub := mustSubscribeFrom(t, b, 0, 10, broadcaster.Disconnect)
		if got := seqs(recvN(t, sub, 3)); !slices.Equal(got, []uint64{1, 2, 3}) {
			t.Errorf("SubscribeFrom(0) replayed seqs %v, want [1 2 3]", got)
		}
		expectNothing(t, sub)
	})

	t.Run("after = the last seq replays nothing", func(t *testing.T) {
		b := newB[int](t, 5)
		publishN(t, b, 1, 3)
		sub := mustSubscribeFrom(t, b, 3, 10, broadcaster.Disconnect)
		expectNothing(t, sub)
		publish(t, b, 4)
		if got := seqs(recvN(t, sub, 1)); !slices.Equal(got, []uint64{4}) {
			t.Errorf("received seqs %v, want [4]", got)
		}
	})

	t.Run("after 0 before the first Publish", func(t *testing.T) {
		b := newB[int](t, 5)
		sub := mustSubscribeFrom(t, b, 0, 10, broadcaster.Disconnect)
		expectNothing(t, sub)
		publish(t, b, 1)
		if got := seqs(recvN(t, sub, 1)); !slices.Equal(got, []uint64{1}) {
			t.Errorf("received seqs %v, want [1]", got)
		}
	})

	t.Run("history 0 still resumes from the last seq", func(t *testing.T) {
		b := newB[int](t, 0)
		publishN(t, b, 1, 2)
		sub := mustSubscribeFrom(t, b, 2, 10, broadcaster.Disconnect)
		publish(t, b, 3)
		if got := seqs(recvN(t, sub, 1)); !slices.Equal(got, []uint64{3}) {
			t.Errorf("received seqs %v, want [3]", got)
		}
	})
}

func TestResumeGone(t *testing.T) {
	tests := []struct {
		name      string
		history   int
		published int
		after     uint64
		gone      bool
		want      []uint64 // replayed seqs if not gone
	}{
		// history 3, events 1 to 5 published: the broadcaster keeps 3, 4, 5.
		{"nothing to replay", 3, 5, 5, false, []uint64{}},
		{"replay 1 event", 3, 5, 4, false, []uint64{5}},
		{"replay every kept event", 3, 5, 2, false, []uint64{3, 4, 5}},
		{"event 2 is no longer kept", 3, 5, 1, true, nil},
		{"events 1 and 2 are no longer kept", 3, 5, 0, true, nil},
		{"seq from the future", 3, 5, 6, true, nil},
		{"seq from before a restart", 3, 5, 1000, true, nil},
		// Nothing was dropped from the history yet.
		{"history not full, from the start", 3, 2, 0, false, []uint64{1, 2}},
		{"nothing published, from the start", 3, 0, 0, false, []uint64{}},
		{"nothing published, seq from the future", 3, 0, 1, true, nil},
		// history 0 keeps nothing.
		{"history 0, from the last seq", 0, 2, 2, false, []uint64{}},
		{"history 0, one event back", 0, 2, 1, true, nil},
		{"history 0, seq from the future", 0, 2, 3, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newB[int](t, tt.history)
			publishN(t, b, 1, tt.published)
			call := fmt.Sprintf("New(%d), %d events published, SubscribeFrom(%d)", tt.history, tt.published, tt.after)
			sub, err := subscribeFrom(t, b, tt.after, 10, broadcaster.Disconnect)
			if tt.gone {
				if !errors.Is(err, broadcaster.ErrGone) {
					t.Errorf("%s: error = %v, want an error that wraps ErrGone", call, err)
				}
				if sub != nil {
					t.Errorf("%s: subscription = %v, want nil with the error", call, sub)
				}
				if n := subscribers(t, b); n != 0 {
					t.Errorf("%s: Subscribers() = %d, want 0", call, n)
				}
				return
			}
			if err != nil || sub == nil {
				t.Fatalf("%s = %v, %v, want a subscription and no error", call, sub, err)
			}
			if got := seqs(recvN(t, sub, len(tt.want))); !slices.Equal(got, tt.want) {
				t.Errorf("%s: replayed seqs %v, want %v", call, got, tt.want)
			}
			expectNothing(t, sub)
		})
	}
}

func TestResumeHistoryWrapsAround(t *testing.T) {
	// The history is a ring buffer that has been overwritten many times. Each
	// value is 10 times its seq, so an event read from the wrong slot shows.
	const history, published = 7, 1000
	b := newB[int](t, history)
	for i := 1; i <= published; i++ {
		publish(t, b, i*10)
	}
	// Kept: 994 to 1000. Resuming works for after = 993 to 1000.
	for after := uint64(990); after <= published+2; after++ {
		sub, err := subscribeFrom(t, b, after, 10, broadcaster.DropNewest)
		if after < published-history || after > published {
			if !errors.Is(err, broadcaster.ErrGone) {
				t.Errorf("SubscribeFrom(%d) error = %v, want ErrGone (kept: 994 to 1000)", after, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("SubscribeFrom(%d) error = %v, want nil (kept: 994 to 1000)", after, err)
			continue
		}
		got := recvN(t, sub, int(published-after))
		for i, ev := range got {
			want := broadcaster.Event[int]{Seq: after + 1 + uint64(i), Value: int(after+1+uint64(i)) * 10}
			if ev != want {
				t.Errorf("SubscribeFrom(%d): replayed event %d = %+v, want %+v", after, i, ev, want)
			}
		}
		unsubscribe(t, sub)
	}
}

func TestResumeReplayUsesBufferAndPolicy(t *testing.T) {
	// 10 events are replayed into a buffer of 3, as if they were published
	// right now while the consumer is not reading.
	setup := func(t *testing.T, p broadcaster.Policy) (*broadcaster.Broadcaster[int], *broadcaster.Subscription[int]) {
		t.Helper()
		b := newB[int](t, 10)
		publishN(t, b, 1, 10)
		return b, mustSubscribeFrom(t, b, 0, 3, p)
	}

	t.Run("DropOldest keeps the newest", func(t *testing.T) {
		b, sub := setup(t, broadcaster.DropOldest)
		if n := dropped(t, sub); n != 7 {
			t.Errorf("Dropped() = %d, want 7", n)
		}
		if got := seqs(recvN(t, sub, 3)); !slices.Equal(got, []uint64{8, 9, 10}) {
			t.Errorf("received seqs %v, want [8 9 10]", got)
		}
		publish(t, b, 11)
		if got := seqs(recvN(t, sub, 1)); !slices.Equal(got, []uint64{11}) {
			t.Errorf("then received seqs %v, want [11]", got)
		}
	})

	t.Run("DropNewest keeps the oldest", func(t *testing.T) {
		_, sub := setup(t, broadcaster.DropNewest)
		if n := dropped(t, sub); n != 7 {
			t.Errorf("Dropped() = %d, want 7", n)
		}
		if got := seqs(recvN(t, sub, 3)); !slices.Equal(got, []uint64{1, 2, 3}) {
			t.Errorf("received seqs %v, want [1 2 3]", got)
		}
	})

	t.Run("Disconnect ends the subscription during the replay", func(t *testing.T) {
		b, sub := setup(t, broadcaster.Disconnect)
		if n := subscribers(t, b); n != 0 {
			t.Errorf("Subscribers() = %d, want 0", n)
		}
		if got := seqs(drain(t, sub)); !slices.Equal(got, []uint64{1, 2, 3}) {
			t.Errorf("received seqs %v before C was closed, want [1 2 3]", got)
		}
		if err := errOf(t, sub); !errors.Is(err, broadcaster.ErrSlowConsumer) {
			t.Errorf("Err() = %v, want ErrSlowConsumer", err)
		}
		publish(t, b, 11) // the ended subscription is not in the way
	})
}

func TestResumeAfterDisconnect(t *testing.T) {
	// The normal life of a watcher with the Disconnect policy: it falls
	// behind, gets disconnected, and continues from the last seq it received.
	b := newB[int](t, 100)
	sub := subscribe(t, b, 2, broadcaster.Disconnect)
	publishN(t, b, 1, 5)
	got := drain(t, sub)
	if !slices.Equal(seqs(got), []uint64{1, 2}) || !errors.Is(errOf(t, sub), broadcaster.ErrSlowConsumer) {
		t.Fatalf("first subscription: received seqs %v and Err() = %v, want [1 2] and ErrSlowConsumer", seqs(got), errOf(t, sub))
	}
	last := got[len(got)-1].Seq

	sub = mustSubscribeFrom(t, b, last, 10, broadcaster.Disconnect)
	publish(t, b, 6)
	if got := seqs(recvN(t, sub, 4)); !slices.Equal(got, []uint64{3, 4, 5, 6}) {
		t.Errorf("after resuming from %d: received seqs %v, want [3 4 5 6]", last, got)
	}
}

func TestResumeAfterClose(t *testing.T) {
	b := newB[int](t, 10)
	publishN(t, b, 1, 3)
	closeB(t, b)
	for _, after := range []uint64{0, 3, 1000} { // valid or not: closed is checked first
		sub, err := subscribeFrom(t, b, after, 10, broadcaster.DropOldest)
		if sub != nil || !errors.Is(err, broadcaster.ErrClosed) {
			t.Errorf("SubscribeFrom(%d) after Close = %v, %v, want nil, ErrClosed", after, sub, err)
		}
	}
}
