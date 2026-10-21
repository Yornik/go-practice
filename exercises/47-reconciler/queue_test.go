package reconciler_test

import (
	"fmt"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	reconciler "github.com/yornik/go-practice/exercises/47-reconciler"
)

// ----- Add, Get, Done -----

func TestQueueFIFO(t *testing.T) {
	hangGuard(t)
	q := reconciler.NewQueue[string](time.Millisecond, time.Second)
	for _, k := range []string{"web", "api", "db"} {
		q.Add(k)
	}
	expectLen(t, q, 3, "after adding 3 keys")
	for i, want := range []string{"web", "api", "db"} {
		got, ok := getNow(t, q)
		if got != want || !ok {
			t.Fatalf("Get() #%d = %q, %v, want %q, true: keys come out in the order they were added", i+1, got, ok, want)
		}
		expectLen(t, q, 2-i, fmt.Sprintf("after Get #%d", i+1))
	}
}

func TestQueueDedup(t *testing.T) {
	hangGuard(t)
	q := reconciler.NewQueue[string](time.Millisecond, time.Second)
	// Three watch events for "web" arrive before a worker gets to it: one
	// reconcile is enough, because it reads the latest state anyway.
	q.Add("web")
	q.Add("api")
	q.Add("web")
	q.Add("web")
	expectLen(t, q, 2, `after Add("web"), Add("api"), Add("web"), Add("web")`)
	// Adding a waiting key again does not move it to the back.
	for _, want := range []string{"web", "api"} {
		if got, ok := getNow(t, q); got != want || !ok {
			t.Fatalf("Get() = %q, %v, want %q, true", got, ok, want)
		}
	}
	expectLen(t, q, 0, "after getting both keys")
}

func TestQueueGetBlocksUntilAdd(t *testing.T) {
	hangGuard(t)
	q := reconciler.NewQueue[string](time.Millisecond, time.Second)
	ch := startGet(q)
	expectBlocked(t, ch, "the queue is empty")
	q.Add("web")
	if got, ok := awaitGet(t, ch, `Add("web") must wake up a blocked Get`); got != "web" || !ok {
		t.Fatalf("Get() = %q, %v, want %q, true", got, ok, "web")
	}
}

func TestQueueAddWhileProcessing(t *testing.T) {
	hangGuard(t)
	t.Run("queued again after Done, once", func(t *testing.T) {
		q := reconciler.NewQueue[string](time.Millisecond, time.Second)
		q.Add("web")
		if got, _ := getNow(t, q); got != "web" {
			t.Fatalf("Get() = %q, want %q", got, "web")
		}
		// Two events arrive while a worker reconciles "web".
		q.Add("web")
		q.Add("web")
		expectLen(t, q, 0, `after Add("web") while "web" is processing (it must wait for Done)`)

		// A second worker must not get "web" while the first one has it.
		ch := startGet(q)
		expectBlocked(t, ch, `"web" is still processing, and a key is never handed to 2 workers at once`)

		q.Done("web")
		if got, ok := awaitGet(t, ch, `Done("web") must queue "web" again and wake up the blocked Get`); got != "web" || !ok {
			t.Fatalf("Get() after Done = %q, %v, want %q, true", got, ok, "web")
		}
		q.Done("web")
		// The 2 Adds during processing give 1 more reconcile, not 2.
		expectLen(t, q, 0, "after the second Done")
	})

	t.Run("not queued again without an Add", func(t *testing.T) {
		q := reconciler.NewQueue[string](time.Millisecond, time.Second)
		q.Add("web")
		getNow(t, q)
		q.Done("web")
		expectLen(t, q, 0, `after Get and Done of "web" with no Add in between`)
	})

	t.Run("other keys are handed out meanwhile", func(t *testing.T) {
		q := reconciler.NewQueue[string](time.Millisecond, time.Second)
		q.Add("web")
		getNow(t, q)
		q.Add("api")
		if got, ok := getNow(t, q); got != "api" || !ok {
			t.Fatalf("Get() = %q, %v while \"web\" is processing, want %q, true", got, ok, "api")
		}
	})
}

// ----- ShutDown -----

