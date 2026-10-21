package ratelimit_test

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	ratelimit "github.com/yornik/go-practice/exercises/33-ratelimit"
)

// Step 2: Reserve and Cancel. As in step 1, most tests use rate 4 and
// burst 4: one token every 250ms, at most 4.

func TestReserveWithinTokens(t *testing.T) {
	defer guard(t)()
	h := newLim(t, 4, 4)
	h.reserve(3, 0) // enough tokens: no wait
	h.tokens(1)
	h.reserve(1, 0)
	h.tokens(0)
}

func TestReserveQueues(t *testing.T) {
	defer guard(t)()

	t.Run("each reservation waits behind the debt", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.reserve(4, 0)
		h.reserve(1, 250*ms) // count -1: one token's worth of debt
		h.reserve(2, 750*ms) // count -3: (2 - (-1)) / 4 seconds
		h.tokens(-3)
		h.allow(false) // nothing for Allow while the count is negative
		h.allowN(1, false)

		h.clock.Advance(500 * ms)
		h.tokens(-1)
		h.reserve(1, 500*ms) // count -2

		h.clock.Advance(500 * ms)
		h.tokens(0)
		h.allow(false) // the debt is paid, but no token has come in yet
		h.clock.Advance(250 * ms)
		h.allow(true)
	})

	t.Run("a fraction of a token shortens the wait", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.allowN(4, true)
		h.clock.Advance(375 * ms) // 1.5 tokens
		h.reserve(2, 125*ms)      // 0.5 missing, at 4 tokens per second
		h.tokens(-0.5)
	})

	t.Run("rate below 1", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 0.5, 1) // a token every 2s
		h.reserve(1, 0)
		h.reserve(1, 2*time.Second)
		h.reserve(1, 4*time.Second)
	})
}

func TestReserveErrors(t *testing.T) {
	defer guard(t)()

	t.Run("n > burst", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.reserveErr(5, ratelimit.ErrExceedsBurst)
		h.tokens(4) // nothing taken
		h.reserve(4, 0)
		h.reserve(4, time.Second)
		h.reserveErr(5, ratelimit.ErrExceedsBurst) // also while in debt
		h.tokens(-4)
	})

	t.Run("rate 0 and too few tokens", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 0, 3)
		h.reserve(2, 0) // rate 0 is fine while there are enough tokens
		h.tokens(1)
		h.reserveErr(2, ratelimit.ErrNoRefill)
		h.tokens(1) // nothing taken
		h.reserve(1, 0)
		h.clock.Advance(time.Hour)
		h.reserveErr(1, ratelimit.ErrNoRefill)
		h.tokens(0)
	})
}

func TestReserveCancel(t *testing.T) {
	defer guard(t)()

	t.Run("gives a reservation back", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.reserve(4, 0)
		h.reserve(2, 500*ms) // count -2
		h.l.Cancel(2)        // the caller gave up before its turn
		h.tokens(0)
		h.reserve(1, 250*ms) // not 750ms: the cancelled reservation is gone
	})

	t.Run("never above burst", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.allow(true)
		h.l.Cancel(1) // a refund: back to 4
		h.tokens(4)
		h.l.Cancel(3) // already full
		h.tokens(4)
		h.allowN(4, true)
		h.allow(false)
	})

	t.Run("keeps the refill", func(t *testing.T) {
		defer guard(t)()
		h := newLim(t, 4, 4)
		h.allowN(4, true)
		h.clock.Advance(500 * ms) // 2 tokens by refill
		h.l.Cancel(1)
		h.tokens(3)
	})
}

func TestReserveConcurrent(t *testing.T) {
	defer guard(t)()

	// Every caller must get its own slot: 4 callers can go now, the others
	// one every 250ms. Two callers with the same wait mean that two Reserve
	// calls read the same count. A fresh Limiter per round makes the calls
	// overlap many times, not just once.
	const n = 16
	want := make([]time.Duration, n)
	for i := range want {
		want[i] = time.Duration(max(0, i-3)) * 250 * ms
	}
	for round := range 100 {
		clock := newClock()
		l := ratelimit.New(4, 4, clock.Now)
		waits := make([]time.Duration, n)
		errs := make([]error, n)
		parallel(t, n, func(i int) {
			waits[i], errs[i] = l.Reserve(1)
		})
		if err := errors.Join(errs...); err != nil {
			t.Fatalf("round %d: concurrent Reserve(1) calls returned errors, want nil:\n%v", round, err)
		}
		slices.Sort(waits)
		for i := range waits {
			if !closeEnough(waits[i], want[i]) {
				t.Fatalf("round %d: %d concurrent Reserve(1) calls, sorted waits:\n got %v\nwant %v", round, n, waits, want)
			}
		}
	}
}

func TestReserveCancelConcurrent(t *testing.T) {
	defer guard(t)()
	clock := newClock()
	l := ratelimit.New(4, 4, clock.Now)
	parallel(t, 20, func(int) {
		for range 50 {
			if d, err := l.Reserve(2); err != nil {
				t.Errorf("Reserve(2) = %v, %v; want a nil error", d, err)
				return
			}
			_ = l.Tokens()
			l.Cancel(2)
		}
	})
	if got := l.Tokens(); math.Abs(got-4) > tokenTolerance {
		t.Errorf("after 1000 concurrent Reserve(2) + Cancel(2) pairs with the clock standing still, Tokens() = %v, want 4", got)
	}
}
