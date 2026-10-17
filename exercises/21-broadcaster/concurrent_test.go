package broadcaster_test

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime"
	"runtime/pprof"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	broadcaster "github.com/yornik/go-practice/exercises/21-broadcaster"
)

// ---------------------------------------------------------------------------
// Step 5: everything at once (go test -race -run TestConcurrent)
// ---------------------------------------------------------------------------

// These tests call the methods from many goroutines at the same time. A bug
// does not show up in every run, so run them with -race, and a few times:
//
//	go test -race -count=5 -run TestConcurrent ./exercises/21-broadcaster/
//
// They call the methods directly, without a timeout wrapper. A send on a
// closed channel or a second close of a channel panics in some goroutine
// and kills the whole test binary; the panic message and its stack show
// where it happened.

// waitGroup waits for wg. It fails the test if that takes longer than the
// timeout.
func waitGroup(t *testing.T, wg *sync.WaitGroup, what string) {
	t.Helper()
	waitGroupFor(t, wg, what, timeout)
}

// waitGroupFor waits for wg. It fails the test if that takes longer than d.
func waitGroupFor(t *testing.T, wg *sync.WaitGroup, what string, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not finish within %v", what, d)
	}
}

// diff describes the first difference between got and want, without printing
// thousands of numbers.
func diff(got, want []uint64) string {
	for i := range min(len(got), len(want)) {
		if got[i] != want[i] {
			return fmt.Sprintf("element %d is %d, want %d", i, got[i], want[i])
		}
	}
	return fmt.Sprintf("got %d elements, want %d", len(got), len(want))
}

// increasing reports the first place where seqs is not strictly increasing,
// or "" if it is.
func increasing(seqs []uint64) string {
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			return fmt.Sprintf("seq %d comes after seq %d (position %d)", seqs[i], seqs[i-1], i)
		}
	}
	return ""
}

// contiguous reports the first gap or disorder in seqs, or "" if every seq is
// the previous one + 1.
func contiguous(seqs []uint64) string {
	for i := 1; i < len(seqs); i++ {
		if seqs[i] != seqs[i-1]+1 {
			return fmt.Sprintf("seq %d comes after seq %d (position %d)", seqs[i], seqs[i-1], i)
		}
	}
	return ""
}

func TestConcurrentPublish(t *testing.T) {
	const publishers, perPublisher = 8, 500
	const total = publishers * perPublisher
	b := newB[int](t, 0)
	sub := subscribe(t, b, total, broadcaster.Disconnect) // big enough to never overflow

	// seqOf[p][i] is the seq that Publish returned for value p*perPublisher+i.
	seqOf := make([][]uint64, publishers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for p := range publishers {
		seqOf[p] = make([]uint64, perPublisher)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := range perPublisher {
				seq, err := b.Publish(p*perPublisher + i)
				if err != nil {
					t.Errorf("Publish = %d, %v, want no error", seq, err)
					return
				}
				seqOf[p][i] = seq
			}
		}()
	}
	close(start)
	waitGroup(t, &wg, "8 goroutines that call Publish 500 times each")

	all := slices.Concat(seqOf...)
	slices.Sort(all)
	if !slices.Equal(all, seqRange(1, total)) {
		t.Errorf("the seqs returned by 4000 concurrent calls of Publish are not 1 to 4000, each once: %s", diff(all, seqRange(1, total)))
	}

	closeB(t, b)
	got := drain(t, sub)
	if s := seqs(got); !slices.Equal(s, seqRange(1, total)) {
		t.Fatalf("the subscriber received seqs that are not 1 to 4000 in order: %s", diff(s, seqRange(1, total)))
	}
	for _, ev := range got {
		p, i := ev.Value/perPublisher, ev.Value%perPublisher
		if ev.Seq != seqOf[p][i] {
			t.Fatalf("event with Value %d has Seq %d, but Publish(%d) returned %d", ev.Value, ev.Seq, ev.Value, seqOf[p][i])
		}
	}
}

