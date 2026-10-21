package middleware_test

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	middleware "github.com/yornik/go-practice/exercises/41-middleware"
)

// stack wraps h in the order the README recommends, from the outside in.
func stack(h http.Handler, logger *slog.Logger) http.Handler {
	var ids atomic.Int32
	return middleware.Chain(h,
		middleware.RequestID(func() string { return fmt.Sprintf("req-%d", ids.Add(1)) }),
		middleware.AccessLog(logger, nil),
		middleware.Recover(logger),
		middleware.LoadShed(1, 10*time.Second),
		middleware.MaxBytes(1024),
	)
}

// routes is the application behind the stack in these tests.
func routes(g *gate) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		write(w, "hello")
	})
	mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
		panic("handler bug")
	})
	mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	})
	mux.Handle("GET /slow", g)
	return mux
}

func TestStackRecommendedOrder(t *testing.T) {
	// Every response carries the request ID, every request gets an access log
	// line with the status the client saw, also when a middleware further in
	// answered, and the panic record has the same ID as the access log line.
	tests := []struct {
		name   string
		req    func() *http.Request
		status int
		panics bool
	}{
		{
			name:   "ok",
			req:    func() *http.Request { return get("/hello") },
			status: http.StatusOK,
		},
		{
			name:   "panic",
			req:    func() *http.Request { return get("/panic") },
			status: http.StatusInternalServerError, panics: true,
		},
		{
			name: "body too large",
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("x", 2048)))
			},
			status: http.StatusRequestEntityTooLarge,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, logs := newLogger()
			w := serve(t, stack(routes(newGate()), logger), tc.req())
			if w.status != tc.status {
				t.Errorf("status = %d, want %d", w.status, tc.status)
			}
			if got := w.wire.Get(middleware.RequestIDHeader); got != "req-1" {
				t.Errorf("response header %s = %q, want %q", middleware.RequestIDHeader, got, "req-1")
			}
			recs := logs.records(t)
			checkAttrs(t, one(t, withMsg(recs, "request"), `"request"`), map[string]any{"status": tc.status, "request_id": "req-1"})
			if tc.panics {
				checkAttrs(t, one(t, withMsg(recs, "panic"), `"panic"`), map[string]any{"panic": "handler bug", "request_id": "req-1"})
			}
		})
	}

	t.Run("incoming ID", func(t *testing.T) {
		logger, logs := newLogger()
		r := get("/hello")
		r.Header.Set(middleware.RequestIDHeader, "edge-7f3a")
		w := serve(t, stack(routes(newGate()), logger), r)
		if got := w.wire.Get(middleware.RequestIDHeader); got != "edge-7f3a" {
			t.Errorf("response header %s = %q, want %q", middleware.RequestIDHeader, got, "edge-7f3a")
		}
		checkAttrs(t, one(t, logs.records(t), "log"), map[string]any{"request_id": "edge-7f3a"})
	})

	t.Run("shed", func(t *testing.T) {
		logger, logs := newLogger()
		g := newGate()
		h := stack(routes(g), logger)
		held := start(h, get("/slow"))
		waitFor(t, "1 request is in the slow handler", func() bool { return g.entered.Load() == 1 })

		w := await(t, start(h, get("/hello")), "request over the limit")
		if w.status != http.StatusServiceUnavailable || w.wire.Get("Retry-After") != "10" {
			t.Errorf("over the limit: %d with Retry-After %q, want %d with %q",
				w.status, w.wire.Get("Retry-After"), http.StatusServiceUnavailable, "10")
		}
		if got := w.wire.Get(middleware.RequestIDHeader); got == "" {
			t.Errorf("the 503 response has no %s header", middleware.RequestIDHeader)
		}
		close(g.release)
		await(t, held, "held request")

		statuses := map[float64]bool{}
		for _, rec := range withMsg(logs.records(t), "request") {
			status, _ := rec["status"].(float64)
			statuses[status] = true
		}
		if !statuses[503] || !statuses[200] {
			t.Errorf("access log has the statuses %v, want one 200 and one 503", statuses)
		}
	})
}

func TestStackKeepsWriterFeatures(t *testing.T) {
	logger, _ := newLogger()
	checkWriterFeatures(t, func(h http.Handler) http.Handler { return stack(h, logger) })
}

func TestStackOverRealServer(t *testing.T) {
	// A streaming endpoint behind the whole stack, on a real net/http server.
	// The client must receive the first event while the handler is still
	// running: Flush has to reach the connection through every wrapper.
	logger, logs := newLogger()
	firstSeen := make(chan struct{})
	type report struct {
		deadlineErr, flushErr error
		clientWasFirst        bool
	}
	reports := make(chan report, 1)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rep report
		rc := http.NewResponseController(w)
		rep.deadlineErr = rc.SetWriteDeadline(time.Now().Add(time.Minute))
		write(w, "event: 1\n")
		rep.flushErr = rc.Flush()
		select {
		case <-firstSeen:
			rep.clientWasFirst = true
		case <-time.After(timeout):
		}
		write(w, "event: 2\n")
		reports <- rep
	})
	srv := httptest.NewServer(stack(h, logger))
	var handlerReturned bool
	defer func() {
		// Close waits for every running handler. A handler that never returns
		// (for example in an endless Unwrap loop) must fail the test, not hang it.
		if handlerReturned {
			srv.Close()
		}
	}()
	client := srv.Client()
	client.Timeout = 2 * timeout

	resp, err := client.Get(srv.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if id := resp.Header.Get(middleware.RequestIDHeader); id != "req-1" {
		t.Errorf("response header %s = %q, want %q", middleware.RequestIDHeader, id, "req-1")
	}
	body := bufio.NewReader(resp.Body)
	line, err := body.ReadString('\n')
	if err != nil || line != "event: 1\n" {
		t.Fatalf("first line = %q, %v, want %q", line, err, "event: 1\n")
	}
	close(firstSeen)
	rest, _ := io.ReadAll(body)
	if string(rest) != "event: 2\n" {
		t.Errorf("rest of the body = %q, want %q", rest, "event: 2\n")
	}

	var rep report
	select {
	case rep = <-reports:
		handlerReturned = true
	case <-time.After(timeout):
		t.Fatalf("the handler did not finish")
	}
	if rep.deadlineErr != nil {
		t.Errorf("SetWriteDeadline through the stack on a real server = %v, want nil", rep.deadlineErr)
	}
	if rep.flushErr != nil {
		t.Errorf("Flush through the stack on a real server = %v, want nil", rep.flushErr)
	}
	if !rep.clientWasFirst {
		t.Errorf("the client got the first event only after the handler returned: Flush did not reach the connection")
	}

	// AccessLog logs after the handler returns, which can be a moment after
	// the client has the whole response.
	waitFor(t, "the access log has a record", func() bool { return len(withMsg(logs.records(t), "request")) == 1 })
	checkAttrs(t, withMsg(logs.records(t), "request")[0], map[string]any{
		"path": "/events", "status": 200, "bytes": 18, "request_id": "req-1",
	})
}
