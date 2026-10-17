// Package loadbalancer is a small, health-aware HTTP reverse proxy: it spreads
// requests over backends, stops sending to backends that fail, retries safe
// requests on another backend and drains a backend before maintenance.
package loadbalancer

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ErrUnknownBackend is returned (wrapped or not) by Drain for a URL that is
// not in Config.Backends.
var ErrUnknownBackend = errors.New("unknown backend")

// Config configures an LB. Negative numbers and durations are errors. Zero
// values get the default given for each field.
type Config struct {
	// Backends are the base URLs of the backends, such as
	// "http://10.0.0.7:8080". At least 1. Each must be valid for NewBackend,
	// and the same string must not appear twice.
	Backends []string
	// Strategy picks the backend for each attempt. nil means RoundRobin().
	Strategy Strategy

	// HealthPath is the path of the active health check, appended to each
	// backend's URL. "" means "/healthz".
	HealthPath string
	// HealthInterval is the time between two rounds of RunHealthChecks.
	// 0 means 10s.
	HealthInterval time.Duration
	// HealthTimeout bounds one health check. 0 means 2s.
	HealthTimeout time.Duration
	// UnhealthyAfter is the number of consecutive failed checks that make a
	// healthy backend unhealthy. 0 means 1.
	UnhealthyAfter int
	// HealthyAfter is the number of consecutive passed checks that make an
	// unhealthy backend healthy again. 0 means 1.
	HealthyAfter int

	// EjectAfter is the number of consecutive failed requests (a 5xx
	// response or a transport error) that eject a backend. 0 turns passive
	// ejection off.
	EjectAfter int
	// EjectFor is how long an ejected backend gets no requests. 0 means 30s.
	EjectFor time.Duration

	// MaxRetries is the maximum number of extra attempts, each on another
	// backend, after a transport error on a safe request. 0 means no retries.
	MaxRetries int

	// Transport sends the proxied requests and the health checks. nil means
	// http.DefaultTransport.
	Transport http.RoundTripper
	// Now returns the current time; ejection uses it. nil means time.Now.
	Now func() time.Time
}

// BackendStatus is a snapshot of one backend.
type BackendStatus struct {
	URL      string // as written in Config.Backends
	Healthy  bool   // the result of the active health checks
	Ejected  bool   // passive ejection is in effect at Now()
	Draining bool   // Drain was called for this backend
	InFlight int64  // requests in flight right now
}

// LB is a load-balancing reverse proxy. All its methods are safe for
// concurrent use.
type LB struct {
	// TODO: add fields.
}

// New validates c, fills in the defaults and returns an LB. Every backend
// starts healthy, not ejected, not draining. New starts no goroutines.
func New(c Config) (*LB, error) {
	panic("TODO: implement")
}

// ServeHTTP proxies r to a backend chosen by the Strategy from the eligible
// backends (healthy, not ejected, not draining), with
// httputil.ReverseProxy. See the README for retries, ejection and the status
// codes the LB answers with itself (502, 503).
func (lb *LB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	panic("TODO: implement")
}

// CheckHealth runs one round of active health checks: a GET of HealthPath on
// every backend, all at the same time, each bounded by HealthTimeout. It
// returns when every check has finished. A 2xx response passes; anything
// else fails. If ctx is done when a check ends, its result is ignored.
func (lb *LB) CheckHealth(ctx context.Context) {
	panic("TODO: implement")
}

// RunHealthChecks runs a round of CheckHealth at once and then one every
// HealthInterval, until ctx is done. It blocks and returns only after ctx is
// done and the last round has finished.
func (lb *LB) RunHealthChecks(ctx context.Context) {
	panic("TODO: implement")
}

// Drain stops new requests to the backend whose URL (as written in
// Config.Backends) is backend, then waits until it has no request in flight
// and returns nil. After that, no request reaches the backend. If ctx ends
// first, Drain returns ctx.Err(). The backend stays draining in both cases.
// An unknown URL gives ErrUnknownBackend. Several Drains of one backend may
// wait at the same time.
func (lb *LB) Drain(ctx context.Context, backend string) error {
	panic("TODO: implement")
}

// Status returns a snapshot of every backend, in the order of
// Config.Backends.
func (lb *LB) Status() []BackendStatus {
	panic("TODO: implement")
}
