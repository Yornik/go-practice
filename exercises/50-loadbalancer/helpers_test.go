package loadbalancer_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	loadbalancer "github.com/yornik/go-practice/exercises/50-loadbalancer"
)

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// window is how long a test watches for something that must NOT happen, such
// as Drain returning too early. A correct solution never does it, so the
// window only decides how likely a wrong solution is to get caught.
const window = 100 * time.Millisecond

// healthPath is the HealthPath of every LB in these tests.
const healthPath = "/ready"

// ----- a fake clock -----

// fakeClock is the Config.Now of the tests. Time only moves when the test
// calls Advance.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// ----- the test environment -----

// env is one test's little data centre: fake backends ("upstreams"), a
// transport that counts what the LB sends, and a fake clock.
type env struct {
	t         *testing.T
	clock     *fakeClock
	transport *countingTransport
	// arrivals receives the name of an upstream every time a held request
	// reaches it (see upstream.hold).
	arrivals chan string
	// quit is closed when the test ends. It lets every held request finish,
	// so that the servers can shut down even after a failed test.
	quit      chan struct{}
	stopOnce  sync.Once
	dead      sync.Map // addresses that refuse connections
	deadCount atomic.Int32
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{
		t:        t,
		clock:    &fakeClock{now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)},
		arrivals: make(chan string, 100),
		quit:     make(chan struct{}),
	}
	tr := &http.Transport{DialContext: e.dial, MaxIdleConnsPerHost: 64}
	e.transport = &countingTransport{base: tr, attempts: map[string]int{}, checks: map[string]int{}}
	t.Cleanup(func() {
		e.stop()
		tr.CloseIdleConnections()
	})
	return e
}

func (e *env) stop() { e.stopOnce.Do(func() { close(e.quit) }) }

// dial is the transport's dialer. It refuses addresses marked dead.
func (e *env) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if _, dead := e.dead.Load(addr); dead {
		return nil, &net.OpError{Op: "dial", Net: network, Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	}
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

// server starts an HTTP server for h and returns its URL. The server is
// closed when the test ends.
func (e *env) server(h http.Handler) *httptest.Server {
	e.t.Helper()
	srv := httptest.NewServer(h)
	e.t.Cleanup(func() {
		e.stop() // release held requests first: Close waits for them
		srv.Close()
	})
	return srv
}

// deadURL returns the URL of a backend that is down: every connection to it
// fails with "connection refused". The transport's dialer refuses it without
// touching the network. (A closed httptest server would do the same, but the
// next server the test starts can get the same port.)
func (e *env) deadURL() string {
	addr := fmt.Sprintf("dead-%d.invalid:8080", e.deadCount.Add(1))
	e.dead.Store(addr, true)
	return "http://" + addr
}

// countingTransport counts the requests the LB sends to each backend: proxied
// requests and health checks separately.
type countingTransport struct {
	base     http.RoundTripper
	mu       sync.Mutex
	attempts map[string]int // host -> proxied requests
	checks   map[string]int // host -> health checks
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	if r.URL.Path == healthPath {
		c.checks[r.URL.Host]++
	} else {
		c.attempts[r.URL.Host]++
	}
	c.mu.Unlock()
	return c.base.RoundTrip(r)
}

func (e *env) count(m map[string]int, rawURL string) int {
	e.t.Helper()
	host := rawURL[len("http://"):]
	e.transport.mu.Lock()
	defer e.transport.mu.Unlock()
	return m[host]
}

// attempts returns how many proxied requests the LB sent to the backend.
func (e *env) attempts(rawURL string) int { return e.count(e.transport.attempts, rawURL) }

// checks returns how many health checks the LB sent to the backend.
func (e *env) checks(rawURL string) int { return e.count(e.transport.checks, rawURL) }

// ----- fake backends -----

// upstream is a fake backend. It answers health checks with its health
// status and every other request with its status and its name as the body,
// so a test can see which backend answered.
type upstream struct {
	e      *env
	name   string
	URL    string
	status atomic.Int32 // status of proxied requests, default 200
	health atomic.Int32 // status of health checks, default 200
	hits   atomic.Int32 // proxied requests that reached this upstream
	// hold makes proxied requests wait: each one sends the upstream's name on
	// env.arrivals and then waits until the test calls releaseOne.
	hold    atomic.Bool
	release chan struct{}
}

func (e *env) upstream(name string) *upstream {
	e.t.Helper()
	u := &upstream{e: e, name: name, release: make(chan struct{})}
	u.status.Store(http.StatusOK)
	u.health.Store(http.StatusOK)
	u.URL = e.server(u).URL
	return u
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == healthPath {
		w.WriteHeader(int(u.health.Load()))
		return
	}
	u.hits.Add(1)
	if u.hold.Load() {
		u.e.arrivals <- u.name
		select {
		case <-u.release:
		case <-u.e.quit:
		case <-r.Context().Done():
		}
	}
	w.WriteHeader(int(u.status.Load()))
	_, _ = io.WriteString(w, u.name)
}

// releaseOne lets one held request of u finish.
func (u *upstream) releaseOne(t *testing.T) {
	t.Helper()
	select {
	case u.release <- struct{}{}:
	case <-time.After(timeout):
		t.Fatalf("no held request at upstream %q to release within %v", u.name, timeout)
	}
}

// waitArrival waits until a held request arrives at an upstream and fails
// the test if it arrives at another upstream than want.
func (e *env) waitArrival(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-e.arrivals:
		if got != want {
			t.Fatalf("the request went to upstream %q, want %q", got, want)
		}
	case <-time.After(timeout):
		t.Fatalf("no request arrived at upstream %q within %v", want, timeout)
	}
}

