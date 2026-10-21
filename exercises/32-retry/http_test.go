package retry_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	retry "github.com/yornik/go-practice/exercises/32-retry"
)

// This file shows how the pieces fit together in real code: an op that calls
// an HTTP API and turns the response into the errors Do understands. The op
// is written here, in the test; you write nothing new for it. It passes once
// ParseRetryAfter, the error types and DoValue work.

// getStatus returns an op for DoValue that GETs url and returns the body.
func getStatus(client *http.Client, url string, now time.Time) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", retry.Permanent(err) // a bad URL will not get better
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", err // network errors are worth another try
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return "", err
		}

		switch code := resp.StatusCode; {
		case code == http.StatusOK:
			return string(body), nil
		case code == http.StatusTooManyRequests || code == http.StatusServiceUnavailable:
			err := fmt.Errorf("GET %s: %s", req.URL.Path, resp.Status)
			if d, ok := retry.ParseRetryAfter(resp.Header.Get("Retry-After"), now); ok {
				return "", &retry.RetryAfterError{Err: err, After: d}
			}
			return "", err
		case code >= 500:
			return "", fmt.Errorf("GET %s: %s", req.URL.Path, resp.Status)
		default:
			// Any other 4xx: our request is wrong. Retrying will not fix it.
			return "", retry.Permanent(fmt.Errorf("GET %s: %s", req.URL.Path, resp.Status))
		}
	}
}

// reply is one scripted HTTP response.
type reply struct {
	status     int
	retryAfter string // the Retry-After header; "" means none
	body       string
}

// scriptedServer answers request number i with replies[i].
func scriptedServer(t *testing.T, replies ...reply) (srv *httptest.Server, requests func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		i := n
		n++
		mu.Unlock()
		if i >= len(replies) {
			http.Error(w, "unexpected request", http.StatusTeapot)
			return
		}
		if replies[i].retryAfter != "" {
			w.Header().Set("Retry-After", replies[i].retryAfter)
		}
		w.WriteHeader(replies[i].status)
		_, _ = io.WriteString(w, replies[i].body)
	}))
	t.Cleanup(srv.Close)
	return srv, func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

func TestDoValueHTTP(t *testing.T) {
	now := time.Date(2026, 10, 21, 7, 27, 0, 0, time.UTC)
	p := retry.Policy{MaxAttempts: 5, Initial: 100 * ms, Max: 10 * time.Second, Multiplier: 2}

	t.Run("429 and 503 with Retry-After, then 500, then 200", func(t *testing.T) {
		srv, requests := scriptedServer(t,
			reply{status: http.StatusServiceUnavailable, retryAfter: "2"},
			reply{status: http.StatusTooManyRequests, retryAfter: "Wed, 21 Oct 2026 07:27:03 GMT"},
			reply{status: http.StatusInternalServerError},
			reply{status: http.StatusOK, body: "Healthy"},
		)
		s := &sleeper{}
		p := p
		p.Sleep = s.Sleep

		got, err := runDoValue(t, context.Background(), p, getStatus(srv.Client(), srv.URL+"/api/v1/status", now))
		if got != "Healthy" || err != nil {
			t.Errorf("DoValue = %q, %v; want %q, nil", got, err, "Healthy")
		}
		if n := requests(); n != 4 {
			t.Errorf("the server got %d requests, want 4", n)
		}
		// 2s and 3s as the server asked, then Backoff(3) = 400ms after the 500.
		checkDelays(t, s, 2*time.Second, 3*time.Second, 400*ms)
	})

	t.Run("404 is permanent", func(t *testing.T) {
		srv, requests := scriptedServer(t, reply{status: http.StatusNotFound})
		s := &sleeper{}
		p := p
		p.Sleep = s.Sleep

		got, err := runDoValue(t, context.Background(), p, getStatus(srv.Client(), srv.URL+"/api/v1/nope", now))
		want := "GET /api/v1/nope: 404 Not Found"
		if got != "" || err == nil || err.Error() != want {
			t.Errorf("DoValue = %q, %v; want \"\", %q", got, err, want)
		}
		var ex *retry.ExhaustedError
		if errors.As(err, &ex) {
			t.Errorf("DoValue returned an ExhaustedError; a permanent error must stop at once")
		}
		if n := requests(); n != 1 {
			t.Errorf("the server got %d requests, want 1", n)
		}
		checkDelays(t, s)
	})
}
