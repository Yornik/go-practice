package healthcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	healthcheck "github.com/yornik/go-practice/exercises/40-healthcheck"
)

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// window is how long a test watches for something that must NOT happen, such
// as a second run of a check. A correct solution never does it, so the window
// only decides how likely it is that a wrong solution gets caught. It never
// makes a correct solution fail.
const window = 100 * time.Millisecond

// fakeClock is a clock that moves only when Advance is called. Checks call
// Advance to pretend that they took a while. It is safe for concurrent use,
// because the checks run in several goroutines.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock starts at the real current time. The tests only look at the
// differences between its readings.
func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Now()}
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

// gate blocks checks and handlers until the test opens it. It opens by
// itself when the test ends, so nothing stays blocked after a failure.
type gate struct {
	ch   chan struct{}
	once sync.Once
}

func newGate(t *testing.T) *gate {
	g := &gate{ch: make(chan struct{})}
	t.Cleanup(g.open)
	return g
}

func (g *gate) open() { g.once.Do(func() { close(g.ch) }) }

// event is a channel that is closed once, the first time fire is called.
// Checks, handlers and hooks use it to tell the test that something happened.
type event struct {
	ch   chan struct{}
	once sync.Once
}

func newEvent() *event                 { return &event{ch: make(chan struct{})} }
func (s *event) fire()                 { s.once.Do(func() { close(s.ch) }) }
func (s *event) done() <-chan struct{} { return s.ch }

// check returns a check that counts its calls in calls (if not nil) and
// returns err.
func check(name string, critical bool, err error, calls *atomic.Int32) healthcheck.Check {
	return healthcheck.Check{
		Name:     name,
		Critical: critical,
		Timeout:  time.Second,
		Fn: func(context.Context) error {
			if calls != nil {
				calls.Add(1)
			}
			return err
		},
	}
}

// newHealth returns a Health with the given checks registered. It fails the
// test if Register returns an error.
func newHealth(t *testing.T, o healthcheck.Options, checks ...healthcheck.Check) *healthcheck.Health {
	t.Helper()
	h := healthcheck.New(o)
	for _, c := range checks {
		if err := h.Register(c); err != nil {
			t.Fatalf("Register(check %q) = %v, want nil", c.Name, err)
		}
	}
	return h
}

// response is what a handler answered.
type response struct {
	Code   int
	Header http.Header
	Body   string
}

type result struct {
	resp     response
	panicked any
}

// startRequest sends one request to handler in a new goroutine. The returned
// channel receives the response when the handler returns.
func startRequest(handler http.Handler, req *http.Request) <-chan result {
	ch := make(chan result, 1)
	go func() {
		rec := httptest.NewRecorder()
		var res result
		defer func() {
			res.panicked = recover()
			// Result shows what a real client would get: headers set after
			// WriteHeader or Write are not in it.
			r := rec.Result()
			res.resp = response{Code: r.StatusCode, Header: r.Header, Body: rec.Body.String()}
			ch <- res
		}()
		handler.ServeHTTP(rec, req)
	}()
	return ch
}

// await waits for the response of startRequest. It fails the test if the
// handler panicked or did not answer within the timeout.
func await(t *testing.T, what string, ch <-chan result) response {
	t.Helper()
	select {
	case res := <-ch:
		if res.panicked != nil {
			t.Fatalf("%s: the handler panicked: %v", what, res.panicked)
		}
		return res.resp
	case <-time.After(timeout):
		t.Fatalf("%s did not answer within %v", what, timeout)
		return response{}
	}
}

// do sends "method path" to handler and waits for the response.
func do(t *testing.T, handler http.Handler, method, path string) response {
	t.Helper()
	return await(t, method+" "+path, startRequest(handler, httptest.NewRequest(method, path, nil)))
}

// report is the /readyz body as the README describes it.
type report struct {
	Status string        `json:"status"`
	Checks []checkResult `json:"checks"`
}

type checkResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	DurationMS *int64 `json:"duration_ms"` // a pointer, to catch a missing field
}

// decodeReport parses a /readyz response. It fails the test if the
// Content-Type is not JSON, the body does not parse, "checks" is not a JSON
// array or an entry has no duration_ms.
func decodeReport(t *testing.T, resp response) report {
	t.Helper()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("GET /readyz: Content-Type = %q, want application/json (headers set after WriteHeader or Write are not sent)", ct)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(resp.Body), &raw); err != nil {
		t.Fatalf("GET /readyz: the body is not a JSON object: %v\nbody: %s", err, resp.Body)
	}
	if c := bytes.TrimSpace(raw["checks"]); len(c) == 0 || c[0] != '[' {
		t.Fatalf("GET /readyz: \"checks\" = %s, want a JSON array ([] when there are no checks, never null or missing)\nbody: %s",
			raw["checks"], resp.Body)
	}
	var rep report
	if err := json.Unmarshal([]byte(resp.Body), &rep); err != nil {
		t.Fatalf("GET /readyz: cannot decode the report: %v\nbody: %s", err, resp.Body)
	}
	for _, c := range rep.Checks {
		if c.DurationMS == nil {
			t.Errorf("GET /readyz: check %q has no duration_ms\nbody: %s", c.Name, resp.Body)
		}
	}
	return rep
}

// getReadyz sends GET /readyz to handler and decodes the report.
func getReadyz(t *testing.T, handler http.Handler) (response, report) {
	t.Helper()
	resp := do(t, handler, http.MethodGet, "/readyz")
	return resp, decodeReport(t, resp)
}

// wantReadiness fails the test unless the response has the given HTTP code
// and top-level status.
func wantReadiness(t *testing.T, resp response, rep report, code int, status string) {
	t.Helper()
	if resp.Code != code || rep.Status != status {
		t.Fatalf("GET /readyz = %d with status %q, want %d with status %q\nbody: %s",
			resp.Code, rep.Status, code, status, resp.Body)
	}
}

// findCheck returns the entry of the check called name. It fails the test if
// the report has no such entry.
func findCheck(t *testing.T, rep report, name string) checkResult {
	t.Helper()
	for _, c := range rep.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("GET /readyz: the report has no check %q: %+v", name, rep.Checks)
	return checkResult{}
}

// names returns the check names of a report, in report order.
func names(rep report) []string {
	var out []string
	for _, c := range rep.Checks {
		out = append(out, c.Name)
	}
	return out
}

// waitFor polls cond until it returns true. It fails the test if that takes
// longer than the timeout.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(time.Millisecond)
	}
}

// waitClosed waits until ch is closed. It fails the test after the timeout.
func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out after %v waiting for %s", timeout, what)
	}
}

// within runs f in a new goroutine and fails the test if f panics or does
// not return within the timeout.
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

// expectNoLeak records how many goroutines exist now. The returned function
// fails the test if that number does not come back down within the timeout.
// On failure it prints every goroutine's stack, so you can see where the
// leaked one is stuck.
func expectNoLeak(t *testing.T) func() {
	t.Helper()
	before := runtime.NumGoroutine()
	return func() {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for runtime.NumGoroutine() > before {
			if time.Now().After(deadline) {
				var stacks bytes.Buffer
				_ = pprof.Lookup("goroutine").WriteTo(&stacks, 1)
				t.Fatalf("%d goroutines are still running %v after the check returned, but only %d existed before the request: "+
					"a goroutine started for the check is stuck. All goroutines:\n%s",
					runtime.NumGoroutine(), timeout, before, stacks.String())
			}
			time.Sleep(time.Millisecond)
		}
	}
}
