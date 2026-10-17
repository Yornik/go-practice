package healthcheck_test

// Step 5: draining.
//
//	go test -race -run TestDraining -v ./exercises/15-healthcheck/

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	healthcheck "github.com/yornik/go-practice/exercises/15-healthcheck"
)

func TestDrainingReadyz(t *testing.T) {
	var calls atomic.Int32
	h := newHealth(t, healthcheck.Options{}, check("postgres", true, nil, &calls))
	handler := h.Handler()

	within(t, "SetDraining", h.SetDraining)
	for i := range 2 { // and it stays that way
		resp, rep := getReadyz(t, handler)
		wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "draining")
		if len(rep.Checks) != 0 {
			t.Errorf("request %d: checks = %+v, want [] while draining", i+1, rep.Checks)
		}
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("/readyz ran a check %d times while draining, want 0", n)
	}
}

func TestDrainingIgnoresCache(t *testing.T) {
	clock := newFakeClock()
	var calls atomic.Int32
	h := newHealth(t, healthcheck.Options{CacheTTL: time.Minute, Now: clock.Now}, check("postgres", true, nil, &calls))
	handler := h.Handler()

	resp, rep := getReadyz(t, handler)
	wantReadiness(t, resp, rep, http.StatusOK, "ok")
	within(t, "SetDraining", h.SetDraining)
	resp, rep = getReadyz(t, handler) // the cached "ok" is still fresh, but draining wins
	wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "draining")
	if n := calls.Load(); n != 1 {
		t.Errorf("the check ran %d times, want 1", n)
	}
}

func TestDrainingLivez(t *testing.T) {
	h := newHealth(t, healthcheck.Options{}, check("postgres", true, nil, nil))
	within(t, "SetDraining", h.SetDraining)
	// Draining is not dead. Liveness answers one question only: is this
	// process stuck? A /livez that fails during the drain invites a restart
	// in the middle of it.
	resp := do(t, h.Handler(), http.MethodGet, "/livez")
	if resp.Code != http.StatusOK || strings.TrimSpace(resp.Body) != "ok" {
		t.Errorf("GET /livez while draining = %d %q, want 200 \"ok\"", resp.Code, resp.Body)
	}
}

// TestDrainingConcurrent: in a real server, Serve calls SetDraining in its
// own goroutine while probes run in the server's goroutines. Run it with
// -race: the draining state must be safe to write and read at the same time.
func TestDrainingConcurrent(t *testing.T) {
	h := newHealth(t, healthcheck.Options{}, check("postgres", true, nil, nil))
	handler := h.Handler()

	pending := make([]<-chan result, 8)
	for i := range pending {
		pending[i] = startRequest(handler, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	}
	within(t, "SetDraining while /readyz requests run", h.SetDraining)
	for i, ch := range pending {
		resp := await(t, fmt.Sprintf("concurrent GET /readyz #%d", i+1), ch)
		// Each request raced with SetDraining, so both answers are right.
		if rep := decodeReport(t, resp); rep.Status != "ok" && rep.Status != "draining" {
			t.Errorf("concurrent GET /readyz #%d: status %q, want \"ok\" or \"draining\"", i+1, rep.Status)
		}
	}
	resp, rep := getReadyz(t, handler)
	wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "draining")
}

// TestDrainingDuringSlowRun: a /readyz run hangs on a slow check (Timeout
// 1 minute). Meanwhile, /livez, SetDraining and a new /readyz must answer at
// once instead of waiting for that run.
func TestDrainingDuringSlowRun(t *testing.T) {
	slow := newGate(t)
	started := newEvent()
	h := newHealth(t, healthcheck.Options{}, healthcheck.Check{
		Name: "postgres", Critical: true, Timeout: time.Minute,
		Fn: func(ctx context.Context) error {
			started.fire()
			select {
			case <-slow.ch:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	handler := h.Handler()

	first := startRequest(handler, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	waitClosed(t, started.done(), "the slow check to start")

	resp := await(t, "GET /livez while a /readyz run is in progress",
		startRequest(handler, httptest.NewRequest(http.MethodGet, "/livez", nil)))
	if resp.Code != http.StatusOK {
		t.Errorf("GET /livez while a /readyz run is in progress = %d, want 200", resp.Code)
	}

	within(t, "SetDraining while a /readyz run is in progress", h.SetDraining)

	resp = await(t, "GET /readyz after SetDraining while another /readyz run is in progress",
		startRequest(handler, httptest.NewRequest(http.MethodGet, "/readyz", nil)))
	rep := decodeReport(t, resp)
	wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "draining")

	slow.open()
	await(t, "the first GET /readyz", first) // its answer is not specified
}
