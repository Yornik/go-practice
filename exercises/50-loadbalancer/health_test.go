package loadbalancer_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	loadbalancer "github.com/yornik/go-practice/exercises/50-loadbalancer"
)

// checkHealth runs one round of lb.CheckHealth with a background context.
func checkHealth(t *testing.T, lb *loadbalancer.LB) {
	t.Helper()
	within(t, "CheckHealth", func() { lb.CheckHealth(context.Background()) })
}

func TestHealthCheckThresholds(t *testing.T) {
	e := newEnv(t)
	a := e.upstream("a")
	c := e.config(a.URL)
	c.UnhealthyAfter = 2
	c.HealthyAfter = 3
	lb := newLB(t, c)

	steps := []struct {
		check       int // status of the health endpoint in this round
		wantHealthy bool
	}{
		{200, true},  // backends start healthy
		{503, true},  // 1 failure
		{200, true},  // a success resets the failures
		{503, true},  // 1 failure
		{503, false}, // 2 in a row: unhealthy
		{200, false}, // 1 success
		{200, false}, // 2 successes
		{503, false}, // a failure resets the successes
		{200, false}, // 1
		{200, false}, // 2
		{200, true},  // 3 in a row: healthy again
	}
	for i, s := range steps {
		a.health.Store(int32(s.check))
		checkHealth(t, lb)
		if got := status(t, lb, a.URL).Healthy; got != s.wantHealthy {
			t.Fatalf("round %d (check status %d): Healthy = %v, want %v (UnhealthyAfter 2, HealthyAfter 3)", i+1, s.check, got, s.wantHealthy)
		}
	}
}

func TestHealthCheckStatusCodes(t *testing.T) {
	// Any 2xx passes; every other status fails.
	for _, tc := range []struct {
		code int
		pass bool
	}{
		{200, true}, {204, true}, {404, false}, {429, false}, {500, false}, {503, false},
	} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			e := newEnv(t)
			a := e.upstream("a")
			a.health.Store(int32(tc.code))
			lb := newLB(t, e.config(a.URL)) // UnhealthyAfter 0 means 1
			checkHealth(t, lb)
			if got := status(t, lb, a.URL).Healthy; got != tc.pass {
				t.Errorf("after a check that returned %d: Healthy = %v, want %v", tc.code, got, tc.pass)
			}
		})
	}
}

func TestHealthCheckRequest(t *testing.T) {
	// One GET on HealthPath per backend per round, sent with Config.Transport.
	e := newEnv(t)
	var mu sync.Mutex
	var seen []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
	})
	a, b := e.server(h), e.server(h)
	lb := newLB(t, e.config(a.URL, b.URL))
	checkHealth(t, lb)

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[0] != "GET "+healthPath || seen[1] != "GET "+healthPath {
		t.Errorf("the backends received %q, want 2 times %q", seen, "GET "+healthPath)
	}
	for _, u := range []string{a.URL, b.URL} {
		if n := e.checks(u); n != 1 {
			t.Errorf("%d health checks to %s went through Config.Transport, want 1", n, u)
		}
	}
}

func TestHealthCheckUnhealthyGetsNoTraffic(t *testing.T) {
	e := newEnv(t)
	a, b := e.upstream("a"), e.upstream("b")
	lb := newLB(t, e.config(a.URL, b.URL))

	a.health.Store(http.StatusServiceUnavailable)
	checkHealth(t, lb)
	for range 3 {
		checkResponse(t, get(t, lb, "/"), http.StatusOK, "b")
	}

	b.health.Store(http.StatusServiceUnavailable)
	checkHealth(t, lb)
	if rec := get(t, lb, "/"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status with every backend unhealthy: %d, want 503", rec.Code)
	}

	a.health.Store(http.StatusOK)
	checkHealth(t, lb)
	checkResponse(t, get(t, lb, "/"), http.StatusOK, "a")
	if got := a.hits.Load(); got != 1 {
		t.Errorf("backend a got %d requests, want 1 (none while it was unhealthy)", got)
	}
}

func TestHealthCheckTimeout(t *testing.T) {
	// A health endpoint that hangs fails after HealthTimeout. Without the
	// timeout, CheckHealth would wait for it until the test ends.
	e := newEnv(t)
	hanging := e.server(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-e.quit:
		}
	}))
	c := e.config(hanging.URL)
	c.HealthTimeout = 50 * time.Millisecond
	lb := newLB(t, c)

	checkHealth(t, lb)
	if status(t, lb, hanging.URL).Healthy {
		t.Errorf("a backend whose health check hangs is still healthy after a round with HealthTimeout 50ms")
	}
}

func TestHealthCheckConcurrent(t *testing.T) {
	// Each health endpoint waits until all 3 backends have received their
	// check, for at most 2s. Checks that run one after the other make the
	// first ones wait 2s and fail. (In production the reason is the same:
	// with 50 backends and one that hangs, one slow check must not delay
	// the others.)
	e := newEnv(t)
	var arrived atomic.Int32
	allIn := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if arrived.Add(1) == 3 {
			close(allIn)
		}
		select {
		case <-allIn:
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusServiceUnavailable)
		case <-e.quit:
		}
	})
	a, b, c := e.server(h), e.server(h), e.server(h)
	cfg := e.config(a.URL, b.URL, c.URL)
	cfg.HealthTimeout = 10 * time.Second
	lb := newLB(t, cfg)

	checkHealth(t, lb)
	for i, u := range []string{a.URL, b.URL, c.URL} {
		if !status(t, lb, u).Healthy {
			t.Errorf("backend %d is unhealthy: CheckHealth must check all backends at the same time", i)
		}
	}
}

