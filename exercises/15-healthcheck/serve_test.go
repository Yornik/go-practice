package healthcheck_test

// Step 6: Serve and the shutdown sequence. These tests run a real HTTP
// server on a free port of 127.0.0.1.
//
//	go test -race -run TestServe -v ./exercises/15-healthcheck/

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	healthcheck "github.com/yornik/go-practice/exercises/15-healthcheck"
)

// listen opens a TCP listener on a free port of the loopback interface.
func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

// newClient returns an HTTP client without keep-alive: every request opens a
// new connection, like a new client during a rollout.
func newClient(t *testing.T) *http.Client {
	tr := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr}
}

type serveResult struct {
	err      error
	panicked any
}

// startServe calls healthcheck.Serve in a new goroutine. The returned channel
// receives the result when Serve returns.
func startServe(ctx context.Context, srv *http.Server, ln net.Listener, h *healthcheck.Health, o healthcheck.ServeOptions) <-chan serveResult {
	ch := make(chan serveResult, 1)
	go func() {
		var res serveResult
		defer func() {
			res.panicked = recover()
			ch <- res
		}()
		res.err = healthcheck.Serve(ctx, srv, ln, h, o)
	}()
	return ch
}

// awaitServe waits until Serve returns and returns its error.
func awaitServe(t *testing.T, what string, ch <-chan serveResult) error {
	t.Helper()
	select {
	case res := <-ch:
		if res.panicked != nil {
			t.Fatalf("Serve panicked: %v", res.panicked)
		}
		return res.err
	case <-time.After(timeout):
		t.Fatalf("Serve did not return within %v %s", timeout, what)
		return nil
	}
}

// failIfReturned fails the test if Serve has already returned.
func failIfReturned(t *testing.T, ch <-chan serveResult, when string) {
	t.Helper()
	select {
	case res := <-ch:
		t.Fatalf("Serve returned (err = %v, panic = %v) %s; it must keep serving", res.err, res.panicked, when)
	default:
	}
}

type getResult struct {
	code int
	body string
	err  error
}

// startGet sends GET url in a new goroutine.
func startGet(c *http.Client, url string) <-chan getResult {
	ch := make(chan getResult, 1)
	go func() {
		var res getResult
		resp, err := c.Get(url)
		if err != nil {
			res.err = err
		} else {
			b, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			res.code, res.body, res.err = resp.StatusCode, string(b), err
		}
		ch <- res
	}()
	return ch
}

// awaitGet waits for the result of startGet.
func awaitGet(t *testing.T, what string, ch <-chan getResult) getResult {
	t.Helper()
	select {
	case res := <-ch:
		return res
	case <-time.After(timeout):
		t.Fatalf("%s: no answer within %v", what, timeout)
		return getResult{}
	}
}

// get sends GET url and waits for the answer. It fails the test if the
// request fails or if Serve returns first.
func get(t *testing.T, c *http.Client, url, when string, serving <-chan serveResult) getResult {
	t.Helper()
	select {
	case res := <-startGet(c, url):
		if res.err != nil {
			t.Fatalf("GET %s %s: %v; the server must still accept and serve requests", url, when, res.err)
		}
		return res
	case res := <-serving:
		t.Fatalf("Serve returned (err = %v, panic = %v) %s, before it answered GET %s", res.err, res.panicked, when, url)
	case <-time.After(timeout):
		t.Fatalf("GET %s %s: no answer within %v", url, when, timeout)
	}
	return getResult{}
}

// fakeSleep records the calls of ServeOptions.Sleep and blocks them until the
// test calls wake.
type fakeSleep struct {
	calls chan time.Duration
	g     *gate
}

func newFakeSleep(t *testing.T) *fakeSleep {
	return &fakeSleep{calls: make(chan time.Duration, 10), g: newGate(t)}
}

func (s *fakeSleep) Sleep(d time.Duration) {
	s.calls <- d
	<-s.g.ch
}

func (s *fakeSleep) wake() { s.g.open() }