func TestQueueShutDown(t *testing.T) {
	hangGuard(t)
	t.Run("wakes every blocked Get", func(t *testing.T) {
		q := reconciler.NewQueue[string](time.Millisecond, time.Second)
		var chans []<-chan getResult
		for range 3 {
			chans = append(chans, startGet(q))
		}
		time.Sleep(window) // let all 3 block
		q.ShutDown()
		for i, ch := range chans {
			got, ok := awaitGet(t, ch, "ShutDown must wake up every blocked Get, not just one")
			if got != "" || ok {
				t.Fatalf("blocked Get #%d returned %q, %v after ShutDown, want \"\", false", i+1, got, ok)
			}
		}
	})

	t.Run("Get drains the waiting keys, then returns false", func(t *testing.T) {
		q := reconciler.NewQueue[string](time.Millisecond, time.Second)
		q.Add("web")
		q.Add("api")
		q.ShutDown()
		for _, want := range []string{"web", "api"} {
			if got, ok := getNow(t, q); got != want || !ok {
				t.Fatalf("Get() after ShutDown = %q, %v, want %q, true (keys that were waiting are still handed out)", got, ok, want)
			}
		}
		if got, ok := getNow(t, q); got != "" || ok {
			t.Fatalf("Get() on a drained queue after ShutDown = %q, %v, want \"\", false", got, ok)
		}
	})

	t.Run("Add after ShutDown is ignored", func(t *testing.T) {
		q, clock := newFakeQueue()
		q.ShutDown()
		q.Add("web")
		q.AddAfter("api", 0)
		q.AddRateLimited("db")
		clock.Advance(time.Minute) // the retry of "db" would be due now
		expectLen(t, q, 0, "after Add, AddAfter and AddRateLimited on a shut-down queue")
		if got, ok := getNow(t, q); got != "" || ok {
			t.Fatalf("Get() = %q, %v, want \"\", false", got, ok)
		}
	})

	t.Run("calling it twice is fine", func(t *testing.T) {
		q := reconciler.NewQueue[string](time.Millisecond, time.Second)
		within(t, "the first ShutDown", q.ShutDown)
		within(t, "the second ShutDown", q.ShutDown)
	})
}

// ----- AddAfter -----

func TestQueueAddAfter(t *testing.T) {
	hangGuard(t)
	t.Run("waits for the delay", func(t *testing.T) {
		q, clock := newFakeQueue()
		q.AddAfter("web", 10*time.Second)
		expectLen(t, q, 0, "right after AddAfter(\"web\", 10s)")
		clock.Advance(10*time.Second - time.Millisecond)
		expectLen(t, q, 0, "9.999s after AddAfter(\"web\", 10s)")
		clock.Advance(time.Millisecond)
		expectLen(t, q, 1, "10s after AddAfter(\"web\", 10s)")
		if got, ok := getNow(t, q); got != "web" || !ok {
			t.Fatalf("Get() = %q, %v, want %q, true", got, ok, "web")
		}
	})

	t.Run("zero or negative delay adds at once", func(t *testing.T) {
		q, _ := newFakeQueue()
		q.AddAfter("web", 0)
		q.AddAfter("api", -time.Second)
		expectLen(t, q, 2, "after AddAfter with delays 0 and -1s, without advancing the clock")
	})

	t.Run("no duplicate when the key is already waiting", func(t *testing.T) {
		q, clock := newFakeQueue()
		q.Add("web")
		q.AddAfter("web", time.Second)
		clock.Advance(time.Second)
		expectLen(t, q, 1, `after Add("web") and AddAfter("web", 1s) fired`)
	})

	t.Run("a delayed key that comes due after ShutDown is ignored", func(t *testing.T) {
		q, clock := newFakeQueue()
		q.AddAfter("web", time.Second)
		q.ShutDown()
		clock.Advance(time.Second)
		expectLen(t, q, 0, "after the delay of an AddAfter passed on a shut-down queue")
		if got, ok := getNow(t, q); got != "" || ok {
			t.Fatalf("Get() = %q, %v, want \"\", false", got, ok)
		}
	})
}

func TestQueueAddAfterRealClock(t *testing.T) {
	hangGuard(t)
	// NewQueue uses real time.
	q := reconciler.NewQueue[string](time.Millisecond, time.Second)
	q.AddAfter("later", time.Hour)
	q.AddAfter("soon", time.Millisecond)
	if got, ok := awaitGet(t, startGet(q), `AddAfter("soon", 1ms) must add "soon" after 1ms`); got != "soon" || !ok {
		t.Fatalf("Get() = %q, %v, want %q, true", got, ok, "soon")
	}
	expectLen(t, q, 0, `after getting "soon" ("later" is due in 1h)`)
}

// ----- AddRateLimited, NumRequeues, Forget -----

func TestQueueAddRateLimitedBackoff(t *testing.T) {
	hangGuard(t)
	q, clock := newFakeQueue() // base 1s, max 1min
	// The reconcile of "web" fails again and again.
	wants := []time.Duration{
		1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		16 * time.Second, 32 * time.Second, time.Minute, time.Minute,
	}
	for i, want := range wants {
		q.AddRateLimited("web")
		if got := q.NumRequeues("web"); got != i+1 {
			t.Fatalf("NumRequeues(\"web\") = %d after %d calls of AddRateLimited, want %d", got, i+1, i+1)
		}
		clock.Advance(want - time.Nanosecond)
		if q.Len() != 0 {
			t.Fatalf("AddRateLimited #%d: \"web\" was added before %v had passed, want a delay of %v", i+1, want-time.Nanosecond, want)
		}
		clock.Advance(time.Nanosecond)
		if q.Len() != 1 {
			t.Fatalf("AddRateLimited #%d: \"web\" was not added after %v, want a delay of %v (baseDelay * 2^n, at most maxDelay)", i+1, want, want)
		}
		getNow(t, q)
		q.Done("web")
	}
}

