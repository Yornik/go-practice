package middleware_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	middleware "github.com/yornik/go-practice/exercises/41-middleware"
)

// ---------------------------------------------------------------- Chain

// trace returns a middleware that records "name>" before it calls the next
// handler and "<name" after.
func trace(name string, events *[]string) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*events = append(*events, name+">")
			next.ServeHTTP(w, r)
			*events = append(*events, "<"+name)
		})
	}
}

func TestChainOrder(t *testing.T) {
	var events []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		events = append(events, "handler")
	})
	chained := middleware.Chain(h, trace("a", &events), trace("b", &events), trace("c", &events))
	serve(t, chained, httptest.NewRequest(http.MethodGet, "/", nil))

	// m[0] is the outermost: first in, last out.
	want := []string{"a>", "b>", "c>", "handler", "<c", "<b", "<a"}
	if !slices.Equal(events, want) {
		t.Errorf("Chain(h, a, b, c): events = %q, want %q", events, want)
	}
}

func TestChainWithoutMiddleware(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		write(w, "short and stout")
	})
	w := serve(t, middleware.Chain(h), httptest.NewRequest(http.MethodGet, "/", nil))
	if w.status != http.StatusTeapot || w.body.String() != "short and stout" {
		t.Errorf("Chain(h) answered %d %q, want %d %q", w.status, w.body.String(), http.StatusTeapot, "short and stout")
	}
}

func TestChainWrapsOnce(t *testing.T) {
	// Middleware constructors run setup work (a semaphore, a compiled regexp)
	// when they wrap. That must happen once, at startup, not per request.
	var wraps int
	counting := func(next http.Handler) http.Handler {
		wraps++
		return next
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	chained := middleware.Chain(h, counting, counting)
	if wraps != 2 {
		t.Fatalf("after Chain(h, m, m): the middleware was called %d times, want 2", wraps)
	}
	for range 3 {
		serve(t, chained, httptest.NewRequest(http.MethodGet, "/", nil))
	}
	if wraps != 2 {
		t.Errorf("after 3 requests: the middleware was called %d times in total, want still 2 (Chain must wrap once, not per request)", wraps)
	}
}

// ---------------------------------------------------------------- RequestID

// idSequence returns a generator for "gen-1", "gen-2", ... and a counter of
// how often it was called.
func idSequence() (gen func() string, calls *atomic.Int32) {
	calls = new(atomic.Int32)
	gen = func() string { return fmt.Sprintf("gen-%d", calls.Add(1)) }
	return gen, calls
}

// idEcho is a handler that writes the request ID it finds in the context as
// the body. Writing sends the header, so a response header set after the
// handler returns never reaches the client.
var idEcho = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	write(w, middleware.RequestIDFrom(r.Context()))
})

func TestRequestIDGenerates(t *testing.T) {
	gen, calls := idSequence()
	h := middleware.Chain(idEcho, middleware.RequestID(gen))

	for i := 1; i <= 2; i++ {
		want := fmt.Sprintf("gen-%d", i)
		w := serve(t, h, httptest.NewRequest(http.MethodGet, "/", nil))
		if got := w.body.String(); got != want {
			t.Errorf("request %d: RequestIDFrom(r.Context()) in the handler = %q, want %q", i, got, want)
		}
		if got := w.wire.Get(middleware.RequestIDHeader); got != want {
			t.Errorf("request %d: response header %s = %q, want %q (set it before calling the next handler)", i, middleware.RequestIDHeader, got, want)
		}
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("gen was called %d times for 2 requests without an ID, want 2", n)
	}
}

func TestRequestIDReusesValid(t *testing.T) {
	valid := []struct{ name, id string }{
		{"one char", "a"},
		{"UUID", "0f8fad5b-d9cb-469f-a165-70867728950e"}, // what most proxies send
		{"dots and underscores", "req_2026.10.17-001"},
		{"mixed case", "ABCxyz789"},
		{"128 chars", strings.Repeat("x", 128)}, // the longest allowed
	}
	for _, tc := range valid {
		id := tc.id
		t.Run(tc.name, func(t *testing.T) {
			gen, calls := idSequence()
			h := middleware.Chain(idEcho, middleware.RequestID(gen))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set(middleware.RequestIDHeader, id)
			w := serve(t, h, r)
			if got := w.body.String(); got != id {
				t.Errorf("incoming ID %q: RequestIDFrom in the handler = %q, want the incoming ID", id, got)
			}
			if got := w.wire.Get(middleware.RequestIDHeader); got != id {
				t.Errorf("incoming ID %q: response header = %q, want the incoming ID", id, got)
			}
			if n := calls.Load(); n != 0 {
				t.Errorf("incoming ID %q: gen was called %d times, want 0", id, n)
			}
		})
	}
}

func TestRequestIDReplacesInvalid(t *testing.T) {
	invalid := []struct{ name, id string }{
		{"empty", ""},
		{"129 chars", strings.Repeat("x", 129)},
		{"space", "abc def"},
		{"newline", "abc\ninjected=1"}, // would forge a log line
		{"slash", "../../etc"},
		{"quote", `abc"def`},
		{"semicolon", "abc;def"},
		{"non-ASCII", "zürich-1"},
		{"control byte", "abc\x00"},
		{"colon and tab", "a:b\tc"},
	}
	for _, tc := range invalid {
		id := tc.id
		t.Run(tc.name, func(t *testing.T) {
			gen, calls := idSequence()
			h := middleware.Chain(idEcho, middleware.RequestID(gen))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set(middleware.RequestIDHeader, id) // Set stores any value as is
			w := serve(t, h, r)
			if got := w.body.String(); got != "gen-1" {
				t.Errorf("incoming ID %q: RequestIDFrom in the handler = %q, want a generated ID %q", id, got, "gen-1")
			}
			if got := w.wire.Get(middleware.RequestIDHeader); got != "gen-1" {
				t.Errorf("incoming ID %q: response header = %q, want %q", id, got, "gen-1")
			}
			if n := calls.Load(); n != 1 {
				t.Errorf("incoming ID %q: gen was called %d times, want 1", id, n)
			}
		})
	}
}

func TestRequestIDFrom(t *testing.T) {
	if got := middleware.RequestIDFrom(context.Background()); got != "" {
		t.Errorf("RequestIDFrom(context.Background()) = %q, want %q", got, "")
	}

	// Other code may store values under plain string keys. They must not be
	// mistaken for the request ID: the key is of an unexported type.
	ctx := context.Background()
	for _, key := range []string{"request_id", "requestID", "requestid", "request-id", middleware.RequestIDHeader} {
		ctx = context.WithValue(ctx, key, "not-the-id") //nolint:staticcheck // a string key on purpose: the collision we guard against
	}
	if got := middleware.RequestIDFrom(ctx); got != "" {
		t.Errorf("RequestIDFrom(ctx with string keys) = %q, want %q", got, "")
	}
}
