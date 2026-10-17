package ratelimit_test

import (
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/yornik/go-practice/exercises/08-ratelimit"
)

// Step 3: Keyed, one bucket per client, and Sweep. The fake clock stands
// still until a test moves it with Advance.

func TestKeyedConfig(t *testing.T) {
	defer guard(t)()
	clock := newClock()
	tests := []struct {
		name      string
		rate      float64
		burst     int
		idle      time.Duration
		wantPanic bool
	}{
		{"valid", 10, 20, time.Minute, false},
		{"rate 0 and idle 0 are valid", 0, 1, 0, false},
		{"negative rate", -1, 20, time.Minute, true},
		{"rate NaN", math.NaN(), 20, time.Minute, true},
		{"rate +Inf", math.Inf(1), 20, time.Minute, true},
		{"burst 0", 10, 0, time.Minute, true},
		{"negative idle", 10, 20, -time.Second, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer guard(t)()
			v := panicValue(func() { ratelimit.NewKeyed(tc.rate, tc.burst, tc.idle, clock.Now) })
			switch {
			case tc.wantPanic && v == nil:
				t.Errorf("NewKeyed(%v, %d, %v, clock) returned a Keyed, want a panic: the config is invalid", tc.rate, tc.burst, tc.idle)
			case !tc.wantPanic && v != nil:
				t.Errorf("NewKeyed(%v, %d, %v, clock) panicked with %q, want a Keyed", tc.rate, tc.burst, tc.idle, v)
			}
		})
	}
}

func TestKeyedPerKey(t *testing.T) {
	defer guard(t)()
	h := newKeyed(t, 1, 2, time.Minute)
	h.length(0)
	h.allow("10.0.0.1", true)
	h.allow("10.0.0.1", true)
	h.allow("10.0.0.1", false) // its bucket is empty
	h.allow("10.0.0.2", true)  // another client is not affected
	h.allow("10.0.0.1", false)
	h.length(2)

	h.clock.Advance(time.Second) // 1 token more for each key
	h.allow("10.0.0.1", true)
	h.allow("10.0.0.1", false)
	h.allow("10.0.0.2", true) // 1 left + 1 refilled = 2
	h.allow("10.0.0.2", true)
	h.allow("10.0.0.2", false)
	h.length(2)
}

// TestKeyedSweep is the example from the README.
func TestKeyedSweep(t *testing.T) {
	defer guard(t)()
	h := newKeyed(t, 1, 2, time.Minute)
	h.allow("10.0.0.1", true)
	h.allow("10.0.0.1", true)
	h.allow("10.0.0.1", false)
	h.allow("10.0.0.2", true)
	h.clock.Advance(30 * time.Second)
	h.allow("10.0.0.3", true)
	h.sweep(0) // nobody has been idle for a minute yet

	h.clock.Advance(30 * time.Second)
	h.sweep(2) // .1 and .2: idle for 60s and full again. .3: idle for 30s only
	h.length(1)
	h.sweep(0) // nothing left to remove

	// A removed key that comes back gets a new, full bucket.
	h.allow("10.0.0.1", true)
	h.allow("10.0.0.1", true)
	h.allow("10.0.0.1", false)
	h.length(2)
}

func TestKeyedSweepIdleBoundary(t *testing.T) {
	defer guard(t)()

	t.Run("1ns short of idle: kept", func(t *testing.T) {
		defer guard(t)()
		h := newKeyed(t, 1, 2, time.Minute)
		h.allow("a", true)
		h.clock.Advance(time.Minute - time.Nanosecond)
		h.sweep(0)
		h.length(1)
	})

	t.Run("exactly idle: removed", func(t *testing.T) {
		defer guard(t)()
		h := newKeyed(t, 1, 2, time.Minute)
		h.allow("a", true)
		h.clock.Advance(time.Minute) // idle for at least a minute
		h.sweep(1)
		h.length(0)
	})
}

func TestKeyedSweepIsNotAUse(t *testing.T) {
	defer guard(t)()
	h := newKeyed(t, 1, 2, 10*time.Second)
	h.allow("a", true)
	h.clock.Advance(5 * time.Second)
	h.sweep(0) // full again, but used 5s ago: Sweep keeps it
	h.clock.Advance(5 * time.Second)
	h.sweep(1) // idle for 10s since the Allow; the Sweep at 5s does not change that
}

func TestKeyedDeniedCallIsAUse(t *testing.T) {
	defer guard(t)()
	h := newKeyed(t, 0.25, 1, 10*time.Second) // a token every 4s, burst 1
	h.allow("a", true)
	h.clock.Advance(2 * time.Second)
	h.allow("a", false) // denied, but the client is clearly active
	h.clock.Advance(8 * time.Second)
	h.sweep(0) // full at t0+10s, but the last use was 8s ago
	h.clock.Advance(2 * time.Second)
	h.sweep(1) // idle for 10s
}