func TestConcurrentJoinWhilePublishing(t *testing.T) {
	// One goroutine publishes 3000 events. While it runs, 40 watchers join:
	// some with Subscribe, most with SubscribeFrom a few events back. Each
	// watcher's buffer can hold every event and the history keeps every
	// event, so nothing is ever dropped or gone. The hand-off between replay
	// and live events must not lose or repeat an event, whenever the
	// publisher runs.
	const events, watchers = 3000, 40
	b := newB[int](t, events)

	var last atomic.Uint64    // the seq the last Publish returned
	var started atomic.Uint64 // the seq of the last Publish that started
	start := make(chan struct{})
	pubDone := make(chan struct{})
	go func() {
		defer close(pubDone)
		<-start
		for i := 1; i <= events; i++ {
			started.Store(uint64(i)) // one publisher, so event i gets seq i
			seq, err := b.Publish(i)
			if err != nil {
				t.Errorf("Publish = %d, %v, want no error", seq, err)
				return
			}
			last.Store(seq)
			if i%16 == 0 {
				runtime.Gosched() // let watchers subscribe between events
			}
		}
	}()

	type watch struct {
		resumed     bool
		after       uint64 // resumed: the after passed to SubscribeFrom; live: last seq published before Subscribe
		afterReturn uint64 // live: the events after this one started to publish after Subscribe returned
		subErr      error  // error from SubscribeFrom
		got         []uint64
		err         error // Err() once C is closed
	}
	results := make([]watch, watchers)
	var wg, joined sync.WaitGroup // joined: Subscribe or SubscribeFrom has returned
	for w := range watchers {
		wg.Add(1)
		joined.Add(1)
		go func() {
			defer wg.Done()
			<-start
			// Spread the watchers over the run of the publisher.
		wait:
			for last.Load() < uint64(w*events/watchers) {
				select {
				case <-pubDone:
					break wait
				default:
					runtime.Gosched()
				}
			}
			res := &results[w]
			var sub *broadcaster.Subscription[int]
			if w%4 == 0 {
				res.after = last.Load()
				sub = b.Subscribe(events, broadcaster.Disconnect)
				res.afterReturn = started.Load()
				joined.Done()
			} else {
				res.resumed = true
				l := last.Load()
				res.after = l - min(l, uint64(w%10))
				sub, res.subErr = b.SubscribeFrom(res.after, events, broadcaster.Disconnect)
				joined.Done()
				if res.subErr != nil {
					return
				}
			}
			for ev := range sub.C() {
				res.got = append(res.got, ev.Seq)
			}
			res.err = sub.Err()
		}()
	}
	close(start)
	select {
	case <-pubDone:
	case <-time.After(timeout):
		t.Fatalf("publishing %d events did not finish within %v", events, timeout)
	}
	waitGroup(t, &joined, "the watchers' calls of Subscribe and SubscribeFrom")
	closeB(t, b)
	waitGroup(t, &wg, "the watchers (Close must close every channel)")

	for w, res := range results {
		if res.subErr != nil {
			t.Errorf("watcher %d: SubscribeFrom(%d) error = %v, want nil (the history keeps every event)", w, res.after, res.subErr)
			continue
		}
		if !errors.Is(res.err, broadcaster.ErrClosed) {
			t.Errorf("watcher %d: Err() = %v, want ErrClosed (its buffer never fills, so only Close ends it)", w, res.err)
			continue
		}
		if res.resumed {
			if want := seqRange(res.after+1, events); !slices.Equal(res.got, want) {
				t.Errorf("watcher %d: SubscribeFrom(%d) while Publish ran: received seqs are not %d to %d: %s",
					w, res.after, res.after+1, events, diff(res.got, want))
			}
			continue
		}
		// Live: no event published before Subscribe was called, every event
		// published after it returned, and nothing missing in between. Events
		// published while Subscribe ran may or may not be included.
		if msg := contiguous(res.got); msg != "" {
			t.Errorf("watcher %d: Subscribe while Publish ran: %s", w, msg)
			continue
		}
		if len(res.got) > 0 && res.got[0] <= res.after {
			t.Errorf("watcher %d: received seq %d, which was published before Subscribe was called", w, res.got[0])
		}
		if res.afterReturn < events && (len(res.got) == 0 || res.got[0] > res.afterReturn+1 || res.got[len(res.got)-1] != events) {
			t.Errorf("watcher %d: missed events published after Subscribe returned (seq %d and later); received %d events", w, res.afterReturn+1, len(res.got))
		}
	}
}

