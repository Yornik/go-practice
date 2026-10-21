package loadbalancer_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// In these tests the strategy is LeastConnections, so a request goes to the
// first eligible backend in the list. Time only moves with e.clock.Advance.

func TestEjectAfterConsecutive5xx(t *testing.T) {
	e := newEnv(t)
	bad, good := e.upstream("bad"), e.upstream("good")
	bad.status.Store(http.StatusInternalServerError)
	c := e.config(bad.URL, good.URL)
	c.EjectAfter = 3
	c.EjectFor = 30 * time.Second
	lb := newLB(t, c)

	// 500s at 12:00:00, 12:00:01 and 12:00:02.
	for i := 1; i <= 3; i++ {
		if i > 1 {
			e.clock.Advance(time.Second)
		}
		// The 5xx goes to the client: it is not retried.
		checkResponse(t, get(t, lb, "/"), http.StatusInternalServerError, "bad")
		if got, want := status(t, lb, bad.URL).Ejected, i == 3; got != want {
			t.Fatalf("after %d consecutive 500s: Ejected = %v, want %v (EjectAfter 3)", i, got, want)
		}
	}

	// Ejected: the next requests go to the other backend.
	for range 3 {
		checkResponse(t, get(t, lb, "/"), http.StatusOK, "good")
	}

	// Still ejected 1ns before EjectFor has passed since the third failure.
	e.clock.Advance(30*time.Second - time.Nanosecond)
	if !status(t, lb, bad.URL).Ejected {
		t.Fatalf("Ejected = false at 12:00:31.999999999, want true: ejected until the time of the third failure (12:00:02) + EjectFor (30s)")
	}
	checkResponse(t, get(t, lb, "/"), http.StatusOK, "good")

	// Back when exactly EjectFor has passed.
	e.clock.Advance(time.Nanosecond)
	if status(t, lb, bad.URL).Ejected {
		t.Fatalf("Ejected = true at 12:00:32, want false: the third failure (12:00:02) + EjectFor (30s) has passed")
	}
	checkResponse(t, get(t, lb, "/"), http.StatusInternalServerError, "bad")

	// The count started again from 0: one failure is not enough.
	if status(t, lb, bad.URL).Ejected {
		t.Fatalf("ejected again after 1 failure: after an ejection, the backend needs EjectAfter new consecutive failures")
	}
	if got := bad.hits.Load(); got != 4 {
		t.Errorf("the bad backend got %d requests, want 4", got)
	}
}

func TestEjectCountsOnlyConsecutiveFailures(t *testing.T) {
	e := newEnv(t)
	flaky := e.upstream("flaky")
	c := e.config(flaky.URL)
	c.EjectAfter = 3
	c.EjectFor = time.Minute
	lb := newLB(t, c)

	// A success, or any status below 500, resets the count.
	for i, code := range []int{500, 502, 200, 503, 500, 404, 500, 500, 429, 500, 500} {
		flaky.status.Store(int32(code))
		get(t, lb, "/")
		if status(t, lb, flaky.URL).Ejected {
			t.Fatalf("ejected after response %d (status %d): never 3 failures in a row yet", i+1, code)
		}
	}
	flaky.status.Store(http.StatusInternalServerError)
	get(t, lb, "/") // the third 500 in a row
	if !status(t, lb, flaky.URL).Ejected {
		t.Fatalf("not ejected after 3 consecutive 500s")
	}
}

func TestEjectOnTransportErrors(t *testing.T) {
	// A transport error counts as a failure of the backend that caused it,
	// also when a retry on another backend then succeeds.
	e := newEnv(t)
	dead := e.deadURL()
	good := e.upstream("good")
	c := e.config(dead, good.URL)
	c.MaxRetries = 1
	c.EjectAfter = 2
	c.EjectFor = time.Minute
	lb := newLB(t, c)

	for i := 1; i <= 2; i++ {
		checkResponse(t, get(t, lb, "/"), http.StatusOK, "good") // dead, then retried on good
		if got, want := status(t, lb, dead).Ejected, i == 2; got != want {
			t.Fatalf("after %d connection errors: Ejected = %v, want %v (EjectAfter 2)", i, got, want)
		}
	}
	checkResponse(t, get(t, lb, "/"), http.StatusOK, "good")
	if got := e.attempts(dead); got != 2 {
		t.Errorf("the LB sent %d requests to the dead backend, want 2: an ejected backend gets none", got)
	}
}

func TestEjectAllBackends(t *testing.T) {
	// With every backend ejected there is nothing to send to: 503.
	e := newEnv(t)
	bad := e.upstream("bad")
	bad.status.Store(http.StatusInternalServerError)
	c := e.config(bad.URL)
	c.EjectAfter = 1
	c.EjectFor = time.Minute
	lb := newLB(t, c)

	checkResponse(t, get(t, lb, "/"), http.StatusInternalServerError, "bad")
	if rec := get(t, lb, "/"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status with the only backend ejected: %d, want 503", rec.Code)
	}
	if got := bad.hits.Load(); got != 1 {
		t.Errorf("the ejected backend got %d requests, want 1", got)
	}
}

func TestEjectDisabled(t *testing.T) {
	// EjectAfter 0 turns passive ejection off.
	e := newEnv(t)
	bad := e.upstream("bad")
	bad.status.Store(http.StatusInternalServerError)
	lb := newLB(t, e.config(bad.URL))
	for range 10 {
		checkResponse(t, get(t, lb, "/"), http.StatusInternalServerError, "bad")
	}
	if status(t, lb, bad.URL).Ejected {
		t.Errorf("ejected with EjectAfter 0, want passive ejection off")
	}
}

func TestEjectIgnoresClientCancel(t *testing.T) {
	// A client that gives up says nothing about the backend. If it counted,
	// a burst of impatient clients would eject healthy backends.
	e := newEnv(t)
	slow := e.upstream("slow")
	slow.hold.Store(true)
	c := e.config(slow.URL)
	c.EjectAfter = 1
	c.EjectFor = time.Minute
	lb := newLB(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := start(lb, httptest.NewRequest(http.MethodGet, "http://lb.example/report", nil).WithContext(ctx))
	e.waitArrival(t, "slow")
	cancel()
	await(t, ch)
	waitInFlight(t, lb, slow.URL, 0)

	if status(t, lb, slow.URL).Ejected {
		t.Fatalf("backend ejected because a client cancelled its request")
	}
	slow.hold.Store(false)
	checkResponse(t, get(t, lb, "/"), http.StatusOK, "slow")
}

func TestEjectWithRealClock(t *testing.T) {
	// Config.Now nil means time.Now.
	e := newEnv(t)
	a := e.upstream("a")
	a.status.Store(http.StatusInternalServerError)
	c := e.config(a.URL)
	c.Now = nil
	c.EjectAfter = 1
	c.EjectFor = time.Hour
	lb := newLB(t, c)
	get(t, lb, "/")
	if !status(t, lb, a.URL).Ejected {
		t.Errorf("backend not ejected after a 500 with EjectAfter 1, EjectFor 1h and Now nil (the real clock)")
	}
}
