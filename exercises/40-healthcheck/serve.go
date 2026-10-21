package healthcheck

import (
	"context"
	"net"
	"net/http"
	"time"
)

// ServeOptions configures the shutdown sequence of Serve.
type ServeOptions struct {
	// DrainDelay is how long Serve keeps serving after SetDraining, before it
	// stops accepting connections. It gives load balancers time to notice
	// that the pod is going away.
	DrainDelay time.Duration

	// ShutdownTimeout is how long Serve waits for in-flight requests to
	// finish once it stops accepting connections.
	ShutdownTimeout time.Duration

	// Sleep waits for the given duration. Serve calls Sleep(DrainDelay) once
	// during shutdown. nil means time.Sleep. Tests inject a fake.
	Sleep func(d time.Duration)
}

// Serve runs srv on ln until ctx is done, then shuts down in this order:
// h.SetDraining(), Sleep(DrainDelay) while srv keeps serving, then
// srv.Shutdown, which stops accepting connections and waits up to
// ShutdownTimeout for in-flight requests to finish.
//
// Serve returns nil after a clean shutdown. If the in-flight requests do not
// finish within ShutdownTimeout, Serve closes the remaining connections with
// srv.Close and returns an error that matches context.DeadlineExceeded. If
// srv.Serve fails before ctx is done, Serve returns that error (wrapped with
// %w if you add context) without waiting for ctx.
//
// Serve does not set srv.Handler: the caller mounts h.Handler() where it
// wants it.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, h *Health, o ServeOptions) error {
	panic("TODO: implement")
}
