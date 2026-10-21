package middleware_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	middleware "github.com/yornik/go-practice/exercises/41-middleware"
)

// ---------------------------------------------------------------- AccessLog

func TestAccessLogRecord(t *testing.T) {
	t.Run("fields", func(t *testing.T) {
		logger, logs := newLogger()
		clock := newClock()
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clock.Advance(1500 * time.Millisecond) // the handler takes 1.5 s
			w.WriteHeader(http.StatusCreated)
			write(w, `{"id":42}`)
		})
		r := httptest.NewRequest(http.MethodPost, "/orders?token=secret", nil)
		serve(t, middleware.Chain(h, middleware.AccessLog(logger, clock.Now)), r)

		rec := one(t, logs.records(t), "log")
		checkAttrs(t, rec, map[string]any{
			"level":      "INFO",
			"msg":        "request",
			"method":     "POST",
			"path":       "/orders", // no query string: it can hold secrets
			"status":     201,
			"bytes":      9,
			"duration":   float64(1500 * time.Millisecond), // the JSON handler writes a time.Duration in nanoseconds
			"request_id": "",
		})
	})

	t.Run("with a request ID", func(t *testing.T) {
		// RequestID runs first, so the context that AccessLog sees has the ID.
		logger, logs := newLogger()
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		chained := middleware.Chain(h,
			middleware.RequestID(func() string { return "req-42" }),
			middleware.AccessLog(logger, newClock().Now),
		)
		serve(t, chained, httptest.NewRequest(http.MethodGet, "/", nil))
		checkAttrs(t, one(t, logs.records(t), "log"), map[string]any{"request_id": "req-42"})
	})

	t.Run("nil clock", func(t *testing.T) {
		logger, logs := newLogger()
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		serve(t, middleware.Chain(h, middleware.AccessLog(logger, nil)), httptest.NewRequest(http.MethodGet, "/", nil))
		rec := one(t, logs.records(t), "log")
		if d, ok := rec["duration"].(float64); !ok || d < 0 {
			t.Errorf("AccessLog(logger, nil): duration = %v, want a number >= 0 (nil means time.Now)", rec["duration"])
		}
	})
}

func TestAccessLogStatus(t *testing.T) {
	tests := []struct {
		name      string
		handler   func(w http.ResponseWriter)
		status    int
		bytes     int
		clientGot string // the body the client must see
	}{
		{
			name:    "Write without WriteHeader means 200",
			handler: func(w http.ResponseWriter) { write(w, "hello") },
			status:  200, bytes: 5, clientGot: "hello",
		},
		{
			name:    "nothing written means 200",
			handler: func(w http.ResponseWriter) {},
			status:  200, bytes: 0,
		},
		{
			name: "WriteHeader then Write",
			handler: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusNotFound)
				write(w, "no such app")
			},
			status: 404, bytes: 11, clientGot: "no such app",
		},
		{
			name: "several writes add up",
			handler: func(w http.ResponseWriter) {
				write(w, "ab")
				write(w, "cde")
				_, _ = w.Write(nil)
			},
			status: 200, bytes: 5, clientGot: "abcde",
		},
		{
			name: "the first WriteHeader wins",
			handler: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				w.WriteHeader(http.StatusInternalServerError) // the client still gets 202
			},
			status: 202, bytes: 0,
		},
		{
			name: "Write sends the header, a later WriteHeader is too late",
			handler: func(w http.ResponseWriter) {
				write(w, "partial")
				w.WriteHeader(http.StatusInternalServerError)
			},
			status: 200, bytes: 7, clientGot: "partial",
		},
		{
			name: "Flush sends the header, a later WriteHeader is too late",
			handler: func(w http.ResponseWriter) {
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				w.WriteHeader(http.StatusInternalServerError)
			},
			status: 200, bytes: 0,
		},
		{
			name: "only bytes that were written count",
			handler: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusNoContent)
				write(w, "ignored") // a 204 has no body: Write returns 0, http.ErrBodyNotAllowed
			},
			status: 204, bytes: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, logs := newLogger()
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tc.handler(w) })
			w := serve(t, middleware.Chain(h, middleware.AccessLog(logger, newClock().Now)), get("/"))

			if w.status != tc.status || w.body.String() != tc.clientGot {
				t.Errorf("the client got %d %q, want %d %q (pass every call on to the wrapped ResponseWriter)",
					w.status, w.body.String(), tc.status, tc.clientGot)
			}
			checkAttrs(t, one(t, logs.records(t), "log"), map[string]any{"status": tc.status, "bytes": tc.bytes})
		})
	}
}