// TestServeGracefulShutdown walks through a rollout: the pod gets SIGTERM
// (ctx is cancelled) while a request is in flight.
func TestServeGracefulShutdown(t *testing.T) {
	const drainDelay = 7 * time.Second // never really waited: Sleep is a fake

	h := newHealth(t, healthcheck.Options{}, check("postgres", true, nil, nil))
	workStarted := newEvent()
	workGate := newGate(t)
	mux := http.NewServeMux()
	hh := h.Handler()
	mux.Handle("/livez", hh) // the health routes next to the app's own routes
	mux.Handle("/readyz", hh)
	mux.HandleFunc("GET /work", func(w http.ResponseWriter, r *http.Request) {
		workStarted.fire()
		<-workGate.ch // a slow request, still running when shutdown starts
		_, _ = io.WriteString(w, "work done")
	})
	mux.HandleFunc("GET /fast", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "fast")
	})
	srv := &http.Server{Handler: mux}
	t.Cleanup(func() { _ = srv.Close() })
	shutdownCalled := newEvent()
	srv.RegisterOnShutdown(shutdownCalled.fire) // srv.Shutdown calls this; srv.Close does not

	ln := listen(t)
	base := "http://" + ln.Addr().String()
	c := newClient(t)
	sleep := newFakeSleep(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serving := startServe(ctx, srv, ln, h, healthcheck.ServeOptions{
		DrainDelay: drainDelay, ShutdownTimeout: time.Minute, Sleep: sleep.Sleep,
	})

	// 1. Normal operation.
	if res := get(t, c, base+"/readyz", "before ctx is done", serving); res.code != http.StatusOK {
		t.Fatalf("GET /readyz before ctx is done = %d, want 200\nbody: %s", res.code, res.body)
	}
	inflight := startGet(c, base+"/work")
	waitClosed(t, workStarted.done(), "the /work request to reach its handler")

	// 2. SIGTERM.
	cancel()
	select {
	case d := <-sleep.calls:
		if d != drainDelay {
			t.Errorf("Sleep(%v), want Sleep(DrainDelay) = Sleep(%v)", d, drainDelay)
		}
	case res := <-serving:
		t.Fatalf("Serve returned (err = %v, panic = %v) without calling Sleep(DrainDelay)", res.err, res.panicked)
	case <-time.After(timeout):
		t.Fatalf("Sleep was not called within %v after ctx was cancelled", timeout)
	}

	// 3. During the drain delay: readiness fails, everything else still works.
	select {
	case <-shutdownCalled.done():
		t.Fatalf("srv.Shutdown was called before the drain delay was over")
	default:
	}
	during := "during the drain delay"
	res := get(t, c, base+"/readyz", during, serving)
	var rep report
	if err := json.Unmarshal([]byte(res.body), &rep); err != nil || res.code != http.StatusServiceUnavailable || rep.Status != "draining" {
		t.Errorf("GET /readyz %s = %d %s, want 503 with status \"draining\": SetDraining comes before the drain delay", during, res.code, res.body)
	}
	if res := get(t, c, base+"/livez", during, serving); res.code != http.StatusOK {
		t.Errorf("GET /livez %s = %d, want 200", during, res.code)
	}
	if res := get(t, c, base+"/fast", during, serving); res.code != http.StatusOK || res.body != "fast" {
		t.Errorf("GET /fast %s = %d %q, want 200 \"fast\": new requests are still served", during, res.code, res.body)
	}
	failIfReturned(t, serving, during)

	// 4. The drain delay is over: Shutdown, which waits for the in-flight request.
	sleep.wake()
	waitClosed(t, shutdownCalled.done(), "srv.Shutdown to be called after the drain delay")
	select {
	case res := <-serving:
		t.Fatalf("Serve returned (err = %v, panic = %v) while a request was still in flight", res.err, res.panicked)
	case <-time.After(window):
	}

	// 5. The in-flight request finishes and gets its whole answer.
	workGate.open()
	work := awaitGet(t, "the in-flight GET /work", inflight)
	if work.err != nil || work.code != http.StatusOK || work.body != "work done" {
		t.Errorf("in-flight GET /work = %d %q, error %v; want 200 \"work done\": in-flight requests must complete",
			work.code, work.body, work.err)
	}
	if err := awaitServe(t, "after the last request finished", serving); err != nil {
		t.Errorf("Serve = %v, want nil after a clean shutdown", err)
	}
}