// ----- building an LB -----

// config returns a Config for the backends that uses the env's transport and
// clock, health checks on healthPath, and LeastConnections. With no load,
// LeastConnections always picks the first eligible backend in the list, which
// makes it easy to predict where a request goes.
func (e *env) config(backends ...string) loadbalancer.Config {
	return loadbalancer.Config{
		Backends:      backends,
		Strategy:      loadbalancer.LeastConnections(),
		HealthPath:    healthPath,
		HealthTimeout: 2 * time.Second,
		Transport:     e.transport,
		Now:           e.clock.Now,
	}
}

// newLB calls loadbalancer.New and fails the test on an error or panic.
func newLB(t *testing.T, c loadbalancer.Config) *loadbalancer.LB {
	t.Helper()
	var lb *loadbalancer.LB
	var err error
	within(t, "New", func() { lb, err = loadbalancer.New(c) })
	if err != nil {
		t.Fatalf("New(%+v) error = %v, want nil", c.Backends, err)
	}
	if lb == nil {
		t.Fatalf("New returned a nil *LB and no error")
	}
	return lb
}

// ----- sending requests -----

// do sends req through lb.ServeHTTP and returns the recorded response. It
// fails the test if ServeHTTP panics or does not return within the timeout.
func do(t *testing.T, lb http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	within(t, req.Method+" "+req.URL.Path, func() { lb.ServeHTTP(rec, req) })
	return rec
}

// get sends a GET for path. The client address is 192.0.2.1 (the default of
// httptest.NewRequest).
func get(t *testing.T, lb http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, lb, httptest.NewRequest(http.MethodGet, "http://lb.example"+path, nil))
}

type pending struct {
	rec      *httptest.ResponseRecorder
	panicked any
}

// start sends req through lb.ServeHTTP in a new goroutine. Use await to get
// the response.
func start(lb http.Handler, req *http.Request) <-chan pending {
	ch := make(chan pending, 1)
	go func() {
		p := pending{rec: httptest.NewRecorder()}
		defer func() {
			p.panicked = recover()
			ch <- p
		}()
		lb.ServeHTTP(p.rec, req)
	}()
	return ch
}

func startGet(lb http.Handler, path string) <-chan pending {
	return start(lb, httptest.NewRequest(http.MethodGet, "http://lb.example"+path, nil))
}

// await waits for a request started with start.
func await(t *testing.T, ch <-chan pending) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case p := <-ch:
		if p.panicked != nil {
			t.Fatalf("ServeHTTP panicked: %v", p.panicked)
		}
		return p.rec
	case <-time.After(timeout):
		t.Fatalf("ServeHTTP did not return within %v", timeout)
		return nil
	}
}

// checkResponse fails the test if rec does not have the status and body.
func checkResponse(t *testing.T, rec *httptest.ResponseRecorder, wantCode int, wantBody string) {
	t.Helper()
	if rec.Code != wantCode || rec.Body.String() != wantBody {
		t.Fatalf("response = %d %q, want %d %q", rec.Code, rec.Body.String(), wantCode, wantBody)
	}
}

// ----- status helpers -----

// status returns the BackendStatus of the backend with the URL.
func status(t *testing.T, lb *loadbalancer.LB, rawURL string) loadbalancer.BackendStatus {
	t.Helper()
	var all []loadbalancer.BackendStatus
	within(t, "Status", func() { all = lb.Status() })
	for _, s := range all {
		if s.URL == rawURL {
			return s
		}
	}
	t.Fatalf("Status() = %+v has no entry with URL %q", all, rawURL)
	return loadbalancer.BackendStatus{}
}

// waitInFlight waits until the backend has n requests in flight.
func waitInFlight(t *testing.T, lb *loadbalancer.LB, rawURL string, n int64) {
	t.Helper()
	waitFor(t, func() string {
		if got := status(t, lb, rawURL).InFlight; got != n {
			return fmt.Sprintf("InFlight of %s is %d, want %d", rawURL, got, n)
		}
		return ""
	})
}

// ----- generic helpers -----

// waitFor polls cond until it returns "". Otherwise cond returns what is
// still wrong, and waitFor reports it if the timeout passes.
func waitFor(t *testing.T, cond func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		msg := cond()
		if msg == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("still after %v: %s", timeout, msg)
		}
		time.Sleep(time.Millisecond)
	}
}

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics or does not return within the timeout.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case v := <-done:
		if v != nil {
			t.Fatalf("%s panicked: %v", what, v)
		}
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v", what, timeout)
	}
}

// catchPanic turns a panic in the test's own goroutine, such as the
// "TODO: implement" of a function you have not written yet, into a test
// failure, so the other tests still run. Use it as: defer catchPanic(t).
func catchPanic(t *testing.T) {
	t.Helper()
	if v := recover(); v != nil {
		t.Fatalf("panicked: %v", v)
	}
}
