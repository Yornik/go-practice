package alertstate_test

import (
	"fmt"
	"testing"
	"time"

	alertstate "github.com/yornik/go-practice/exercises/22-alertstate"
)

// Step 4: a whole night, a clock that moves backwards, and History.

// TestEvalScenariosSpikesAllNight evaluates every 30 s for 8 h, 960
// evaluations from t0 on. The condition is true when the time since t0,
// modulo 10 minutes, is under 40 s: at 0 s and 30 s of every 10 minutes.
func TestEvalScenariosSpikesAllNight(t *testing.T) {
	cases := []struct {
		rule            alertstate.Rule
		wantTransitions int
		wantFired       int // transitions to Firing
		wantResolved    int // transitions from Firing to Inactive
		wantFinal       alertstate.State
	}{
		{alertstate.Rule{For: 2 * time.Minute}, 96, 0, 0, inactive},
		{alertstate.Rule{}, 96, 48, 48, inactive},
		{alertstate.Rule{KeepFiringFor: 10 * time.Minute}, 1, 1, 0, firing},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%+v", tc.rule), func(t *testing.T) {
			defer guard(t)()
			clock := newClock()
			a, err := alertstate.New(tc.rule, clock.Now)
			if err != nil || a == nil {
				t.Fatalf("New(%+v, clock) = %v, %v; want an Alert and nil", tc.rule, a, err)
			}
			const every, night, cycle, visit = 30 * time.Second, 8 * time.Hour, 10 * time.Minute, 40 * time.Second
			var got alertstate.State
			for d := time.Duration(0); d < night; d += every {
				got = a.Eval(d%cycle < visit) // the clock shows t0+d
				clock.Advance(every)
			}
			if got != tc.wantFinal {
				t.Errorf("rule %+v: the last Eval returned %v, want %v", tc.rule, got, tc.wantFinal)
			}
			history := a.History()
			fired, resolved := 0, 0
			for _, x := range history {
				if x.To == firing {
					fired++
				}
				if x.From == firing && x.To == inactive {
					resolved++
				}
			}
			if len(history) != tc.wantTransitions || fired != tc.wantFired || resolved != tc.wantResolved {
				t.Errorf("rule %+v: History() has %d transitions, %d to firing, %d from firing to inactive; want %d, %d, %d",
					tc.rule, len(history), fired, resolved, tc.wantTransitions, tc.wantFired, tc.wantResolved)
			}
		})
	}
}

func TestEvalScenariosClockBackwards(t *testing.T) {
	runScripts(t, []scriptCase{
		{"an earlier evaluation does not start Pending", alertstate.Rule{For: 2 * minute}, []step{
			{0, false, inactive},
			{-1 * minute, true, inactive}, // ignored
		}},
		{"an earlier evaluation does not resolve", alertstate.Rule{}, []step{
			{0, true, firing},
			{-1 * minute, false, firing}, // ignored
		}},
		{"an earlier evaluation does not end Pending", alertstate.Rule{For: 2 * minute}, []step{
			{0, true, pending},
			{-1 * minute, false, pending}, // ignored
			{1 * minute, true, pending},   // t0 again: the same time is not earlier
			{2 * minute, true, firing},
		}},
		{"an ignored evaluation does not move the latest evaluation back", alertstate.Rule{}, []step{
			{0, true, firing},
			{-1 * minute, true, firing},  // t0-1m: ignored
			{30 * second, false, firing}, // t0-30s: still before t0, ignored
			{30 * second, false, inactive},
		}},
		{"an ignored true evaluation does not restart KeepFiringFor", alertstate.Rule{KeepFiringFor: 5 * minute}, []step{
			{0, true, firing},
			{4 * minute, false, firing},
			{-2 * minute, true, firing},   // t0+2m: ignored
			{3 * minute, false, inactive}, // t0+5m: 5m after the last true evaluation at t0
		}},
	})

	t.Run("an ignored evaluation adds no history", func(t *testing.T) {
		defer guard(t)()
		h := newHarness(t, alertstate.Rule{})
		h.eval(0, true, firing)
		h.eval(-1*minute, false, firing)
		want := []alertstate.Transition{tr(0, inactive, firing)}
		if got := h.a.History(); !sameHistory(got, want) {
			t.Errorf("History() = %s, want %s", formatHistory(got), formatHistory(want))
		}
		h.activeAt(0)
	})
}

func TestEvalScenariosHistory(t *testing.T) {
	defer guard(t)()
	h := newHarness(t, alertstate.Rule{For: 1 * minute, KeepFiringFor: 2 * minute})
	h.eval(0, false, inactive)
	h.eval(1*minute, true, pending)
	h.eval(1*minute, true, firing)
	h.eval(1*minute, false, firing)
	h.eval(1*minute, false, inactive)
	h.eval(1*minute, true, pending)
	h.eval(30*second, false, inactive)

	want := []alertstate.Transition{
		tr(1*minute, inactive, pending),
		tr(2*minute, pending, firing),
		tr(4*minute, firing, inactive),
		tr(5*minute, inactive, pending),
		tr(5*minute+30*second, pending, inactive),
	}
	got := h.a.History()
	if !sameHistory(got, want) {
		t.Fatalf("History() = %s, want %s", formatHistory(got), formatHistory(want))
	}

	// The caller owns the returned slice.
	got[0].To = firing
	got[0].At = t0.Add(time.Hour)
	_ = append(got[:1], tr(time.Hour, firing, pending))
	if again := h.a.History(); !sameHistory(again, want) {
		t.Errorf("after the caller changed the slice that History() returned, History() = %s, want %s (return a copy)",
			formatHistory(again), formatHistory(want))
	}

	t.Run("For 0 is 1 transition", func(t *testing.T) {
		defer guard(t)()
		h := newHarness(t, alertstate.Rule{})
		h.eval(10*second, true, firing)
		h.eval(10*second, false, inactive)
		want := []alertstate.Transition{
			tr(10*second, inactive, firing),
			tr(20*second, firing, inactive),
		}
		if got := h.a.History(); !sameHistory(got, want) {
			t.Errorf("History() = %s, want %s (Inactive to Firing is 1 transition, without Pending)",
				formatHistory(got), formatHistory(want))
		}
	})
}

// TestEvalScenariosReadmeExample runs the first example of the README.
func TestEvalScenariosReadmeExample(t *testing.T) {
	defer guard(t)()
	h := newHarness(t, alertstate.Rule{For: 2 * minute, KeepFiringFor: 5 * minute})
	h.eval(0, false, inactive) // 00:00
	h.noActiveAt()
	h.eval(30*second, true, pending) // 00:30
	h.activeAt(30 * second)
	h.eval(1*minute, true, pending)           // 01:30
	h.eval(1*minute, true, firing)            // 02:30
	h.eval(30*second, false, firing)          // 03:00
	h.eval(4*minute, true, firing)            // 07:00
	h.eval(4*minute+59*second, false, firing) // 11:59
	h.activeAt(30 * second)
	h.eval(1*second, false, inactive) // 12:00
	h.noActiveAt()
	h.eval(-1*minute, true, inactive) // 11:00: ignored
	h.noActiveAt()
	h.eval(1*minute+30*second, true, pending) // 12:30
	h.activeAt(12*minute + 30*second)

	want := []alertstate.Transition{
		tr(30*second, inactive, pending),
		tr(2*minute+30*second, pending, firing),
		tr(12*minute, firing, inactive),
		tr(12*minute+30*second, inactive, pending),
	}
	if got := h.a.History(); !sameHistory(got, want) {
		t.Errorf("History() = %s, want %s", formatHistory(got), formatHistory(want))
	}
}
