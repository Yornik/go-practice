package loadbalancer_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// In these tests EjectAfter is 0 (no passive ejection) and the strategy is
// LeastConnections, so every request tries the backends in list order: the
// dead ones first.

func TestRetrySafeMethods(t *testing.T) {
	tests := []struct {
		method   string
		retried  bool
		wantCode int
	}{
		{http.MethodGet, true, http.StatusOK},
		{http.MethodHead, true, http.StatusOK},
		{http.MethodOptions, true, http.StatusOK},
		{http.MethodPost, false, http.StatusBadGateway},
		{http.MethodPut, false, http.StatusBadGateway},
		{http.MethodPatch, false, http.StatusBadGateway},
		{http.MethodDelete, false, http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			e := newEnv(t)
			dead := e.deadURL()
			live := e.upstream("live")
			c := e.config(dead, live.URL)
			c.MaxRetries = 1
			lb := newLB(t, c)

			// No body: only the method decides.
			rec := do(t, lb, httptest.NewRequest(tc.method, "http://lb.example/items/1", nil))
			if rec.Code != tc.wantCode {
				t.Errorf("%s with the first backend down: status %d, want %d", tc.method, rec.Code, tc.wantCode)
			}
			wantLive := 0
			if tc.retried {
				wantLive = 1
			}
			if got := live.hits.Load(); int(got) != wantLive {
				t.Errorf("%s reached the live backend %d times, want %d (retried: %v)", tc.method, got, wantLive, tc.retried)
			}
			if got := e.attempts(dead); got != 1 {
				t.Errorf("the LB sent %d requests to the dead backend, want 1", got)
			}
		})
	}
}

func TestRetryLimit(t *testing.T) {
	// Two dead backends, then a live one. MaxRetries is the number of extra
	// attempts after the first one.
	tests := []struct {
		maxRetries int
		wantCode   int
		wantLive   int32
	}{
		{0, http.StatusBadGateway, 0}, // dead-1
		{1, http.StatusBadGateway, 0}, // dead-1, dead-2
		{2, http.StatusOK, 1},         // dead-1, dead-2, live
		{5, http.StatusOK, 1},         // a limit, not a target
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("MaxRetries=%d", tc.maxRetries), func(t *testing.T) {
			e := newEnv(t)
			dead1, dead2 := e.deadURL(), e.deadURL()
			live := e.upstream("live")
			c := e.config(dead1, dead2, live.URL)
			c.MaxRetries = tc.maxRetries
			lb := newLB(t, c)

			rec := get(t, lb, "/")
			if rec.Code != tc.wantCode {
				t.Errorf("status %d, want %d", rec.Code, tc.wantCode)
			}
			if got := live.hits.Load(); got != tc.wantLive {
				t.Errorf("the live backend got %d requests, want %d", got, tc.wantLive)
			}
			wantDead2 := min(tc.maxRetries, 1)
			if a1, a2 := e.attempts(dead1), e.attempts(dead2); a1 != 1 || a2 != wantDead2 {
				t.Errorf("attempts on dead-1, dead-2 = %d, %d; want 1, %d", a1, a2, wantDead2)
			}
			for _, u := range []string{dead1, dead2, live.URL} {
				if got := status(t, lb, u).InFlight; got != 0 {
					t.Errorf("InFlight of %s = %d after the request, want 0", u, got)
				}
			}
		})
	}
}

func TestRetryOnlyOnOtherBackends(t *testing.T) {
	// A retry never goes to a backend this request already tried, so with
	// every backend down the request stops after one attempt per backend.
	e := newEnv(t)
	dead1, dead2 := e.deadURL(), e.deadURL()
	c := e.config(dead1, dead2)
	c.MaxRetries = 5
	lb := newLB(t, c)
	rec := get(t, lb, "/")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status %d, want 502", rec.Code)
	}
	if a1, a2 := e.attempts(dead1), e.attempts(dead2); a1 != 1 || a2 != 1 {
		t.Errorf("attempts on dead-1, dead-2 = %d, %d; want 1, 1", a1, a2)
	}
}

func TestRetryNotOn5xx(t *testing.T) {
	// A 5xx is an answer: the backend may have done the work. It goes back
	// to the client as it is.
	e := newEnv(t)
	failing := e.upstream("failing")
	failing.status.Store(http.StatusServiceUnavailable)
	live := e.upstream("live")
	c := e.config(failing.URL, live.URL)
	c.MaxRetries = 2
	lb := newLB(t, c)
	checkResponse(t, get(t, lb, "/"), http.StatusServiceUnavailable, "failing")
	if got := live.hits.Load(); got != 0 {
		t.Errorf("the live backend got %d requests, want 0: a 5xx response is not retried", got)
	}
}

func TestRetryNotWithBody(t *testing.T) {
	// The first attempt has consumed the request body, so a request with a
	// body is never retried, whatever its method.
	e := newEnv(t)
	dead := e.deadURL()
	live := e.upstream("live")
	c := e.config(dead, live.URL)
	c.MaxRetries = 1
	lb := newLB(t, c)
	req := httptest.NewRequest(http.MethodGet, "http://lb.example/search", strings.NewReader(`{"q":"x"}`))
	rec := do(t, lb, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("GET with a body, first backend down: status %d, want 502", rec.Code)
	}
	if got := live.hits.Load(); got != 0 {
		t.Errorf("the live backend got %d requests, want 0", got)
	}
}

func TestRetryNotWhenClientCancels(t *testing.T) {
	// The client gives up while the backend is still working. The transport
	// then fails with the client's context error. Nobody waits for an answer
	// any more, so the LB must not try another backend.
	e := newEnv(t)
	slow, other := e.upstream("slow"), e.upstream("other")
	slow.hold.Store(true)
	c := e.config(slow.URL, other.URL)
	c.MaxRetries = 1
	lb := newLB(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "http://lb.example/report", nil).WithContext(ctx)
	ch := start(lb, req)
	e.waitArrival(t, "slow")
	cancel()
	await(t, ch)

	if got := e.attempts(other.URL); got != 0 {
		t.Errorf("the LB sent %d requests to the other backend after the client cancelled, want 0", got)
	}
	waitInFlight(t, lb, slow.URL, 0)
}
