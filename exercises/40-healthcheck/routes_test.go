package healthcheck_test

// Step 1: Register and the routes.
//
//	go test -run 'TestRegister|TestRoutes' -v ./exercises/40-healthcheck/

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	healthcheck "github.com/yornik/go-practice/exercises/40-healthcheck"
)

func TestRegisterValid(t *testing.T) {
	h := healthcheck.New(healthcheck.Options{})
	for _, c := range []healthcheck.Check{
		check("postgres", true, nil, nil),
		check("search", false, nil, nil),
		{Name: "redis", Timeout: time.Nanosecond, Fn: func(context.Context) error { return nil }},
	} {
		if err := h.Register(c); err != nil {
			t.Errorf("Register(check %q) = %v, want nil", c.Name, err)
		}
	}
}

func TestRegisterInvalid(t *testing.T) {
	ok := func(context.Context) error { return nil }
	tests := []struct {
		name  string
		check healthcheck.Check
	}{
		{"empty Name", healthcheck.Check{Name: "", Timeout: time.Second, Fn: ok}},
		{"nil Fn", healthcheck.Check{Name: "postgres", Timeout: time.Second}},
		{"zero Timeout", healthcheck.Check{Name: "postgres", Fn: ok}},
		{"negative Timeout", healthcheck.Check{Name: "postgres", Timeout: -time.Second, Fn: ok}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := healthcheck.New(healthcheck.Options{})
			if err := h.Register(tt.check); !errors.Is(err, healthcheck.ErrInvalidCheck) {
				t.Errorf("Register(check with %s) = %v, want an error that matches ErrInvalidCheck", tt.name, err)
			}
		})
	}
}

func TestRegisterDuplicate(t *testing.T) {
	h := healthcheck.New(healthcheck.Options{})
	if err := h.Register(check("postgres", true, nil, nil)); err != nil {
		t.Fatalf("Register(check %q) = %v, want nil", "postgres", err)
	}
	// The same name again, critical or not, is a configuration bug.
	for _, critical := range []bool{true, false} {
		if err := h.Register(check("postgres", critical, nil, nil)); !errors.Is(err, healthcheck.ErrDuplicateCheck) {
			t.Errorf("second Register(check %q, Critical: %v) = %v, want an error that matches ErrDuplicateCheck",
				"postgres", critical, err)
		}
	}
	// Other names are still fine after a rejected one.
	if err := h.Register(check("redis", true, nil, nil)); err != nil {
		t.Errorf("Register(check %q) after a rejected duplicate = %v, want nil", "redis", err)
	}
}

func TestRoutesLivez(t *testing.T) {
	// A critical check that fails: liveness must not care.
	var calls atomic.Int32
	h := newHealth(t, healthcheck.Options{}, check("postgres", true, errors.New("connection refused"), &calls))

	resp := do(t, h.Handler(), http.MethodGet, "/livez")
	if resp.Code != http.StatusOK || strings.TrimSpace(resp.Body) != "ok" {
		t.Errorf("GET /livez = %d %q, want 200 \"ok\"", resp.Code, resp.Body)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("GET /livez ran a check %d times, want 0: liveness must not depend on any dependency", n)
	}
}

func TestRoutesMethodNotAllowed(t *testing.T) {
	var calls atomic.Int32
	h := newHealth(t, healthcheck.Options{}, check("postgres", true, nil, &calls))
	handler := h.Handler()

	for _, path := range []string{"/livez", "/readyz"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			t.Run(method+" "+path, func(t *testing.T) {
				resp := do(t, handler, method, path)
				if resp.Code != http.StatusMethodNotAllowed {
					t.Errorf("%s %s = %d, want 405", method, path, resp.Code)
				}
				if allow := resp.Header.Get("Allow"); !strings.Contains(allow, http.MethodGet) {
					t.Errorf("%s %s: Allow header = %q, want a list that contains GET", method, path, allow)
				}
			})
		}
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("requests with a wrong method ran a check %d times, want 0", n)
	}
}

func TestRoutesNotFound(t *testing.T) {
	h := newHealth(t, healthcheck.Options{})
	handler := h.Handler()
	for _, path := range []string{"/", "/healthz", "/metrics"} {
		if resp := do(t, handler, http.MethodGet, path); resp.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.Code)
		}
	}
}