func TestAccessLogConcurrent(t *testing.T) {
	// 20 requests are in the handler at the same time, each with its own
	// status and body size. Every log line must describe its own request:
	// whatever records the status and the size belongs to one request.
	const n = 20
	logger, logs := newLogger()
	var arrived atomic.Int32
	var gaveUp atomic.Bool
	all := make(chan struct{})    // closed when all 20 are in the handler
	giveUp := make(chan struct{}) // closed after the timeout, so a wrong solution cannot hang the test
	stop := time.AfterFunc(timeout, func() { close(giveUp) })
	defer stop.Stop()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if arrived.Add(1) == n {
			close(all)
		}
		select {
		case <-all:
		case <-giveUp:
			gaveUp.Store(true)
		}
		i, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		w.WriteHeader(400 + i) // 400 to 419: all of them may have a body
		write(w, strings.Repeat("x", i))
	})
	chained := middleware.Chain(h, middleware.AccessLog(logger, newClock().Now))

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v := callHandler(chained, newSpy(), get(fmt.Sprintf("/%d", i))); v != nil {
				t.Errorf("request %d panicked: %v", i, v)
			}
		}()
	}
	wg.Wait()

	if gaveUp.Load() {
		t.Fatalf("the %d requests were not all in the handler at the same time within %v: AccessLog must not make requests wait for each other", n, timeout)
	}
	recs := logs.records(t)
	if len(recs) != n {
		t.Fatalf("got %d log records for %d requests, want %d", len(recs), n, n)
	}
	for _, rec := range recs {
		path, _ := rec["path"].(string)
		i, _ := strconv.Atoi(strings.TrimPrefix(path, "/"))
		if rec["status"] != float64(400+i) || rec["bytes"] != float64(i) {
			t.Errorf("log record for %s has status %v and bytes %v, want %d and %d", path, rec["status"], rec["bytes"], 400+i, i)
		}
	}
}

func TestAccessLogPassesPanics(t *testing.T) {
	// Recovering is the job of Recover. AccessLog lets a panic through as is.
	logger, _ := newLogger()
	h := middleware.Chain(panicWith("boom"), middleware.AccessLog(logger, nil))
	if v := callHandler(h, newSpy(), get("/")); v != "boom" {
		t.Errorf("handler panicked with %q; what came out of AccessLog = %v, want the same panic", "boom", v)
	}
}

func TestAccessLogKeepsWriterFeatures(t *testing.T) {
	logger, _ := newLogger()
	checkWriterFeatures(t, middleware.AccessLog(logger, nil))
}

// checkWriterFeatures checks that the ResponseWriter a handler gets behind mw
// can still flush and still reaches the features of the writer underneath
// through http.ResponseController.
func checkWriterFeatures(t *testing.T, mw middleware.Middleware) {
	t.Helper()

	t.Run("http.Flusher", func(t *testing.T) {
		spy := newSpy()
		var isFlusher bool
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			write(w, "event: 1\n")
			var f http.Flusher
			if f, isFlusher = w.(http.Flusher); isFlusher {
				f.Flush()
			}
		}))
		if v := callHandler(h, spy, get("/stream")); v != nil {
			t.Fatalf("ServeHTTP panicked: %v", v)
		}
		if !isFlusher {
			t.Fatalf("the ResponseWriter the handler gets does not implement http.Flusher: streaming responses (SSE, watch APIs) cannot flush")
		}
		if spy.flushes != 1 {
			t.Errorf("the handler called Flush once, the underlying ResponseWriter was flushed %d times, want 1", spy.flushes)
		}
	})

	t.Run("ResponseController.Flush", func(t *testing.T) {
		spy := newSpy()
		var err error
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			write(w, "event: 1\n")
			err = http.NewResponseController(w).Flush()
		}))
		if v := callHandler(h, spy, get("/stream")); v != nil {
			t.Fatalf("ServeHTTP panicked: %v", v)
		}
		if err != nil || spy.flushes != 1 {
			t.Errorf("http.NewResponseController(w).Flush() = %v and %d flushes underneath, want nil and 1", err, spy.flushes)
		}
	})

	t.Run("ResponseController.SetWriteDeadline", func(t *testing.T) {
		// Only the writer underneath can set a deadline. ResponseController
		// gets there through the wrappers' Unwrap methods. The request runs in
		// its own goroutine: an Unwrap that returns the wrapper itself makes
		// ResponseController loop forever.
		deadline := time.Date(2026, 10, 17, 12, 0, 30, 0, time.UTC)
		var err error
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err = http.NewResponseController(w).SetWriteDeadline(deadline)
		}))
		spy := await(t, start(h, get("/export")), "SetWriteDeadline through the wrapper (does Unwrap return the writer it wraps?)")
		if err != nil {
			t.Errorf("http.NewResponseController(w).SetWriteDeadline() = %v, want nil (does the wrapper have an Unwrap method?)", err)
		}
		if !spy.deadline.Equal(deadline) {
			t.Errorf("the underlying ResponseWriter got deadline %v, want %v", spy.deadline, deadline)
		}
	})

	t.Run("Flush when the writer underneath cannot flush", func(t *testing.T) {
		spy := newSpy()
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if f, ok := w.(http.Flusher); ok {
				f.Flush() // must do nothing, not panic
			}
			write(w, "ok")
		}))
		if v := callHandler(h, plainWriter{spy}, get("/")); v != nil {
			t.Fatalf("over a ResponseWriter without Flush: ServeHTTP panicked: %v (Flush must do nothing then)", v)
		}
		if spy.status != http.StatusOK || spy.body.String() != "ok" {
			t.Errorf("over a ResponseWriter without Flush: the client got %d %q, want %d %q", spy.status, spy.body.String(), http.StatusOK, "ok")
		}
	})
}