func TestHealthCheckCancelledContext(t *testing.T) {
	// A round whose ctx is done changes nothing: the LB is shutting down,
	// the backends did not fail or recover.
	e := newEnv(t)
	up, down := e.upstream("up"), e.upstream("down")
	lb := newLB(t, e.config(up.URL, down.URL))
	down.health.Store(http.StatusServiceUnavailable)
	checkHealth(t, lb) // down is now unhealthy

	up.health.Store(http.StatusServiceUnavailable)
	down.health.Store(http.StatusOK)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	within(t, "CheckHealth with a cancelled ctx", func() { lb.CheckHealth(ctx) })

	if !status(t, lb, up.URL).Healthy {
		t.Errorf("a healthy backend became unhealthy in a round with a cancelled ctx")
	}
	if status(t, lb, down.URL).Healthy {
		t.Errorf("an unhealthy backend became healthy in a round with a cancelled ctx")
	}
}

// runHealthChecks starts lb.RunHealthChecks and returns a function that
// cancels it and waits until it has returned.
func runHealthChecks(t *testing.T, lb *loadbalancer.LB) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		lb.RunHealthChecks(ctx)
	}()
	stopped := false
	stop = func() {
		t.Helper()
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case v := <-done:
			if v != nil {
				t.Fatalf("RunHealthChecks panicked: %v", v)
			}
		case <-time.After(timeout):
			t.Fatalf("RunHealthChecks did not return within %v after its ctx was cancelled", timeout)
		}
	}
	t.Cleanup(stop)
	return stop
}

func TestHealthRunFirstRoundAtOnce(t *testing.T) {
	// The first round runs at once, not after the first HealthInterval. And
	// a cancelled ctx stops the wait for the next round.
	e := newEnv(t)
	a := e.upstream("a")
	c := e.config(a.URL)
	c.HealthInterval = time.Hour
	lb := newLB(t, c)

	stop := runHealthChecks(t, lb)
	waitFor(t, func() string {
		if e.checks(a.URL) == 0 {
			return "RunHealthChecks has sent no health check (HealthInterval is 1h: the first round must run at once)"
		}
		return ""
	})
	stop()
}

// roundTripFunc turns a function into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHealthRunWaitsForRunningRound(t *testing.T) {
	// RunHealthChecks owns the goroutines of its rounds, so it returns only
	// after the running round has finished. Here a health check is slow to
	// give up: its transport ignores the cancel until the test releases it.
	e := newEnv(t)
	a := e.upstream("a")
	checking := make(chan struct{}, 1)
	release := make(chan struct{})
	c := e.config(a.URL)
	c.HealthInterval = time.Hour
	c.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == healthPath {
			select {
			case checking <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-e.quit:
			}
		}
		return e.transport.RoundTrip(r)
	})
	lb := newLB(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	var panicked any
	go func() {
		defer close(done)
		defer func() { panicked = recover() }()
		lb.RunHealthChecks(ctx)
	}()
	select {
	case <-checking:
	case <-done:
		close(release)
		t.Fatalf("RunHealthChecks returned before its first health check ended (panic: %v)", panicked)
	case <-time.After(timeout):
		close(release)
		t.Fatalf("RunHealthChecks sent no health check within %v", timeout)
	}
	cancel()
	select {
	case <-done:
		close(release)
		t.Fatalf("RunHealthChecks returned while a check of its round was still running (panic: %v): wait for the round to finish", panicked)
	case <-time.After(window):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("RunHealthChecks did not return within %v after its ctx was cancelled and its last check ended", timeout)
	}
}

func TestHealthRunRepeats(t *testing.T) {
	e := newEnv(t)
	a := e.upstream("a")
	c := e.config(a.URL)
	c.HealthInterval = 10 * time.Millisecond
	c.UnhealthyAfter = 3
	c.HealthyAfter = 2
	lb := newLB(t, c)
	stop := runHealthChecks(t, lb)

	a.health.Store(http.StatusServiceUnavailable)
	waitFor(t, func() string {
		if status(t, lb, a.URL).Healthy {
			return "backend still healthy while its checks fail (HealthInterval 10ms, UnhealthyAfter 3)"
		}
		return ""
	})
	a.health.Store(http.StatusOK)
	waitFor(t, func() string {
		if !status(t, lb, a.URL).Healthy {
			return "backend still unhealthy while its checks pass (HealthInterval 10ms, HealthyAfter 2)"
		}
		return ""
	})

	stop()
	n := e.checks(a.URL)
	time.Sleep(window)
	if after := e.checks(a.URL); after != n {
		t.Errorf("%d health checks were sent after RunHealthChecks returned, want 0", after-n)
	}
}