func TestConcurrentResumeNoGapNoDuplicate(t *testing.T) {
	// One goroutine publishes 3000 events. Meanwhile 8 watchers follow the
	// stream the way a real watch client does: read a few events, drop the
	// connection (Unsubscribe) or get disconnected (buffer of 16), and resume
	// with SubscribeFrom(last seq received). That is thousands of hand-offs
	// from replay to live events while Publish runs. The history keeps every
	// event, so resuming never fails. Across all its reconnects, each watcher
	// must receive every event exactly once and in order.
	const events, watchers, buf = 3000, 8, 16
	b := newB[int](t, events)

	start := make(chan struct{})
	pubDone := make(chan struct{})
	go func() {
		defer close(pubDone)
		<-start
		for i := 1; i <= events; i++ {
			if _, err := b.Publish(i); err != nil {
				t.Errorf("Publish = %v, want no error", err)
				return
			}
			if i%16 == 0 {
				runtime.Gosched()
			}
		}
	}()

	var wg sync.WaitGroup
	for w := range watchers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(w), 7))
			deadline := time.After(timeout)
			<-start
			var lastSeen uint64 // the last seq this watcher received
			for resumes := 0; lastSeen < events; resumes++ {
				after := lastSeen
				sub, err := b.SubscribeFrom(after, buf, broadcaster.Disconnect)
				if err != nil {
					t.Errorf("watcher %d, resume %d: SubscribeFrom(%d) error = %v, want nil (the history keeps every event)", w, resumes, after, err)
					return
				}
				// Read a few events, then Unsubscribe and read what is
				// still buffered, until C is closed.
				readUpTo := 1 + rng.IntN(20)
				for n := 0; ; n++ {
					if n == readUpTo {
						sub.Unsubscribe()
					}
					var ev broadcaster.Event[int]
					var ok bool
					select {
					case ev, ok = <-sub.C():
					case <-deadline:
						t.Errorf("watcher %d: stuck at seq %d after %d resumes: no event within %v", w, lastSeen, resumes, timeout)
						sub.Unsubscribe()
						return
					}
					if !ok {
						break
					}
					if ev.Seq != lastSeen+1 {
						t.Errorf("watcher %d, resume %d: SubscribeFrom(%d) while Publish ran: received seq %d after seq %d, want %d",
							w, resumes, after, ev.Seq, lastSeen, lastSeen+1)
						sub.Unsubscribe()
						return
					}
					lastSeen = ev.Seq
					if lastSeen == events {
						sub.Unsubscribe() // done: nothing more will come
					}
				}
				if err := sub.Err(); err != nil && !errors.Is(err, broadcaster.ErrSlowConsumer) {
					t.Errorf("watcher %d: Err() = %v, want nil or ErrSlowConsumer", w, err)
					return
				}
			}
		}()
	}
	close(start)
	// Each watcher gives up by itself after the timeout, with a message that
	// says where it is stuck. Wait longer than that.
	waitGroupFor(t, &wg, "8 watchers that each follow 3000 events", 2*timeout)
	<-pubDone
	closeB(t, b)
}

