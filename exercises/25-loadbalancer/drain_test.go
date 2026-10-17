package loadbalancer_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	loadbalancer "github.com/yornik/go-practice/exercises/25-loadbalancer"
)

// startDrain calls lb.Drain in a new goroutine. The channel receives its
// result.
func startDrain(lb *loadbalancer.LB, ctx context.Context, backend string) <-chan error {
	ch := make(chan error, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				ch <- errors.New("Drain panicked")
			}
		}()
		ch <- lb.Drain(ctx, backend)
	}()
	return ch
}

// awaitDrain waits for the result of startDrain.
func awaitDrain(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		t.Fatalf("Drain did not return within %v", timeout)
		return nil
	}
}

// notReturned fails the test if Drain returns within the window.
func notReturned(t *testing.T, ch <-chan error, why string) {
	t.Helper()
	select {
	case err := <-ch:
		t.Fatalf("Drain returned (error %v) while %s", err, why)
	case <-time.After(window):
	}
}

func TestDrainUnknownBackend(t *testing.T) {
	e := newEnv(t)
	lb := newLB(t, e.config("http://10.0.0.7:8080"))
	err := awaitDrain(t, startDrain(lb, context.Background(), "http://10.0.0.99:8080"))
	if !errors.Is(err, loadbalancer.ErrUnknownBackend) {
		t.Errorf("Drain of a URL that is not in Config.Backends: error = %v, want ErrUnknownBackend", err)
	}
}

func TestDrainIdleBackend(t *testing.T) {
	e := newEnv(t)
	a, b := e.upstream("a"), e.upstream("b")
	lb := newLB(t, e.config(a.URL, b.URL))

	// Nothing in flight: Drain returns at once.
	if err := awaitDrain(t, startDrain(lb, context.Background(), a.URL)); err != nil {
		t.Fatalf("Drain(a) = %v, want nil", err)
	}
	if s := status(t, lb, a.URL); !s.Draining || !s.Healthy {
		t.Errorf("Status of a after Drain = %+v, want Draining and still Healthy", s)
	}
	for range 3 {
		checkResponse(t, get(t, lb, "/"), http.StatusOK, "b")
	}

	if err := awaitDrain(t, startDrain(lb, context.Background(), b.URL)); err != nil {
		t.Fatalf("Drain(b) = %v, want nil", err)
	}
	if rec := get(t, lb, "/"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status with every backend draining: %d, want 503", rec.Code)
	}
	if got := a.hits.Load(); got != 0 {
		t.Errorf("drained backend a got %d requests, want 0", got)
	}
}

func TestDrainWaitsForInFlightRequests(t *testing.T) {
	e := newEnv(t)
	a, b := e.upstream("a"), e.upstream("b")
	a.hold.Store(true)
	c := e.config(a.URL, b.URL)
	c.Strategy = loadbalancer.RoundRobin()
	lb := newLB(t, c)

	// Round robin: a (held), b (answers at once), a (held).
	r1 := startGet(lb, "/upload/1")
	e.waitArrival(t, "a")
	checkResponse(t, get(t, lb, "/"), http.StatusOK, "b")
	r3 := startGet(lb, "/upload/3")
	e.waitArrival(t, "a")

	drained := startDrain(lb, context.Background(), a.URL)
	notReturned(t, drained, "2 requests to the backend are in flight")

	// New requests avoid the draining backend.
	waitFor(t, func() string {
		if !status(t, lb, a.URL).Draining {
			return "Status of a does not show Draining while Drain(a) runs"
		}
		return ""
	})
	for range 3 {
		checkResponse(t, get(t, lb, "/"), http.StatusOK, "b")
	}

	a.releaseOne(t)
	notReturned(t, drained, "1 request to the backend is still in flight")

	a.releaseOne(t)
	if err := awaitDrain(t, drained); err != nil {
		t.Fatalf("Drain = %v, want nil", err)
	}
	// The requests that were in flight finished normally.
	checkResponse(t, await(t, r1), http.StatusOK, "a")
	checkResponse(t, await(t, r3), http.StatusOK, "a")
	if got := status(t, lb, a.URL).InFlight; got != 0 {
		t.Errorf("InFlight of a after Drain returned = %d, want 0", got)
	}
}

func TestDrainContextEnds(t *testing.T) {
	e := newEnv(t)
	a, b := e.upstream("a"), e.upstream("b")
	a.hold.Store(true)
	lb := newLB(t, e.config(a.URL, b.URL))

	r1 := startGet(lb, "/long-poll")
	e.waitArrival(t, "a")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	drained := startDrain(lb, ctx, a.URL)
	notReturned(t, drained, "a request is in flight and ctx is not done")
	cancel()
	if err := awaitDrain(t, drained); !errors.Is(err, context.Canceled) {
		t.Fatalf("Drain after its ctx was cancelled = %v, want an error that matches context.Canceled", err)
	}

	// The backend stays draining, and the request in flight is not cut off.
	if s := status(t, lb, a.URL); !s.Draining || s.InFlight != 1 {
		t.Errorf("Status of a after Drain gave up = %+v, want Draining with 1 in flight", s)
	}
	checkResponse(t, get(t, lb, "/"), http.StatusOK, "b")
	a.releaseOne(t)
	checkResponse(t, await(t, r1), http.StatusOK, "a")
	waitInFlight(t, lb, a.URL, 0)
}

