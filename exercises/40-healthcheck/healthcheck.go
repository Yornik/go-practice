// Package healthcheck serves the health endpoints of a service that runs on
// Kubernetes. /livez says whether the process works at all. /readyz runs
// dependency checks with timeouts and says whether the pod should get traffic.
// Serve runs an HTTP server and shuts it down without dropping requests.
package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ErrInvalidCheck is returned by Register for a check with an empty Name, a
// nil Fn or a Timeout <= 0.
var ErrInvalidCheck = errors.New("healthcheck: invalid check")

// ErrDuplicateCheck is returned by Register when a check with the same Name is
// already registered.
var ErrDuplicateCheck = errors.New("healthcheck: duplicate check name")

// Check is one dependency check that /readyz runs.
type Check struct {
	// Name identifies the check in the report. It must not be empty and must
	// be unique within a Health.
	Name string

	// Critical checks decide readiness: if one of them fails or times out,
	// /readyz answers 503. A non-critical check is only reported.
	Critical bool

	// Timeout is how long the check may take. It must be > 0.
	Timeout time.Duration

	// Fn runs the check and returns nil if the dependency is healthy. Its ctx
	// is cancelled when Timeout expires. A call that ignores ctx may still be
	// running when the next run calls Fn again, so Fn must be safe for
	// concurrent use.
	Fn func(ctx context.Context) error
}

// Options configures a Health.
type Options struct {
	// CacheTTL is how long a /readyz result is reused, counted from the end
	// of the run that produced it. CacheTTL <= 0 means no caching: a result
	// is not reused once its run has ended.
	CacheTTL time.Duration

	// Now returns the current time. Health uses it for the cache and for
	// duration_ms in the report. nil means time.Now. Now may be called from
	// several goroutines at once.
	Now func() time.Time
}

// Health holds the registered checks and the state behind /livez and
// /readyz. All its methods and the handler it returns are safe for
// concurrent use.
type Health struct {
	// TODO: add fields.
}

// New returns a Health with no checks that is not draining.
func New(o Options) *Health {
	panic("TODO: implement")
}

// Register adds a check. It returns an error that matches ErrInvalidCheck if
// the check has an empty Name, a nil Fn or a Timeout <= 0, and an error that
// matches ErrDuplicateCheck if a check with the same Name is already
// registered. A rejected check is not added.
func (h *Health) Register(c Check) error {
	panic("TODO: implement")
}

// Handler returns the HTTP handler for two routes:
//
//   - GET /livez answers 200 "ok". It runs no check and never waits for one.
//   - GET /readyz runs the checks (or reuses a cached result) and answers
//     with a JSON report: 200 if every critical check passed, 503 otherwise,
//     and 503 "draining" after SetDraining. The request's context does not
//     cancel the checks.
//
// Other methods on these paths get 405 with an Allow header. Other paths get
// 404.
func (h *Health) Handler() http.Handler {
	panic("TODO: implement")
}

// SetDraining switches /readyz to 503 "draining" for good, so that load
// balancers stop sending new traffic before the server shuts down. /livez is
// not affected. SetDraining never waits for a /readyz run in progress.
func (h *Health) SetDraining() {
	panic("TODO: implement")
}