func TestConcurrentSlowConsumer(t *testing.T) {
	// A consumer that reads, but more slowly than the producer publishes.
	// Whatever the timing, the policy's promise must hold. With buf 1 the
	// consumer and the policy compete for a single slot all the time.
	const events = 5000
	for _, buf := range []int{1, 8} {
		for _, p := range policies {
			t.Run(fmt.Sprintf("%v buf %d", p, buf), func(t *testing.T) {
				b := newB[int](t, 0)
				sub := subscribe(t, b, buf, p)
				c := channel(t, sub)
				gotCh := make(chan []uint64, 1)
				go func() {
					var got []uint64
					for ev := range c {
						got = append(got, ev.Seq)
						if len(got)%4 == 0 {
							runtime.Gosched() // a bit slow
						}
					}
					gotCh <- got
				}()
				// A metrics scraper reads Err and Dropped while the policy acts.
				stopScraper := make(chan struct{})
				scraperDone := make(chan struct{})
				go func() {
					defer close(scraperDone)
					for {
						select {
						case <-stopScraper:
							return
						default:
						}
						_, _ = sub.Err(), sub.Dropped()
						runtime.Gosched()
					}
				}()
				within(t, "publishing 5000 events", func() {
					for i := 1; i <= events; i++ {
						if _, err := b.Publish(i); err != nil {
							panic(err)
						}
					}
				})
				closeB(t, b)
				close(stopScraper)
				<-scraperDone
				var got []uint64
				select {
				case got = <-gotCh:
				case <-time.After(timeout):
					t.Fatalf("the consumer's range loop over C did not end within %v after Close", timeout)
				}
				if msg := increasing(got); msg != "" {
					t.Fatalf("received seqs are not strictly increasing: %s", msg)
				}
				n := dropped(t, sub)
				err := errOf(t, sub)

				switch p {
				case broadcaster.DropOldest, broadcaster.DropNewest:
					// Every event was either received or counted as dropped.
					if uint64(len(got))+n != events {
						t.Errorf("received %d events and Dropped() = %d, want them to add up to %d", len(got), n, events)
					}
					if !errors.Is(err, broadcaster.ErrClosed) {
						t.Errorf("Err() = %v, want ErrClosed", err)
					}
				}
				switch p {
				case broadcaster.DropOldest:
					// The newest event is never the one that gets dropped.
					if len(got) == 0 || got[len(got)-1] != events {
						t.Errorf("the last received seq is not %d: DropOldest must keep the newest events", events)
					}
				case broadcaster.DropNewest:
					// The first buf events found an empty buffer.
					if len(got) < buf || !slices.Equal(got[:buf], seqRange(1, uint64(buf))) {
						t.Errorf("the first received seqs are not 1 to %d: DropNewest must keep the oldest events", buf)
					}
				case broadcaster.Disconnect:
					if n != 0 {
						t.Errorf("Dropped() = %d, want 0", n)
					}
					if len(got) > 0 && got[0] != 1 {
						t.Errorf("first received seq = %d, want 1", got[0])
					}
					if msg := contiguous(got); msg != "" {
						t.Errorf("Disconnect must deliver events without gaps, but %s", msg)
					}
					switch {
					case errors.Is(err, broadcaster.ErrSlowConsumer) && len(got) < events:
					case errors.Is(err, broadcaster.ErrClosed) && len(got) == events:
					default:
						t.Errorf("received %d of %d events and Err() = %v: want ErrSlowConsumer with fewer than %d, or ErrClosed with all of them",
							len(got), events, err, events)
					}
				}
			})
		}
	}
}

