package healthcheck_test

// Step 2: the readiness report.
//
//	go test -run TestReport -v ./exercises/15-healthcheck/

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	healthcheck "github.com/yornik/go-practice/exercises/15-healthcheck"
)

func TestReportAllOK(t *testing.T) {
	h := newHealth(t, healthcheck.Options{},
		check("postgres", true, nil, nil),
		check("cache", false, nil, nil),
	)
	resp, rep := getReadyz(t, h.Handler())
	wantReadiness(t, resp, rep, http.StatusOK, "ok")
	for _, name := range []string{"cache", "postgres"} {
		if c := findCheck(t, rep, name); c.Status != "ok" || c.Error != "" {
			t.Errorf("check %q: status %q, error %q; want status \"ok\" and no error", name, c.Status, c.Error)
		}
	}
}

func TestReportNoChecks(t *testing.T) {
	h := newHealth(t, healthcheck.Options{})
	resp, rep := getReadyz(t, h.Handler()) // also checks that "checks" is [], not null
	wantReadiness(t, resp, rep, http.StatusOK, "ok")
	if len(rep.Checks) != 0 {
		t.Errorf("checks = %+v, want []", rep.Checks)
	}
}

func TestReportSortedByName(t *testing.T) {
	h := newHealth(t, healthcheck.Options{},
		check("search", false, nil, nil),
		check("postgres", true, nil, nil),
		check("cache", false, nil, nil),
		check("auth-api", true, nil, nil),
		check("kafka", true, nil, nil),
	)
	_, rep := getReadyz(t, h.Handler())
	want := []string{"auth-api", "cache", "kafka", "postgres", "search"}
	if got := names(rep); !slices.Equal(got, want) {
		t.Errorf("check names in the report = %q, want %q (sorted by name)", got, want)
	}
}

// TestReportStatus runs the decision table of the README: critical checks
// decide the status, non-critical ones are only reported. Each failing check
// must show status "fail" and its error text.
func TestReportStatus(t *testing.T) {
	refused := errors.New("dial tcp 10.0.3.7:5432: connect: connection refused")
	wrapped := fmt.Errorf("ping kafka: %w", errors.New("i/o timeout"))

	type spec struct {
		name     string
		critical bool
		err      error
	}
	tests := []struct {
		name       string
		checks     []spec
		wantCode   int
		wantStatus string
	}{
		{"a critical check fails",
			[]spec{{"cache", true, nil}, {"postgres", true, refused}},
			http.StatusServiceUnavailable, "fail"},
		{"a non-critical check fails",
			[]spec{{"postgres", true, nil}, {"search", false, refused}},
			http.StatusOK, "ok"},
		{"only non-critical checks, all fail",
			[]spec{{"recommendations", false, refused}, {"search", false, wrapped}},
			http.StatusOK, "ok"},
		{"one of three critical checks fails",
			[]spec{{"auth-api", true, nil}, {"kafka", true, wrapped}, {"postgres", true, nil}},
			http.StatusServiceUnavailable, "fail"},
		{"critical and non-critical checks fail",
			[]spec{{"postgres", true, refused}, {"search", false, wrapped}},
			http.StatusServiceUnavailable, "fail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := healthcheck.New(healthcheck.Options{})
			for _, s := range tt.checks {
				if err := h.Register(check(s.name, s.critical, s.err, nil)); err != nil {
					t.Fatalf("Register(check %q) = %v, want nil", s.name, err)
				}
			}
			resp, rep := getReadyz(t, h.Handler())
			wantReadiness(t, resp, rep, tt.wantCode, tt.wantStatus)
			for _, s := range tt.checks {
				c := findCheck(t, rep, s.name)
				wantStatus, wantErr := "ok", ""
				if s.err != nil {
					wantStatus, wantErr = "fail", s.err.Error()
				}
				if c.Status != wantStatus || c.Error != wantErr {
					t.Errorf("check %q: status %q, error %q; want status %q, error %q",
						s.name, c.Status, c.Error, wantStatus, wantErr)
				}
			}
		})
	}
}

// TestReportDuration uses a fake clock: the check advances it to pretend that
// it took a while, so duration_ms is exact.
func TestReportDuration(t *testing.T) {
	tests := []struct {
		name string
		took time.Duration
		err  error
		want int64
	}{
		{"instant", 0, nil, 0},
		{"1.5s", 1500 * time.Millisecond, nil, 1500},
		{"2.25s", 2*time.Second + 250*time.Millisecond, nil, 2250},
		{"failure after 300ms", 300 * time.Millisecond, errors.New("connection refused"), 300}, // failures have a duration too
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newFakeClock()
			h := newHealth(t, healthcheck.Options{Now: clock.Now}, healthcheck.Check{
				Name: "postgres", Critical: true, Timeout: time.Minute,
				Fn: func(context.Context) error {
					clock.Advance(tt.took)
					return tt.err
				},
			})
			_, rep := getReadyz(t, h.Handler())
			c := findCheck(t, rep, "postgres")
			if c.DurationMS != nil && *c.DurationMS != tt.want {
				t.Errorf("duration_ms = %d, want %d (measured with Options.Now)", *c.DurationMS, tt.want)
			}
		})
	}
}

func TestReportRejectedChecksNotListed(t *testing.T) {
	h := newHealth(t, healthcheck.Options{}, check("postgres", true, nil, nil))
	// Both are rejected; neither may show up in the report or change it.
	if err := h.Register(check("postgres", true, errors.New("I am the duplicate"), nil)); err == nil {
		t.Errorf("Register(duplicate check %q) = nil, want an error", "postgres")
	}
	if err := h.Register(healthcheck.Check{Name: "no-fn", Critical: true, Timeout: time.Second}); err == nil {
		t.Errorf("Register(check %q without Fn) = nil, want an error", "no-fn")
	}

	resp, rep := getReadyz(t, h.Handler())
	wantReadiness(t, resp, rep, http.StatusOK, "ok")
	if got, want := names(rep), []string{"postgres"}; !slices.Equal(got, want) {
		t.Errorf("check names in the report = %q, want %q", got, want)
	}
}
