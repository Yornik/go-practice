package middleware_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// spyWriter is an http.ResponseWriter that follows the rules of the one
// net/http gives a handler, and records what the client would see:
//
//   - The first WriteHeader sends the header. A Write or a Flush before any
//     WriteHeader sends it with status 200.
//   - Once the header is sent, later WriteHeader calls change nothing. net/http
//     logs them as "superfluous response.WriteHeader call"; the spy records them
//     in superfluous.
//   - The header map is copied when the header is sent (wire). Changes made
//     after that never reach the client.
//   - A 204 or 304 response has no body: Write returns 0, http.ErrBodyNotAllowed.
//
// It also implements http.Flusher and SetWriteDeadline, which
// http.ResponseController finds and calls.
type spyWriter struct {
	header      http.Header
	wire        http.Header // the header as sent; nil until then
	status      int         // the status the client got; 0 until the header is sent
	body        bytes.Buffer
	superfluous []int // WriteHeader calls after the header was sent
	flushes     int
	deadline    time.Time // the last SetWriteDeadline
}

func newSpy() *spyWriter { return &spyWriter{header: http.Header{}} }

func (s *spyWriter) Header() http.Header { return s.header }

func (s *spyWriter) WriteHeader(code int) {
	if s.status != 0 {
		s.superfluous = append(s.superfluous, code)
		return
	}
	s.status = code
	s.wire = s.header.Clone()
}

func (s *spyWriter) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.WriteHeader(http.StatusOK)
	}
	if s.status == http.StatusNoContent || s.status == http.StatusNotModified {
		return 0, http.ErrBodyNotAllowed
	}
	return s.body.Write(p)
}

func (s *spyWriter) Flush() {
	if s.status == 0 {
		s.WriteHeader(http.StatusOK)
	}
	s.flushes++
}

// finish does what net/http does when a handler returns: if nothing was
// sent yet, it sends the header with status 200.
func (s *spyWriter) finish() {
	if s.status == 0 {
		s.WriteHeader(http.StatusOK)
	}
}

func (s *spyWriter) SetWriteDeadline(t time.Time) error {
	s.deadline = t
	return nil
}

// plainWriter is a ResponseWriter that can do nothing but the three methods
// of the interface: no Flush, no SetWriteDeadline, no Unwrap. Other
// libraries' wrappers often look like this.
type plainWriter struct{ spy *spyWriter }

func (p plainWriter) Header() http.Header         { return p.spy.Header() }
func (p plainWriter) Write(b []byte) (int, error) { return p.spy.Write(b) }
func (p plainWriter) WriteHeader(code int)        { p.spy.WriteHeader(code) }

// write writes s to w. Like most handlers, the test handlers ignore write
// errors: a failed write means the client is gone, and nobody is left to tell.
func write(w http.ResponseWriter, s string) { _, _ = io.WriteString(w, s) }

// get returns a GET request for path, as a server would pass it to a handler.
func get(path string) *http.Request { return httptest.NewRequest(http.MethodGet, path, nil) }

// callHandler calls h.ServeHTTP in the current goroutine and returns the
// value it panicked with, or nil.
func callHandler(h http.Handler, w http.ResponseWriter, r *http.Request) (panicked any) {
	defer func() { panicked = recover() }()
	h.ServeHTTP(w, r)
	return nil
}

// serve runs one request through h and returns what the client would see. A
// panic that comes out of h fails the test.
func serve(t *testing.T, h http.Handler, r *http.Request) *spyWriter {
	t.Helper()
	w := newSpy()
	if v := callHandler(h, w, r); v != nil {
		t.Fatalf("ServeHTTP(%s %s) panicked: %v", r.Method, r.URL, v)
	}
	w.finish()
	return w
}

// result is what start delivers.
type result struct {
	w        *spyWriter
	panicked any
}

// start runs one request through h in a new goroutine. The channel receives
// the result when ServeHTTP returns.
func start(h http.Handler, r *http.Request) <-chan result {
	ch := make(chan result, 1)
	go func() {
		w := newSpy()
		v := callHandler(h, w, r)
		if v == nil {
			w.finish()
		}
		ch <- result{w: w, panicked: v}
	}()
	return ch
}

// await waits for a request started with start. It fails the test if the
// request panicked or did not finish within the timeout.
func await(t *testing.T, ch <-chan result, what string) *spyWriter {
	t.Helper()
	select {
	case res := <-ch:
		if res.panicked != nil {
			t.Fatalf("%s: ServeHTTP panicked: %v", what, res.panicked)
		}
		return res.w
	case <-time.After(timeout):
		t.Fatalf("%s: ServeHTTP did not return within %v", what, timeout)
		return nil
	}
}

// waitFor polls cond until it returns true. It fails the test if that takes
// longer than the timeout.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting until %s", timeout, what)
		}
		time.Sleep(time.Millisecond)
	}
}

// fakeClock is a clock that only moves when the test calls Advance.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
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

// logBuffer collects the output of a JSON slog handler. It is safe for
// concurrent use, because handlers can log from many goroutines.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// newLogger returns a logger that writes JSON lines into the returned buffer,
// the way a service logs in production.
func newLogger() (*slog.Logger, *logBuffer) {
	b := &logBuffer{}
	return slog.New(slog.NewJSONHandler(b, nil)), b
}

// logRecord is one decoded JSON log line, for example
// {"time":"...","level":"INFO","msg":"request","method":"GET",...}.
// JSON numbers decode as float64.
type logRecord map[string]any

// records decodes every line logged so far.
func (b *logBuffer) records(t *testing.T) []logRecord {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var recs []logRecord
	sc := bufio.NewScanner(bytes.NewReader(b.buf.Bytes()))
	sc.Buffer(nil, 1<<20) // stack traces make long lines
	for sc.Scan() {
		var rec logRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("log line is not JSON: %v\n%s", err, sc.Text())
		}
		recs = append(recs, rec)
	}
	return recs
}

// withMsg returns the records whose msg is msg.
func withMsg(recs []logRecord, msg string) []logRecord {
	var out []logRecord
	for _, r := range recs {
		if r["msg"] == msg {
			out = append(out, r)
		}
	}
	return out
}

// one fails the test unless recs holds exactly one record, and returns it.
func one(t *testing.T, recs []logRecord, what string) logRecord {
	t.Helper()
	if len(recs) != 1 {
		t.Fatalf("got %d %s records, want 1:\n%s", len(recs), what, dump(recs))
	}
	return recs[0]
}

// checkAttrs compares the attributes of a log record with want. An int in
// want is compared with the JSON number in the record.
func checkAttrs(t *testing.T, rec logRecord, want map[string]any) {
	t.Helper()
	for key, w := range want {
		got, ok := rec[key]
		if !ok {
			t.Errorf("log record has no %q attribute:\n%s", key, dump([]logRecord{rec}))
			continue
		}
		if i, isInt := w.(int); isInt {
			w = float64(i)
		}
		if got != w {
			t.Errorf("log record %q = %v (%T), want %v (%T)", key, got, got, w, w)
		}
	}
}

// dump formats records for failure messages, without the long stack traces.
func dump(recs []logRecord) string {
	var sb strings.Builder
	for _, r := range recs {
		short := logRecord{}
		for k, v := range r {
			if s, ok := v.(string); ok && len(s) > 80 {
				v = s[:80] + "..."
			}
			short[k] = v
		}
		b, _ := json.Marshal(short)
		fmt.Fprintf(&sb, "  %s\n", b)
	}
	return sb.String()
}