func TestConcurrentCloseWhileUnsubscribing(t *testing.T) {
	// Close and Unsubscribe can both end the same subscription. Here they run
	// at the same moment on 200 subscriptions: one goroutine per subscription
	// calls Unsubscribe and Err, another goroutine calls Close. Every channel
	// must be closed exactly once, after its buffered event, and Err must be
	// the cause that came first: nil (Unsubscribe) or ErrClosed (Close). The
	// race is short, so it runs 5 times with a new broadcaster.
	const rounds, n = 5, 200
	for round := range rounds {
		b := newB[int](t, 0)
		subs := make([]*broadcaster.Subscription[int], n)
		for i := range subs {
			subs[i] = subscribe(t, b, 1, broadcaster.DropOldest)
		}
		publish(t, b, 1) // one buffered event in every subscription

		start := make(chan struct{})
		var wg sync.WaitGroup
		unsub := func(sub *broadcaster.Subscription[int]) {
			defer wg.Done()
			<-start
			sub.Unsubscribe()
			_ = sub.Err()
		}
		for _, sub := range subs[:n/2] {
			wg.Add(1)
			go unsub(sub)
		}
		wg.Add(1)
		go func() { // the closer, started in the middle so that it runs between the others
			defer wg.Done()
			<-start
			b.Close()
		}()
		for _, sub := range subs[n/2:] {
			wg.Add(1)
			go unsub(sub)
		}
		close(start)
		waitGroup(t, &wg, "200 calls of Unsubscribe at the same time as Close")

		for i, sub := range subs {
			if got := seqs(drain(t, sub)); !slices.Equal(got, []uint64{1}) {
				t.Fatalf("round %d, subscription %d: received seqs %v, want [1]", round, i, got)
			}
			if err := errOf(t, sub); err != nil && !errors.Is(err, broadcaster.ErrClosed) {
				t.Fatalf("round %d, subscription %d: Err() = %v, want nil or ErrClosed", round, i, err)
			}
		}
		if got := subscribers(t, b); got != 0 {
			t.Fatalf("round %d: Subscribers() = %d after Close, want 0", round, got)
		}
	}
}

