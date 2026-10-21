package alertstate_test

import (
	"testing"
	"time"

	alertstate "github.com/yornik/go-practice/exercises/22-alertstate"
)

const (
	second = time.Second
	minute = time.Minute
)

// Step 2: Inactive, Pending and the wait of For.

func TestEvalFor(t *testing.T) {
	for2m := alertstate.Rule{For: 2 * minute}
	runScripts(t, []scriptCase{
		{"inactive while the condition is false", for2m, []step{
			{0, false, inactive},
			{30 * second, false, inactive},
			{30 * second, false, inactive},
		}},
		{"fires exactly at For", for2m, []step{
			{0, true, pending},
			{1 * minute, true, pending},
			{1 * minute, true, firing}, // 2m after the start of Pending
		}},
		{"1 s before For is still pending", for2m, []step{
			{0, true, pending},
			{1*minute + 59*second, true, pending},
			{1 * second, true, firing},
		}},
		{"For counts from the first true evaluation, not from t0", for2m, []step{
			{0, false, inactive},
			{1 * minute, true, pending}, // Pending starts here, at t0+1m
			{1 * minute, true, pending}, // only 1m later
			{1 * minute, true, firing},
		}},
		{"a false evaluation ends Pending", for2m, []step{
			{0, true, pending},
			{1 * minute, false, inactive},
		}},
		{"an interrupted Pending starts over", for2m, []step{
			{0, true, pending},
			{1 * minute, false, inactive},
			{1 * minute, true, pending}, // a new Pending starts at t0+2m
			{1*minute + 59*second, true, pending},
			{1 * second, true, firing},
		}},
		{"Pending fires only on a true evaluation", for2m, []step{
			{0, true, pending},
			{5 * minute, false, inactive}, // For has passed, but the condition is false now
		}},
		{"a long gap between true evaluations fires at once", for2m, []step{
			{0, true, pending},
			{10 * minute, true, firing},
		}},
		{"Firing stays Firing while the condition is true", for2m, []step{
			{0, true, pending},
			{2 * minute, true, firing},
			{30 * second, true, firing},
			{10 * minute, true, firing},
		}},
		{"For 0 fires at the first true evaluation", alertstate.Rule{}, []step{
			{0, false, inactive},
			{30 * second, true, firing},
			{30 * second, true, firing},
		}},
		{"For 1ns is pending first", alertstate.Rule{For: time.Nanosecond}, []step{
			{0, true, pending},
			{time.Nanosecond, true, firing},
		}},
	})
}

func TestEvalForActiveAt(t *testing.T) {
	t.Run("For 2m", func(t *testing.T) {
		defer guard(t)()
		h := newHarness(t, alertstate.Rule{For: 2 * minute})
		h.eval(0, false, inactive)
		h.noActiveAt()
		h.eval(30*second, true, pending)
		h.activeAt(30 * second)
		h.eval(1*minute, true, pending)
		h.activeAt(30 * second)
		h.eval(1*minute, true, firing)
		h.activeAt(30 * second) // Firing keeps the start of the episode
		h.eval(1*minute, true, firing)
		h.activeAt(30 * second)
	})
	t.Run("an interrupted Pending", func(t *testing.T) {
		defer guard(t)()
		h := newHarness(t, alertstate.Rule{For: 2 * minute})
		h.eval(0, true, pending)
		h.activeAt(0)
		h.eval(1*minute, false, inactive)
		h.noActiveAt()
		h.eval(1*minute, true, pending)
		h.activeAt(2 * minute)
		h.eval(2*minute, true, firing)
		h.activeAt(2 * minute)
	})
	t.Run("For 0", func(t *testing.T) {
		defer guard(t)()
		h := newHarness(t, alertstate.Rule{})
		h.eval(0, false, inactive)
		h.noActiveAt()
		h.eval(45*second, true, firing)
		h.activeAt(45 * second)
	})
}

func TestEvalForNilClock(t *testing.T) {
	defer guard(t)()
	// With a nil clock, New uses time.Now. The test does not read the clock
	// itself: it only checks that Eval uses some clock.
	a, err := alertstate.New(alertstate.Rule{}, nil)
	if err != nil || a == nil {
		t.Fatalf("New({}, nil) = %v, %v; want an Alert and nil", a, err)
	}
	if got := a.Eval(true); got != firing {
		t.Errorf("New({}, nil): Eval(true) = %v, want firing", got)
	}
	if got := a.ActiveAt(); got.IsZero() {
		t.Errorf("New({}, nil): after Eval(true), ActiveAt() is the zero time, want the time of the evaluation")
	}
}

// Step 3: Firing and the wait of KeepFiringFor.

func TestEvalKeepFiring(t *testing.T) {
	keep5m := alertstate.Rule{KeepFiringFor: 5 * minute}
	runScripts(t, []scriptCase{
		{"KeepFiringFor 0 resolves at the first false evaluation", alertstate.Rule{}, []step{
			{0, true, firing},
			{30 * second, false, inactive},
		}},
		{"resolves exactly KeepFiringFor after the last true evaluation", keep5m, []step{
			{0, true, firing},
			{1 * minute, true, firing}, // the last true evaluation: t0+1m
			{1 * minute, false, firing},
			{4 * minute, false, inactive}, // t0+6m: 5m after t0+1m
		}},
		{"1 s before KeepFiringFor is still firing", keep5m, []step{
			{0, true, firing},
			{4*minute + 59*second, false, firing},
			{1 * second, false, inactive},
		}},
		{"a true evaluation restarts the wait", keep5m, []step{
			{0, true, firing},
			{30 * second, false, firing},
			{2 * minute, false, firing},
			{1 * minute, true, firing}, // the condition is back at t0+3m30s
			{4*minute + 59*second, false, firing},
			{1 * second, false, inactive}, // t0+8m30s: 5m after t0+3m30s
		}},
		{"a long gap after the last true evaluation resolves at once", keep5m, []step{
			{0, true, firing},
			{10 * minute, false, inactive},
		}},
		{"KeepFiringFor counts from the last true evaluation of Pending too", alertstate.Rule{For: 1 * minute, KeepFiringFor: 5 * minute}, []step{
			{0, true, pending},
			{1 * minute, true, firing}, // the last true evaluation: t0+1m
			{4 * minute, false, firing},
			{1 * minute, false, inactive}, // t0+6m
		}},
		{"a return inside KeepFiringFor does not wait for For again", alertstate.Rule{For: 2 * minute, KeepFiringFor: 5 * minute}, []step{
			{0, true, pending},
			{2 * minute, true, firing},
			{1 * minute, false, firing},
			{1 * minute, true, firing}, // still Firing: no new Pending
		}},
		{"after it resolved, a new episode waits for For again", alertstate.Rule{For: 2 * minute, KeepFiringFor: 1 * minute}, []step{
			{0, true, pending},
			{2 * minute, true, firing},
			{30 * second, false, firing},
			{30 * second, false, inactive},
			{30 * second, true, pending},
			{2 * minute, true, firing},
		}},
	})
}

func TestEvalKeepFiringActiveAt(t *testing.T) {
	defer guard(t)()
	h := newHarness(t, alertstate.Rule{For: 1 * minute, KeepFiringFor: 5 * minute})
	h.eval(0, true, pending)
	h.activeAt(0)
	h.eval(1*minute, true, firing)
	h.activeAt(0)
	h.eval(1*minute, false, firing)
	h.activeAt(0) // still the same episode while it keeps firing
	h.eval(4*minute, false, inactive)
	h.noActiveAt()
	h.eval(1*minute, true, pending)
	h.activeAt(7 * minute)
}
