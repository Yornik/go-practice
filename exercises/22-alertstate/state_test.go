package alertstate_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	alertstate "github.com/yornik/go-practice/exercises/22-alertstate"
)

func TestStateString(t *testing.T) {
	defer guard(t)()
	cases := []struct {
		s    alertstate.State
		want string
	}{
		{alertstate.Inactive, "inactive"},
		{alertstate.Pending, "pending"},
		{alertstate.Firing, "firing"},
		{alertstate.State(-1), "State(-1)"},
		{alertstate.State(3), "State(3)"},
		{alertstate.State(42), "State(42)"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("State(%d).String() = %q, want %q", int(tc.s), got, tc.want)
		}
	}
	// fmt calls String for %v, so log lines and test messages show the name.
	if got := fmt.Sprint(alertstate.Firing); got != "firing" {
		t.Errorf("fmt.Sprint(Firing) = %q, want %q", got, "firing")
	}
}

func TestStateNew(t *testing.T) {
	valid := []alertstate.Rule{
		{},
		{For: 2 * time.Minute},
		{KeepFiringFor: 5 * time.Minute},
		{For: 2 * time.Minute, KeepFiringFor: 5 * time.Minute},
	}
	for _, rule := range valid {
		t.Run(fmt.Sprintf("valid %+v", rule), func(t *testing.T) {
			defer guard(t)()
			clock := newClock()
			a, err := alertstate.New(rule, clock.Now)
			if err != nil || a == nil {
				t.Fatalf("New(%+v, clock) = %v, %v; want an Alert and nil", rule, a, err)
			}
			if got := a.State(); got != alertstate.Inactive {
				t.Errorf("New(%+v, clock).State() = %v, want inactive", rule, got)
			}
			if got := a.ActiveAt(); !got.IsZero() {
				t.Errorf("New(%+v, clock).ActiveAt() = %v, want the zero time", rule, got)
			}
			if got := a.History(); len(got) != 0 {
				t.Errorf("New(%+v, clock).History() = %v, want no transitions", rule, formatHistory(got))
			}
		})
	}

	t.Run("nil clock", func(t *testing.T) {
		defer guard(t)()
		a, err := alertstate.New(alertstate.Rule{For: time.Minute}, nil)
		if err != nil || a == nil {
			t.Fatalf("New({For:1m0s}, nil) = %v, %v; want an Alert that uses time.Now, and nil", a, err)
		}
		if got := a.State(); got != alertstate.Inactive {
			t.Errorf("New({For:1m0s}, nil).State() = %v, want inactive", got)
		}
	})

	invalid := []alertstate.Rule{
		{For: -time.Second},
		{KeepFiringFor: -time.Nanosecond},
		{For: -time.Minute, KeepFiringFor: -time.Minute},
		{For: time.Minute, KeepFiringFor: -5 * time.Minute},
	}
	for _, rule := range invalid {
		t.Run(fmt.Sprintf("invalid %+v", rule), func(t *testing.T) {
			defer guard(t)()
			clock := newClock()
			a, err := alertstate.New(rule, clock.Now)
			if !errors.Is(err, alertstate.ErrInvalidRule) {
				t.Errorf("New(%+v, clock) error = %v, want an error that wraps ErrInvalidRule", rule, err)
			}
			if a != nil {
				t.Errorf("New(%+v, clock) returned a non-nil *Alert, want nil", rule)
			}
		})
	}
}