func TestDrainTwiceAtOnce(t *testing.T) {
	// Two Drains of the same backend wait at the same time, for example a
	// preStop hook and an operator. Both return when the last request ends.
	e := newEnv(t)
	a, b := e.upstream("a"), e.upstream("b")
	a.hold.Store(true)
	lb := newLB(t, e.config(a.URL, b.URL))

	r1 := startGet(lb, "/upload")
	e.waitArrival(t, "a")
	first := startDrain(lb, context.Background(), a.URL)
	second := startDrain(lb, context.Background(), a.URL)
	notReturned(t, first, "a request is in flight")

	a.releaseOne(t)
	checkResponse(t, await(t, r1), http.StatusOK, "a")
	for i, ch := range []<-chan error{first, second} {
		select {
		case err := <-ch:
			if err != nil {
				t.Errorf("Drain %d of 2 = %v, want nil", i+1, err)
			}
		case <-time.After(timeout):
			t.Fatalf("Drain %d of 2 did not return within %v after the last request ended: every waiting Drain must see in-flight 0", i+1, timeout)
		}
	}
}

// gatedStrategy wraps a Strategy. Its first Pick tells the test that it has
// started (entered is closed) and then waits until the test closes proceed.
// Later Picks go straight to the wrapped Strategy.
type gatedStrategy struct {
	inner   loadbalancer.Strategy
	once    sync.Once
	entered chan struct{}
	proceed chan struct{}
}

func (g *gatedStrategy) Pick(bs []*loadbalancer.Backend) *loadbalancer.Backend {
	g.once.Do(func() {
		close(g.entered)
		<-g.proceed
	})
	return g.inner.Pick(bs)
}

func TestDrainWhilePicking(t *testing.T) {
	// Check-then-act. A request has found backend a eligible and is inside
	// Pick when Drain(a) is called. Two outcomes are correct: the request counts as in
	// flight on a and Drain waits for it, or the request goes to another
	// backend. Wrong: Drain sees in-flight 0 and returns nil, and then the
	// request reaches a. The operator would restart a with a request on it.
	e := newEnv(t)
	a, b := e.upstream("a"), e.upstream("b")
	a.hold.Store(true)
	b.hold.Store(true)
	g := &gatedStrategy{
		inner:   loadbalancer.LeastConnections(), // picks a, the first eligible one
		entered: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	var letPickGo sync.Once
	t.Cleanup(func() { letPickGo.Do(func() { close(g.proceed) }) })
	c := e.config(a.URL, b.URL)
	c.Strategy = g
	lb := newLB(t, c)

	r1 := startGet(lb, "/upload")
	select {
	case <-g.entered:
	case <-time.After(timeout):
		t.Fatalf("ServeHTTP did not call Pick within %v", timeout)
	}
	drained := startDrain(lb, context.Background(), a.URL)
	time.Sleep(window) // let Drain get as far as it can while Pick runs
	letPickGo.Do(func() { close(g.proceed) })

	var got string
	select {
	case got = <-e.arrivals:
	case <-time.After(timeout):
		t.Fatalf("the request reached no backend within %v", timeout)
	}
	if got == "a" {
		select {
		case err := <-drained:
			t.Fatalf("Drain(a) returned %v, and then the request reached a: the pick and the in-flight count must be one step for Drain", err)
		default:
		}
		a.releaseOne(t)
	} else {
		b.releaseOne(t)
	}
	checkResponse(t, await(t, r1), http.StatusOK, got)
	if err := awaitDrain(t, drained); err != nil {
		t.Errorf("Drain = %v, want nil", err)
	}
}

func TestDrainWaitsForResponseBody(t *testing.T) {
	// A download: the backend sends the headers and the first part of the
	// body, then the rest later. The request is in flight until the whole
	// body has been copied to the client, so Drain must wait for that.
	//
	// The body has no Content-Length, so the LB streams it: the client gets
	// the headers and part 1 while the backend still holds part 2.
	// ReverseProxy flushes such a body by itself; a wrapper around the
	// ResponseWriter without Flush would hold everything back.
	e := newEnv(t)
	rest := make(chan struct{})
	srv := e.server(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "part 1\n")
		_ = http.NewResponseController(w).Flush()
		select {
		case <-rest:
		case <-r.Context().Done():
		case <-e.quit:
		}
		_, _ = io.WriteString(w, "part 2\n")
	}))
	lb := newLB(t, e.config(srv.URL))
	front := e.server(lb)

	var res *http.Response
	within(t, "GET through the LB (the headers and part 1 must reach the client while the backend holds part 2: is the body streamed?)", func() {
		var err error
		res, err = front.Client().Get(front.URL + "/export.csv")
		if err != nil {
			t.Errorf("GET through the LB: %v", err)
		}
	})
	if res == nil {
		t.FailNow()
	}
	defer func() { _ = res.Body.Close() }()

	// The client has the headers now. The body is still coming.
	if got := status(t, lb, srv.URL).InFlight; got != 1 {
		t.Fatalf("InFlight = %d while the response body is still being copied, want 1", got)
	}
	drained := startDrain(lb, context.Background(), srv.URL)
	notReturned(t, drained, "the response body is still being copied")

	close(rest)
	var body []byte
	within(t, "reading the response body", func() { body, _ = io.ReadAll(res.Body) })
	if string(body) != "part 1\npart 2\n" {
		t.Errorf("body = %q, want %q", body, "part 1\npart 2\n")
	}
	if err := awaitDrain(t, drained); err != nil {
		t.Errorf("Drain = %v, want nil", err)
	}
}