// ---------------------------------------------------------------- Recover

// panicWith returns a handler that panics with v.
func panicWith(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		panic(v)
	}
}

// stats has the most common bug in Go services: nobody made its map, and
// countHit writes to the nil map.
type stats struct{ hits map[string]int }

func (s *stats) countHit(w http.ResponseWriter, r *http.Request) {
	s.hits[r.URL.Path]++
}

func TestRecoverBeforeWrite(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		value   string // the panic attribute: the panic value formatted with %v
		inStack string // a function the stack trace must show
	}{
		{"string", panicWith("boom"), "boom", "panicWith"},
		{"error", panicWith(errors.New("db: connection reset")), "db: connection reset", "panicWith"},
		{"runtime error", (&stats{}).countHit, "assignment to entry in nil map", "countHit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, logs := newLogger()
			w := serve(t, middleware.Chain(tc.handler, middleware.Recover(logger)), get("/orders?token=secret"))

			if w.status != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d", w.status, http.StatusInternalServerError)
			}
			if len(w.superfluous) > 0 {
				t.Errorf("WriteHeader was called again after the header was sent, with %v", w.superfluous)
			}
			rec := one(t, logs.records(t), "log")
			checkAttrs(t, rec, map[string]any{
				"level":      "ERROR",
				"msg":        "panic",
				"method":     "GET",
				"path":       "/orders", // no query string, as in AccessLog
				"request_id": "",
				"panic":      tc.value,
			})
			stack, _ := rec["stack"].(string)
			if !strings.Contains(stack, "goroutine ") || !strings.Contains(stack, tc.inStack) {
				t.Errorf("stack attribute does not look like the stack of the goroutine that panicked (want it to mention %s):\n%s",
					tc.inStack, stack)
			}
		})
	}
}

func TestRecoverAfterWrite(t *testing.T) {
	// The header is on its way to the client: the status can no longer change.
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		status    int
		clientGot string
	}{
		{
			name: "after WriteHeader",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				panic("boom")
			},
			status: http.StatusAccepted,
		},
		{
			name: "after Write",
			handler: func(w http.ResponseWriter, r *http.Request) {
				write(w, `{"items":[1,2,`)
				panic("boom")
			},
			status: http.StatusOK, clientGot: `{"items":[1,2,`,
		},
		{
			name: "after Flush",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				panic("boom")
			},
			status: http.StatusOK,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, logs := newLogger()
			w := serve(t, middleware.Chain(tc.handler, middleware.Recover(logger)), get("/"))

			if w.status != tc.status || w.body.String() != tc.clientGot {
				t.Errorf("the client got %d %q, want %d %q (nothing may be added after the panic)",
					w.status, w.body.String(), tc.status, tc.clientGot)
			}
			if len(w.superfluous) > 0 {
				t.Errorf("WriteHeader was called again after the header was sent, with %v", w.superfluous)
			}
			checkAttrs(t, one(t, logs.records(t), "log"), map[string]any{"level": "ERROR", "msg": "panic", "panic": "boom"})
		})
	}
}

func TestRecoverAbortHandler(t *testing.T) {
	// http.ErrAbortHandler tells net/http to cut the connection without
	// logging. Recover must let it through.
	logger, logs := newLogger()
	spy := newSpy()
	v := callHandler(middleware.Chain(panicWith(http.ErrAbortHandler), middleware.Recover(logger)), spy, get("/"))
	if v != http.ErrAbortHandler {
		t.Errorf("handler panicked with http.ErrAbortHandler; what came out of Recover = %v, want http.ErrAbortHandler", v)
	}
	if recs := logs.records(t); len(recs) != 0 {
		t.Errorf("Recover logged %d records for http.ErrAbortHandler, want none:\n%s", len(recs), dump(recs))
	}
	if spy.status != 0 {
		t.Errorf("Recover wrote a response with status %d for http.ErrAbortHandler, want nothing written", spy.status)
	}
}

func TestRecoverWithoutPanic(t *testing.T) {
	logger, logs := newLogger()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		write(w, "created")
	})
	w := serve(t, middleware.Chain(h, middleware.Recover(logger)), get("/"))
	if w.status != http.StatusCreated || w.body.String() != "created" {
		t.Errorf("the client got %d %q, want %d %q", w.status, w.body.String(), http.StatusCreated, "created")
	}
	if recs := logs.records(t); len(recs) != 0 {
		t.Errorf("Recover logged %d records without a panic, want none:\n%s", len(recs), dump(recs))
	}
}

func TestRecoverKeepsWriterFeatures(t *testing.T) {
	logger, _ := newLogger()
	checkWriterFeatures(t, middleware.Recover(logger))
}