// TestServeShutdownTimeout: a request hangs (a stuck download, a client that
// never reads). After ShutdownTimeout, Serve cuts it and reports the timeout.
func TestServeShutdownTimeout(t *testing.T) {
	h := newHealth(t, healthcheck.Options{})
	hangStarted, hangCut := newEvent(), newEvent()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hang", func(w http.ResponseWriter, r *http.Request) {
		hangStarted.fire()
		<-r.Context().Done() // cancelled when the server closes the connection
		hangCut.fire()
	})
	srv := &http.Server{Handler: mux}
	t.Cleanup(func() { _ = srv.Close() })

	ln := listen(t)
	c := newClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const drainDelay = 100 * time.Millisecond
	// Sleep is nil: Serve must use time.Sleep for the drain delay.
	serving := startServe(ctx, srv, ln, h, healthcheck.ServeOptions{
		DrainDelay: drainDelay, ShutdownTimeout: 50 * time.Millisecond,
	})

	hang := startGet(c, "http://"+ln.Addr().String()+"/hang")
	waitClosed(t, hangStarted.done(), "the /hang request to reach its handler")
	cancelled := time.Now()
	cancel()

	err := awaitServe(t, "after ShutdownTimeout (50ms) with a request that never finishes", serving)
	if took := time.Since(cancelled); took < drainDelay {
		t.Errorf("Serve returned %v after ctx was cancelled, want at least DrainDelay (%v): with Sleep nil, Serve must use time.Sleep", took, drainDelay)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Serve = %v, want an error that matches context.DeadlineExceeded", err)
	}
	waitClosed(t, hangCut.done(), "the hanging request to be cut (its r.Context() cancelled): after ShutdownTimeout, Serve must close the remaining connections with srv.Close")
	if res := awaitGet(t, "the hanging GET /hang", hang); res.err == nil {
		t.Errorf("the hanging GET /hang got %d %q, want a connection error", res.code, res.body)
	}
}

// TestServeShutdownTimeoutStartsAtShutdown: ShutdownTimeout is the budget of
// srv.Shutdown alone and starts when Shutdown is called. Here the drain delay
// (700ms) is longer than ShutdownTimeout (600ms), and the in-flight request
// finishes about 100ms after Shutdown started: it must still complete.
func TestServeShutdownTimeoutStartsAtShutdown(t *testing.T) {
	h := newHealth(t, healthcheck.Options{})
	workStarted := newEvent()
	workGate := newGate(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /work", func(w http.ResponseWriter, r *http.Request) {
		workStarted.fire()
		<-workGate.ch
		_, _ = io.WriteString(w, "work done")
	})
	srv := &http.Server{Handler: mux}
	t.Cleanup(func() { _ = srv.Close() })
	shutdownCalled := newEvent()
	srv.RegisterOnShutdown(shutdownCalled.fire)

	ln := listen(t)
	c := newClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serving := startServe(ctx, srv, ln, h, healthcheck.ServeOptions{
		DrainDelay: 700 * time.Millisecond, ShutdownTimeout: 600 * time.Millisecond, // Sleep nil: a real drain delay
	})

	inflight := startGet(c, "http://"+ln.Addr().String()+"/work")
	waitClosed(t, workStarted.done(), "the /work request to reach its handler")
	cancel()
	waitClosed(t, shutdownCalled.done(), "srv.Shutdown to be called after the drain delay")
	time.Sleep(window) // the request needs a bit longer, well within ShutdownTimeout
	workGate.open()

	work := awaitGet(t, "the in-flight GET /work", inflight)
	if work.err != nil || work.code != http.StatusOK || work.body != "work done" {
		t.Errorf("in-flight GET /work = %d %q, error %v; want 200 \"work done\": ShutdownTimeout counts from the call to srv.Shutdown, the drain delay must not use it up",
			work.code, work.body, work.err)
	}
	if err := awaitServe(t, "after the last request finished", serving); err != nil {
		t.Errorf("Serve = %v, want nil: the request finished %v after Shutdown started, within ShutdownTimeout (600ms) counted from the call to srv.Shutdown", err, window)
	}
}

// TestServeListenerError: srv.Serve fails at once (here: the listener is
// closed). Serve must report it instead of waiting for a ctx that may never
// end.
func TestServeListenerError(t *testing.T) {
	h := newHealth(t, healthcheck.Options{})
	ln := listen(t)
	_ = ln.Close()
	srv := &http.Server{Handler: http.NotFoundHandler()}
	t.Cleanup(func() { _ = srv.Close() })
	sleep := newFakeSleep(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel) // never cancelled during the test

	serving := startServe(ctx, srv, ln, h, healthcheck.ServeOptions{
		DrainDelay: time.Second, ShutdownTimeout: time.Second, Sleep: sleep.Sleep,
	})
	err := awaitServe(t, "when srv.Serve failed (closed listener) and ctx is not done", serving)
	if !errors.Is(err, net.ErrClosed) {
		t.Errorf("Serve = %v, want the error of srv.Serve, which matches net.ErrClosed", err)
	}
	select {
	case d := <-sleep.calls:
		t.Errorf("Serve called Sleep(%v), want no shutdown sequence when the server never ran", d)
	default:
	}
}
