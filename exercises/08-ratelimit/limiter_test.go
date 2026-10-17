package ratelimit_test

import (
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/yornik/go-practice/exercises/08-ratelimit"
)

// Step 1: one Limiter. New, Allow, AllowN and Tokens.
//
// Most tests use rate 4 and burst 4: one token every 250ms, at most 4. The
// fake clock stands still until a test moves it with Advance.

func TestLimiterStartsFull(t *testing.T) {
	defer guard(t)()
	tests := []struct {
		rate  float64
		burst int
	}{
		{4, 4},
		{0.5, 1},
		{0, 3}, // rate 0: no refill, but the first burst is there
		{1000, 250},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("rate=%v,burst=%d", tc.rate, tc.burst), func(t *testing.T) {
			defer guard(t)()
			h := newLim(t, tc.rate, tc.burst)
			h.tokens(float64(tc.burst))
		})
	}
}

func TestLimiterConfig(t *testing.T) {
	defer guard(t)()
	clock := newClock()
	tests := []struct {
		name      string
		rate      float64
		burst     int
		wantPanic bool
	}{
		{"rate 0 is valid", 0, 1, false},
		{"rate below 1 is valid", 0.5, 10, false},
		{"burst 1 is valid", 100, 1, false},
		{"negative rate", -1, 10, true},
		{"rate NaN", math.NaN(), 10, true},
		{"rate +Inf", math.Inf(1), 10, true},
		{"burst 0", 10, 0, true},
		{"negative burst", 10, -5, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer guard(t)()
			v := panicValue(func() { ratelimit.New(tc.rate, tc.burst, clock.Now) })
			switch {
			case tc.wantPanic && v == nil:
				t.Errorf("New(%v, %d, clock) returned a Limiter, want a panic: the config is invalid", tc.rate, tc.burst)
			case !tc.wantPanic && v != nil:
				t.Errorf("New(%v, %d, clock) panicked with %q, want a Limiter", tc.rate, tc.burst, v)
			}
		})
	}
}

func TestLimiterAllowUsesUpTheBurst(t *testing.T) {
	defer guard(t)()
	h := newLim(t, 1, 5)
	for range 5 {
		h.allow(true)
	}
	h.allow(false)
	h.allow(false)
	h.tokens(0) // denied calls take nothing: the count is 0, not negative
}

func TestLimiterAllowN(t *testing.T) {
	defer guard(t)()
	h := newLim(t, 4, 4)
	h.allowN(3, true)
	h.tokens(1)
	h.allowN(2, false) // all or nothing: 1 token is not enough for 2
	h.tokens(1)        // and the failed call took nothing
	h.allowN(1, true)
	h.tokens(0)

	h.clock.Advance(time.Second)
	h.allowN(4, true) // n == burst works when the bucket is full

	h.clock.Advance(time.Hour)
	h.allowN(5, false) // n > burst: the bucket never holds 5 tokens
	h.tokens(4)
}

func TestLimiterRefill(t *testing.T) {
	defer guard(t)()

	t.Run("one token every 1/rate seconds", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.allowN(4, true)
		h.clock.Advance(250 * ms)
		h.allow(true)
		h.allow(false)
		h.clock.Advance(750 * ms)
		h.tokens(3)
		h.allowN(3, true)
		h.allow(false)
	})

	t.Run("fractions add up", func(t *testing.T) {
		// A solution that keeps whole tokens only, or forgets the time of a
		// denied call, loses the first half token or counts it twice.
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.allowN(4, true)
		h.clock.Advance(125 * ms)
		h.allow(false)
		h.tokens(0.5)
		h.clock.Advance(125 * ms)
		h.allow(true)
		h.tokens(0)
	})

	t.Run("rate below 1", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 0.5, 2) // a token every 2s
		h.allowN(2, true)
		h.clock.Advance(time.Second)
		h.allow(false)
		h.tokens(0.5)
		h.clock.Advance(time.Second)
		h.allow(true)
	})

	t.Run("capped at burst", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.allowN(4, true)
		h.clock.Advance(time.Hour) // 14400 tokens' worth of time
		h.tokens(4)
		h.allowN(4, true)
		h.allow(false)
	})

	t.Run("rate 0 never refills", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 0, 3)
		h.allowN(3, true)
		h.clock.Advance(24 * time.Hour)
		h.tokens(0)
		h.allow(false)
	})
}

