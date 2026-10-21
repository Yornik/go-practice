package healthcheck_test

// Step 4: caching the readiness result.
//
//	go test -race -run TestCache -v ./exercises/40-healthcheck/

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	healthcheck "github.com/yornik/go-practice/exercises/40-healthcheck"
)

func TestCacheDisabled(t *testing.T) {
	clock := newFakeClock() // never moves: only CacheTTL decides here
	var calls atomic.Int32
	h := newHealth(t, healthcheck.Options{CacheTTL: 0, Now: clock.Now}, check("postgres", true, nil, &calls))
	handler := h.Handler()
	for i := range 3 {
		getReadyz(t, handler)
		if got, want := calls.Load(), int32(i+1); got != want {
			t.Fatalf("CacheTTL 0: after %d requests the check ran %d times, want %d", i+1, got, want)
		}
	}
}

func TestCacheTTL(t *testing.T) {
	clock := newFakeClock()
	var calls atomic.Int32
	h := newHealth(t, healthcheck.Options{CacheTTL: 10 * time.Second, Now: clock.Now},
		check("postgres", true, nil, &calls))
	handler := h.Handler()

	steps := []struct {
		advance   time.Duration // move the clock this much before the request
		wantCalls int32         // total runs of the check after the request
		why       string
	}{
		{0, 1, "the first request runs the checks"},
		{0, 1, "same instant: the result is reused"},
		{9*time.Second + 999*time.Millisecond, 1, "9.999s after the run: still fresh"},
		{time.Millisecond, 2, "10s after the run: expired, so the checks run again"},
		{5 * time.Second, 2, "5s after the second run: fresh"},
		{5 * time.Second, 3, "10s after the second run: expired"},
	}
	for i, s := range steps {
		clock.Advance(s.advance)
		resp, rep := getReadyz(t, handler)
		wantReadiness(t, resp, rep, http.StatusOK, "ok")
		if got := calls.Load(); got != s.wantCalls {
			t.Fatalf("request %d (%s): the check ran %d times in total, want %d", i+1, s.why, got, s.wantCalls)
		}
	}
}

func TestCacheFailedResult(t *testing.T) {
	clock := newFakeClock()
	var calls atomic.Int32
	h := newHealth(t, healthcheck.Options{CacheTTL: 10 * time.Second, Now: clock.Now},
		check("postgres", true, errors.New("too many connections"), &calls))
	handler := h.Handler()
	for i := range 3 {
		resp, rep := getReadyz(t, handler)
		wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "fail")
		if c := findCheck(t, rep, "postgres"); c.Error != "too many connections" {
			t.Errorf("request %d: check %q error = %q, want %q", i+1, "postgres", c.Error, "too many connections")
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("3 requests within the TTL ran a failing check %d times, want 1: failures are cached too", got)
	}
}

// TestCacheFromEndOfRun: the TTL counts from the end of the run. The first
// run takes 4s (on the fake clock) and the TTL is 5s, so its result is fresh
// until 9s after the start, not 5s.
func TestCacheFromEndOfRun(t *testing.T) {
	clock := newFakeClock()
	var calls atomic.Int32
	h := newHealth(t, healthcheck.Options{CacheTTL: 5 * time.Second, Now: clock.Now}, healthcheck.Check{
		Name: "postgres", Critical: true, Timeout: time.Minute,
		Fn: func(context.Context) error {
			if calls.Add(1) == 1 {
				clock.Advance(4 * time.Second) // the first run is slow
			}
			return nil
		},
	})
	handler := h.Handler()

	getReadyz(t, handler) // runs from 0s to 4s
	clock.Advance(4 * time.Second)
	getReadyz(t, handler) // 8s: 4s after the end of the run
	if got := calls.Load(); got != 1 {
		t.Fatalf("8s after the start of a 4s run with CacheTTL 5s, the check ran %d times, want 1: the TTL counts from the end of the run", got)
	}
	clock.Advance(time.Second)
	getReadyz(t, handler) // 9s: 5s after the end of the run
	if got := calls.Load(); got != 2 {
		t.Fatalf("5s after the end of the run with CacheTTL 5s, the check ran %d times, want 2", got)
	}
}

// TestCacheConcurrentRequests: 10 probes arrive while a slow run is in
// progress. They wait for it and use its result instead of each starting
// their own run against a database that is already slow.
func TestCacheConcurrentRequests(t *testing.T) {
	const n = 10
	clock := newFakeClock() // never moves, so the result stays fresh
	var calls atomic.Int32
	slow := newGate(t)
	h := newHealth(t, healthcheck.Options{CacheTTL: time.Minute, Now: clock.Now}, healthcheck.Check{
		Name: "postgres", Critical: true, Timeout: time.Minute,
		Fn: func(ctx context.Context) error {
			calls.Add(1)
			select {
			case <-slow.ch:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	handler := h.Handler()

	pending := make([]<-chan result, n)
	for i := range pending {
		pending[i] = startRequest(handler, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	}
	waitFor(t, "the check to start", func() bool { return calls.Load() >= 1 })
	// Give the other requests time to arrive while the run is in progress.
	for deadline := time.Now().Add(window); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if got := calls.Load(); got > 1 {
			t.Fatalf("%d concurrent requests started %d runs of the check while the first run was in progress, want 1", n, got)
		}
	}
	slow.open()
	for i, ch := range pending {
		resp := await(t, fmt.Sprintf("concurrent GET /readyz #%d", i+1), ch)
		rep := decodeReport(t, resp)
		wantReadiness(t, resp, rep, http.StatusOK, "ok")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("%d concurrent requests ran the check %d times, want 1", n, got)
	}
}
