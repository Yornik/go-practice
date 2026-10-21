// Package middleware provides the HTTP middleware that almost every service
// needs: request IDs, access logs, panic recovery, request body limits and
// load shedding. Each one is a Middleware, and Chain stacks them.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Middleware wraps an http.Handler and returns a new http.Handler that adds
// behaviour before and/or after calling the wrapped one.
type Middleware func(http.Handler) http.Handler

// Chain wraps h with the middleware in m. m[0] is the outermost: it sees the
// request first and the response last. Chain(h, a, b) is a(b(h)).
//
// Chain calls every middleware exactly once, when Chain is called, not once
// per request. With no middleware, Chain returns a handler that behaves like h.
func Chain(h http.Handler, m ...Middleware) http.Handler {
	panic("TODO: implement")
}

// RequestIDHeader is the header that carries the request ID, on the request
// and on the response.
const RequestIDHeader = "X-Request-ID"

// RequestID returns a middleware that gives every request an ID.
//
// If the request has a valid X-Request-ID header (1 to 128 characters, each
// one of A-Z, a-z, 0-9, '.', '_' or '-'), the middleware reuses that ID.
// Otherwise it calls gen exactly once for a new one. It sets the ID as the
// X-Request-ID response header before it calls the next handler, and passes
// the next handler a request whose context carries the ID (see RequestIDFrom).
//
// The context key is of an unexported type, so no other package can read or
// overwrite the ID by accident.
func RequestID(gen func() string) Middleware {
	panic("TODO: implement")
}

// RequestIDFrom returns the request ID that the RequestID middleware stored
// in ctx, or "" if there is none.
func RequestIDFrom(ctx context.Context) string {
	panic("TODO: implement")
}

// AccessLog returns a middleware that logs one record per request, after the
// next handler has returned: level INFO, message "request", and the
// attributes
//
//	method      string         r.Method
//	path        string         r.URL.Path, without the query string
//	status      int            the status the client got
//	bytes       int64          the number of response body bytes written
//	duration    time.Duration  from before the next handler was called to after it returned
//	request_id  string         RequestIDFrom(r.Context()), "" if there is none
//
// The next handler gets a ResponseWriter that still supports http.Flusher
// and http.NewResponseController. AccessLog does not recover panics.
//
// now is the clock for the duration. nil means time.Now.
func AccessLog(l *slog.Logger, now func() time.Time) Middleware {
	panic("TODO: implement")
}

// Recover returns a middleware that turns a panic in the next handler into a
// log record and, if the response has not started yet, a 500 response.
//
// The record has level ERROR, message "panic", and the attributes method,
// path, request_id (as in AccessLog), panic (the panic value formatted with
// %v) and stack (the stack trace of the goroutine that panicked).
//
// If the handler already sent the header (with WriteHeader, Write or Flush),
// Recover only logs: it writes no second header and no body.
//
// A panic with http.ErrAbortHandler is not recovered: Recover panics again
// with the same value, logs nothing and writes nothing.
//
// The next handler gets a ResponseWriter that still supports http.Flusher
// and http.NewResponseController.
func Recover(l *slog.Logger) Middleware {
	panic("TODO: implement")
}

// MaxBytes returns a middleware that limits request bodies to n bytes.
//
// A request that declares a length above n (r.ContentLength > n) gets 413
// Request Entity Too Large, and the next handler is not called. Otherwise
// the next handler gets a body wrapped with http.MaxBytesReader: a Read
// beyond n bytes returns an error of type *http.MaxBytesError.
func MaxBytes(n int64) Middleware {
	panic("TODO: implement")
}

// LoadShed returns a middleware that lets at most maxInFlight requests run
// the next handler at the same time. A request over the limit gets 503
// Service Unavailable with a Retry-After header (retryAfter in whole seconds,
// rounded up), and the next handler is not called. LoadShed never waits for
// a free slot. A slot is free again when the next handler returns or panics.
//
// The limit belongs to the returned Middleware: every handler wrapped with
// it shares the same maxInFlight slots. maxInFlight must be at least 1.
// LoadShed is safe for concurrent use, like every http.Handler.
func LoadShed(maxInFlight int, retryAfter time.Duration) Middleware {
	panic("TODO: implement")
}