func TestLimiterClockBackwards(t *testing.T) {
	defer guard(t)()

	t.Run("no tokens lost", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.allowN(2, true)
		h.clock.Advance(-time.Hour)
		h.tokens(2) // not 2 + 4 × (-3600)
		h.allowN(2, true)
		h.allow(false)
	})

	t.Run("refill continues from the earlier time", func(t *testing.T) {
		// A limiter that waits until the clock passes t0 again refills
		// nothing for an hour: every client is blocked for an hour.
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.allowN(4, true)
		h.clock.Advance(-time.Hour)
		h.allow(false)
		h.tokens(0)
		h.clock.Advance(500 * ms)
		h.tokens(2)
		h.allowN(2, true)
	})
}

func TestLimiterConcurrent(t *testing.T) {
	defer guard(t)()

	t.Run("Allow", func(t *testing.T) {
		defer guard(t)()
		clock := newClock()
		l := ratelimit.New(1, 100, clock.Now)
		var allowed atomic.Int64
		parallel(t, 50, func(int) {
			for range 20 {
				if l.Allow() {
					allowed.Add(1)
				}
				_ = l.Tokens()
			}
		})
		if got := allowed.Load(); got != 100 {
			t.Errorf("50 goroutines called Allow 1000 times with the clock standing still: %d calls returned true, want exactly 100 (the burst)", got)
		}
	})

	t.Run("AllowN", func(t *testing.T) {
		defer guard(t)()
		clock := newClock()
		l := ratelimit.New(1, 99, clock.Now)
		var allowed atomic.Int64
		parallel(t, 40, func(int) {
			for range 10 {
				if l.AllowN(3) {
					allowed.Add(1)
				}
			}
		})
		if got := allowed.Load(); got != 33 {
			t.Errorf("40 goroutines called AllowN(3) 400 times with the clock standing still: %d calls returned true, want exactly 33 (99 tokens / 3)", got)
		}
	})

	// Draining one big bucket races for the last token only once. Here 32
	// goroutines drain a small bucket at the same moment, round after round,
	// so every round is a race for the last tokens. A check ("enough
	// tokens?") and a take in separate critical sections lets two goroutines
	// take the same token.
	lastTokens := func(t *testing.T, burst, n int) {
		t.Helper()
		clock := newClock()
		want := int64(burst / n)
		for round := range 500 {
			l := ratelimit.New(1, burst, clock.Now)
			var allowed atomic.Int64
			parallel(t, 32, func(int) {
				if l.AllowN(n) {
					allowed.Add(1)
				}
			})
			if got := allowed.Load(); got != want {
				t.Fatalf("round %d: 32 goroutines called AllowN(%d) at the same moment on a full bucket of %d: %d returned true, want exactly %d",
					round, n, burst, got, want)
			}
		}
	}
	t.Run("last token AllowN(1)", func(t *testing.T) {
		defer guard(t)()
		lastTokens(t, 3, 1)
	})
	t.Run("last tokens AllowN(2)", func(t *testing.T) {
		defer guard(t)()
		lastTokens(t, 5, 2)
	})
}

func TestLimiterRealClock(t *testing.T) {
	defer guard(t)()
	// now == nil means time.Now. At rate 1000, real time refills a token
	// every millisecond.
	l := ratelimit.New(1000, 1, nil)
	if !l.Allow() {
		t.Fatal("New(1000, 1, nil): the first Allow() = false, want true: a new Limiter is full")
	}
	deadline := time.Now().Add(timeout)
	for !l.Allow() {
		if time.Now().After(deadline) {
			t.Fatalf("New(1000, 1, nil): Allow() stayed false for %v, want true after about 1ms: with a nil clock, use time.Now", timeout)
		}
		time.Sleep(ms)
	}
}