func TestKeyedSweepOnlyFull(t *testing.T) {
	defer guard(t)()

	t.Run("not full yet: kept", func(t *testing.T) {
		// Removing a bucket that is not full would hand the client a new,
		// full one: a free burst for exactly the clients that use the most.
		defer guard(t)()
		h := newKeyed(t, 0.25, 4, 10*time.Second) // a token every 4s
		for range 4 {
			h.allow("a", true)
		}
		h.clock.Advance(10 * time.Second)
		h.sweep(0) // idle for 10s, but only 2.5 tokens
		h.length(1)
		h.clock.Advance(6 * time.Second)
		h.sweep(1) // 4 tokens at t0+16s: full
	})

	t.Run("refill counts", func(t *testing.T) {
		defer guard(t)()
		h := newKeyed(t, 1, 5, 10*time.Second)
		h.allow("a", true) // 4 tokens left at t0
		h.clock.Advance(10 * time.Second)
		h.sweep(1) // 4 + 10 refilled, capped at 5: full, although the stored count is 4
	})

	t.Run("rate 0: never full again", func(t *testing.T) {
		defer guard(t)()
		h := newKeyed(t, 0, 2, 10*time.Second)
		h.allow("a", true)
		h.allow("b", true)
		h.allow("b", true)
		h.allow("b", false)
		h.clock.Advance(24 * time.Hour)
		h.sweep(0)
		h.length(2)
		h.allow("a", true) // the second token of its burst
		h.allow("a", false)
	})
}

func TestKeyedConcurrent(t *testing.T) {
	defer guard(t)()
	clock := newClock()
	k := ratelimit.NewKeyed(1, 20, time.Minute, clock.Now)

	const nKeys, perKey = 8, 25
	const workers = nKeys * perKey
	var allowed [nKeys]atomic.Int64
	// 200 goroutines share 8 keys, 25 goroutines per key, and each also
	// makes one call for a key of its own, so new buckets keep being added.
	// 4 more goroutines call Len and Sweep while that happens. The clock
	// stands still, so no key is idle and Sweep must remove nothing.
	parallel(t, workers+4, func(i int) {
		if i >= workers {
			for range 100 {
				_ = k.Len()
				_ = k.Sweep()
			}
			return
		}
		key := i % nKeys
		for j := range 10 {
			if k.Allow(fmt.Sprintf("client-%d", key)) {
				allowed[key].Add(1)
			}
			if j == 5 && !k.Allow(fmt.Sprintf("once-%d", i)) {
				t.Errorf("the first Allow(\"once-%d\") = false, want true: a new bucket is full", i)
			}
		}
	})
	for key := range allowed {
		if got := allowed[key].Load(); got != 20 {
			t.Errorf("Allow(\"client-%d\") returned true %d times in 250 concurrent calls with the clock standing still, want exactly 20 (the burst)", key, got)
		}
	}
	if got := k.Len(); got != nKeys+workers {
		t.Errorf("Len() = %d, want %d: %d shared keys and %d keys used once. The clock stands still, "+
			"so no bucket is idle and the concurrent Sweep calls must not remove any", got, nKeys+workers, nKeys, workers)
	}
}

func TestKeyedConcurrentNewKey(t *testing.T) {
	defer guard(t)()
	clock := newClock()
	k := ratelimit.NewKeyed(1, 3, time.Minute, clock.Now)
	// 32 goroutines hit a key that has no bucket yet, all at the same moment.
	// A check for the key followed by a separate insert lets two of them
	// create a bucket each, and the client gets two bursts.
	const rounds = 200
	for round := range rounds {
		key := fmt.Sprintf("client-%d", round)
		var allowed atomic.Int64
		parallel(t, 32, func(int) {
			if k.Allow(key) {
				allowed.Add(1)
			}
		})
		if got := allowed.Load(); got != 3 {
			t.Fatalf("32 concurrent first calls of Allow(%q): %d returned true, want exactly 3 (the burst)", key, got)
		}
	}
	if got := k.Len(); got != rounds {
		t.Errorf("Len() = %d, want %d", got, rounds)
	}
}

func TestKeyedRealClock(t *testing.T) {
	defer guard(t)()
	// now == nil means time.Now. At rate 1000, real time refills a token
	// every millisecond.
	k := ratelimit.NewKeyed(1000, 1, time.Hour, nil)
	if !k.Allow("a") {
		t.Fatal(`NewKeyed(1000, 1, time.Hour, nil): the first Allow("a") = false, want true: a new bucket is full`)
	}
	deadline := time.Now().Add(timeout)
	for !k.Allow("a") {
		if time.Now().After(deadline) {
			t.Fatalf(`NewKeyed(1000, 1, time.Hour, nil): Allow("a") stayed false for %v, want true after about 1ms: with a nil clock, use time.Now`, timeout)
		}
		time.Sleep(ms)
	}
}