func TestQueueAddRateLimitedNeverOverflows(t *testing.T) {
	hangGuard(t)
	// client-go's defaults for controllers: 5ms, doubling up to 1000s. A key
	// that fails all day gets there and stays there.
	clock := &fakeClock{}
	q := reconciler.NewQueueWithClock[string](5*time.Millisecond, 1000*time.Second, clock)
	for range 99 {
		q.AddRateLimited("web")
	}
	clock.Advance(1000 * time.Second) // every one of the 99 retries is due now
	getNow(t, q)
	q.Done("web")

	// 5ms * 2^99 does not fit in any integer type.
	q.AddRateLimited("web")
	clock.Advance(1000*time.Second - time.Nanosecond)
	if q.Len() != 0 {
		t.Fatalf("AddRateLimited #100: \"web\" was added before 1000s had passed, want a delay of maxDelay (did the backoff overflow?)")
	}
	clock.Advance(time.Nanosecond)
	if q.Len() != 1 {
		t.Fatalf("AddRateLimited #100: \"web\" was not added after 1000s, want a delay of maxDelay")
	}
}

func TestQueueNumRequeuesAndForget(t *testing.T) {
	hangGuard(t)
	q, clock := newFakeQueue() // base 1s, max 1min
	if got := q.NumRequeues("web"); got != 0 {
		t.Fatalf("NumRequeues of an unknown key = %d, want 0", got)
	}
	for range 3 {
		q.AddRateLimited("web")
	}
	q.AddRateLimited("api")
	if web, api := q.NumRequeues("web"), q.NumRequeues("api"); web != 3 || api != 1 {
		t.Fatalf("NumRequeues: web = %d, api = %d, want 3 and 1 (each key has its own count)", web, api)
	}

	q.Forget("web") // the reconcile of "web" succeeded
	if web, api := q.NumRequeues("web"), q.NumRequeues("api"); web != 0 || api != 1 {
		t.Fatalf(`after Forget("web"): NumRequeues web = %d, api = %d, want 0 and 1`, web, api)
	}

	// Forget does not cancel the retries that are already scheduled.
	clock.Advance(time.Minute)
	var got []string
	for range 2 {
		k, _ := getNow(t, q)
		got = append(got, k)
		q.Done(k)
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"api", "web"}) {
		t.Fatalf("keys after the retries came due = %v, want [api web]: Forget must not cancel a pending retry", got)
	}

	// After Forget the backoff starts again at baseDelay.
	q.AddRateLimited("web")
	clock.Advance(time.Second - time.Nanosecond)
	expectLen(t, q, 0, "just before 1s after Forget and AddRateLimited")
	clock.Advance(time.Nanosecond)
	expectLen(t, q, 1, "1s after Forget and AddRateLimited (the backoff restarts at baseDelay)")

	// Forget resets the count and nothing else: the waiting "web" stays.
	q.Forget("web")
	expectLen(t, q, 1, `after Forget("web") while "web" is waiting (Forget must not remove it from the queue)`)
}

// ----- concurrency -----

// TestQueueConcurrent runs workers and producers at the same time, like a
// controller with 8 workers and a busy watch stream. Run it with -race.
func TestQueueConcurrent(t *testing.T) {
	hangGuard(t)
	q := reconciler.NewQueue[string](time.Millisecond, 10*time.Millisecond)
	keys := names("key", 6)
	const workers, producers, adds = 8, 4, 400

	var (
		mu       sync.Mutex
		version  = map[string]int{} // bumped before every Add: "the object changed"
		seen     = map[string]int{} // the version a worker saw when it got the key
		inflight = map[string]int{}
		overlaps []string
	)

	var workersDone sync.WaitGroup
	for range workers {
		workersDone.Add(1)
		go func() {
			defer workersDone.Done()
			for {
				k, ok := q.Get()
				if !ok {
					return
				}
				mu.Lock()
				if inflight[k] > 0 {
					overlaps = append(overlaps, k)
				}
				inflight[k]++
				seen[k] = version[k]
				mu.Unlock()

				runtime.Gosched() // the reconcile

				mu.Lock()
				inflight[k]--
				mu.Unlock()
				q.Done(k)
			}
		}()
	}

	var producersDone sync.WaitGroup
	for p := range producers {
		producersDone.Add(1)
		go func() {
			defer producersDone.Done()
			for i := range adds {
				k := keys[(p*7+i)%len(keys)]
				mu.Lock()
				version[k]++
				mu.Unlock()
				q.Add(k)
				switch i % 10 {
				case 3:
					q.AddRateLimited(k)
				case 7:
					q.Forget(k)
				case 9:
					_ = q.NumRequeues(k) + q.Len()
				}
			}
		}()
	}
	within(t, "the producers", producersDone.Wait)

	// Every change must be seen by a reconcile that started after it.
	waitFor(t, "every key was processed after its last Add (an Add during processing must not get lost)", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, k := range keys {
			if seen[k] != version[k] {
				return false
			}
		}
		return true
	})

	q.ShutDown()
	within(t, "the workers after ShutDown", workersDone.Wait)
	if len(overlaps) > 0 {
		t.Fatalf("%d times a key was handed to a second worker while it was processing, e.g. %q", len(overlaps), overlaps[0])
	}
}
