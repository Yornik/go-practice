package ratelimit_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ratelimit "github.com/yornik/go-practice/exercises/08-ratelimit"
)

// limitByIP is the middleware a service puts in front of its handlers. It is
// part of the tests, not of the exercise: it shows how Keyed is used.
//
// The key is the client's IP without the port. r.RemoteAddr is "IP:port",
// and every new connection gets a new port: keyed by RemoteAddr, a client
// gets a fresh bucket per connection and the limit does nothing. Behind a
// load balancer or an Ingress, RemoteAddr is the proxy's address, and the
// client's IP has to come from a header that you trust, such as the last
// X-Forwarded-For entry that your own proxy added.
func limitByIP(k *ratelimit.Keyed, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !k.Allow(ip) {
			// At 1 token per second, the next token comes within 1s.
			w.Header().Set("Retry-After", "1")
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func TestKeyedHTTPMiddleware(t *testing.T) {
	defer guard(t)()
	clock := newClock()
	k := ratelimit.NewKeyed(1, 2, time.Minute, clock.Now)
	h := limitByIP(k, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "config v42")
	}))

	get := func(remoteAddr string, want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s: GET /v1/config from %s: status %d, want %d", clock.at(), remoteAddr, rec.Code, want)
		}
		if want == http.StatusTooManyRequests && rec.Header().Get("Retry-After") == "" {
			t.Errorf("%s: a 429 without a Retry-After header", clock.at())
		}
	}

	// The noisy CronJob: one IP, a new connection (port) per request.
	get("10.1.2.3:40001", http.StatusOK)
	get("10.1.2.3:40002", http.StatusOK)
	get("10.1.2.3:40003", http.StatusTooManyRequests)
	get("10.1.2.3:40004", http.StatusTooManyRequests)
	// Everybody else does not notice.
	get("10.9.8.7:51000", http.StatusOK)
	// One bucket per IP, not per connection.
	if got := k.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}

	clock.Advance(time.Second)
	get("10.1.2.3:40005", http.StatusOK)
	get("10.1.2.3:40006", http.StatusTooManyRequests)

	// The CronJob is fixed. A minute later both buckets are idle and full.
	clock.Advance(time.Minute)
	if got := k.Sweep(); got != 2 {
		t.Errorf("Sweep() = %d, want 2", got)
	}
}