func TestConcurrentHammer(t *testing.T) {
	// Publish, Subscribe, SubscribeFrom, Unsubscribe, Close, Subscribers,
	// Err and Dropped, all at the same time. Small buffers make the policies
	// kick in all the time. The test checks that nothing panics, that every
	// channel gets closed, and what every watcher received.
	const publishes = 10000
	b := newB[int](t, 32)
	var last atomic.Uint64
	var wg sync.WaitGroup

	for range 3 { // publishers, until Close
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				seq, err := b.Publish(1)
				if err != nil {
					if !errors.Is(err, broadcaster.ErrClosed) {
						t.Errorf("Publish = %d, %v, want no error or ErrClosed", seq, err)
					}
					return
				}
				last.Store(seq)
				if seq%8 == 0 {
					runtime.Gosched()
				}
			}
		}()
	}

	wg.Add(1)
	go func() { // the closer
		defer wg.Done()
		for last.Load() < publishes {
			runtime.Gosched()
		}
		b.Close()
		b.Close()
	}()

	wg.Add(1)
	go func() { // a metrics scraper
		defer wg.Done()
		for {
			if b.Subscribers() < 0 {
				t.Errorf("Subscribers() < 0")
			}
			sub := b.Subscribe(1, broadcaster.DropNewest)
			_ = sub.Dropped()
			sub.Unsubscribe()
			if errors.Is(sub.Err(), broadcaster.ErrClosed) {
				return
			}
			runtime.Gosched()
		}
	}()

	for w := range 12 { // watchers
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(w), 21))
			for {
				p := policies[rng.IntN(len(policies))]
				buf := rng.IntN(4) // 0 to 3
				resumed := rng.IntN(2) == 0
				var after uint64
				var sub *broadcaster.Subscription[int]
				if resumed {
					l := last.Load()
					after = l - min(l, uint64(rng.IntN(48))) // sometimes too old
					s, err := b.SubscribeFrom(after, buf, p)
					switch {
					case errors.Is(err, broadcaster.ErrClosed):
						return
					case errors.Is(err, broadcaster.ErrGone):
						continue
					case err != nil:
						t.Errorf("SubscribeFrom(%d) error = %v, want nil, ErrGone or ErrClosed", after, err)
						return
					}
					sub = s
				} else {
					sub = b.Subscribe(buf, p)
				}

				unsubscribed := false
				limit := -1
				switch rng.IntN(3) {
				case 0: // Unsubscribe from another goroutine while this one reads
					unsubscribed = true
					wg.Add(1)
					go func() {
						defer wg.Done()
						runtime.Gosched()
						sub.Unsubscribe()
					}()
				case 1: // Unsubscribe from the consumer itself after a few events
					limit = rng.IntN(20)
				}
				var got []uint64
				if limit == 0 {
					sub.Unsubscribe()
					unsubscribed = true
				}
				for ev := range sub.C() {
					got = append(got, ev.Seq)
					if len(got) == limit {
						sub.Unsubscribe()
						unsubscribed = true
					}
				}
				err := sub.Err()
				desc := fmt.Sprintf("watcher %d (%v, buf %d)", w, p, buf)
				if resumed {
					desc += fmt.Sprintf(", SubscribeFrom(%d)", after)
				}

				if msg := increasing(got); msg != "" {
					t.Errorf("%s: %s", desc, msg)
				}
				if resumed && len(got) > 0 && got[0] <= after {
					t.Errorf("%s: received seq %d, want only seqs > %d", desc, got[0], after)
				}
				if p == broadcaster.Disconnect {
					if msg := contiguous(got); msg != "" {
						t.Errorf("%s: Disconnect must deliver events without gaps, but %s", desc, msg)
					}
					if resumed && len(got) > 0 && got[0] != after+1 {
						t.Errorf("%s: first received seq = %d, want %d", desc, got[0], after+1)
					}
					if n := sub.Dropped(); n != 0 {
						t.Errorf("%s: Dropped() = %d, want 0", desc, n)
					}
				}
				switch {
				case err == nil && !unsubscribed:
					t.Errorf("%s: C was closed and Err() = nil, but the watcher never called Unsubscribe", desc)
				case errors.Is(err, broadcaster.ErrSlowConsumer) && p != broadcaster.Disconnect:
					t.Errorf("%s: Err() = ErrSlowConsumer, but the policy is %v", desc, p)
				case err != nil && !errors.Is(err, broadcaster.ErrClosed) && !errors.Is(err, broadcaster.ErrSlowConsumer):
					t.Errorf("%s: Err() = %v, want nil, ErrSlowConsumer or ErrClosed", desc, err)
				}
				if errors.Is(err, broadcaster.ErrClosed) {
					return
				}
			}
		}()
	}

	waitGroup(t, &wg, "the hammer (publishers, watchers and Close)")
	if n := b.Subscribers(); n != 0 {
		t.Errorf("Subscribers() = %d after Close, want 0", n)
	}
}

func TestConcurrentNoGoroutineLeak(t *testing.T) {
	// If your design starts goroutines, they must all be gone once the
	// broadcaster is closed and every channel has been drained.
	before := runtime.NumGoroutine()
	b := newB[int](t, 16)
	var subs []*broadcaster.Subscription[int]
	var lastSeq uint64
	for i := range 30 {
		p := policies[i%len(policies)]
		if i%2 == 0 {
			subs = append(subs, subscribe(t, b, 4, p))
		} else {
			subs = append(subs, mustSubscribeFrom(t, b, lastSeq-min(lastSeq, 3), 4, p))
		}
		lastSeq = publish(t, b, i)
	}
	for _, sub := range subs[:10] {
		unsubscribe(t, sub)
	}
	closeB(t, b)
	for _, sub := range subs {
		drain(t, sub)
	}

	deadline := time.Now().Add(timeout)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			var stacks bytes.Buffer
			_ = pprof.Lookup("goroutine").WriteTo(&stacks, 1)
			t.Fatalf("%d goroutines are running after Close and draining every channel, but only %d ran before New: "+
				"a goroutine of the broadcaster is stuck. All goroutines:\n%s", runtime.NumGoroutine(), before, stacks.String())
		}
		time.Sleep(time.Millisecond)
	}
}
