package slo_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	slo "github.com/yornik/go-practice/exercises/30-slo"
)

func TestDefaultRules(t *testing.T) {
	defer catchPanic(t)
	want := []slo.Rule{
		{Name: "1h/5m", Severity: "page", Long: time.Hour, Short: 5 * time.Minute, Factor: 14.4},
		{Name: "6h/30m", Severity: "page", Long: 6 * time.Hour, Short: 30 * time.Minute, Factor: 6},
		{Name: "1d/2h", Severity: "ticket", Long: 24 * time.Hour, Short: 2 * time.Hour, Factor: 3},
		{Name: "3d/6h", Severity: "ticket", Long: 72 * time.Hour, Short: 6 * time.Hour, Factor: 1},
	}
	got := slo.DefaultRules()
	if !slices.Equal(got, want) {
		t.Fatalf("DefaultRules() =\n %+v\nwant\n %+v", got, want)
	}

	// A caller tunes the rules for one SLO. That must not change the rules
	// that the next caller gets.
	got[0].Factor = 20
	got[3].Severity = "page"
	// Undo the change at the end, so that a shared slice fails only this
	// test and not TestEvaluateIncidentReplay as well.
	defer func() { got[0].Factor, got[3].Severity = 14.4, "ticket" }()
	if again := slo.DefaultRules(); !slices.Equal(again, want) {
		t.Errorf("after a caller changed its result, DefaultRules() =\n %+v\nwant\n %+v\n(every call must return a new slice)", again, want)
	}
}

// fast is a rule like the 1h/5m page, used alone so each case checks one thing.
var fast = slo.Rule{Name: "fast", Severity: "page", Long: time.Hour, Short: 5 * time.Minute, Factor: 14.4}

func TestEvaluateBothWindows(t *testing.T) {
	cases := []struct {
		name    string
		buckets []slo.Bucket
		want    []slo.Alert
	}{
		{
			name:    "healthy: nothing fires",
			buckets: minutes(t0.Add(-time.Hour), 60, 1000, 0),
		},
		{
			// Short window: 500 bad / 5000 = burn 100. Long window: 500 bad /
			// 60000 = burn 8.3. Too small to be worth a page.
			name: "a 1-minute spike: only the short window is high",
			buckets: slices.Concat(
				minutes(t0.Add(-time.Hour), 59, 1000, 0),
				minutes(t0.Add(-time.Minute), 1, 1000, 500),
			),
		},
		{
			// Long window: 2000 bad / 60000 = burn 33.3. Short window: burn 0.
			// The problem is over, so nobody needs to wake up.
			name: "recovered 10 minutes ago: only the long window is high",
			buckets: slices.Concat(
				minutes(t0.Add(-time.Hour), 30, 1000, 0),
				minutes(t0.Add(-30*time.Minute), 20, 1000, 100),
				minutes(t0.Add(-10*time.Minute), 10, 1000, 0),
			),
		},
		{
			// Long window: 2000 bad / 60000 = burn 33.3. Short window: 500 bad
			// / 5000 = burn 100.
			name: "ongoing: both windows are high",
			buckets: slices.Concat(
				minutes(t0.Add(-time.Hour), 40, 1000, 0),
				minutes(t0.Add(-20*time.Minute), 20, 1000, 100),
			),
			want: []slo.Alert{{Rule: "fast", Severity: "page", LongBurn: 100.0 / 3, ShortBurn: 100}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			got := slo999.Evaluate(tc.buckets, t0, []slo.Rule{fast})
			checkAlerts(t, got, tc.want)
		})
	}
}

func TestEvaluateThreshold(t *testing.T) {
	// A rule with Factor 1 fires when the error ratio reaches the budget,
	// 0.1 %. Both windows contain the same single bucket.
	slow := slo.Rule{Name: "slow", Severity: "ticket", Long: time.Hour, Short: 5 * time.Minute, Factor: 1}
	cases := []struct {
		name string
		bad  int64 // of 1 000 000 requests
		want []slo.Alert
	}{
		// On paper 1000 / 1000000 / (1 - 0.999) is exactly 1. In float64 it
		// is 0.9999999999999991, so this needs the tolerance from the README.
		{"burn rate exactly at Factor fires", 1000, []slo.Alert{{Rule: "slow", Severity: "ticket", LongBurn: 1, ShortBurn: 1}}},
		{"burn rate 0.1 % below Factor does not fire", 999, nil},
		{"burn rate above Factor fires", 1001, []slo.Alert{{Rule: "slow", Severity: "ticket", LongBurn: 1.001, ShortBurn: 1.001}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			buckets := []slo.Bucket{bucket(t0.Add(-time.Minute), 1_000_000, tc.bad)}
			got := slo999.Evaluate(buckets, t0, []slo.Rule{slow})
			checkAlerts(t, got, tc.want)
		})
	}
}

func TestEvaluateRuleOrder(t *testing.T) {
	// The last 20 minutes fail 10 %: burn 33.3 over 1h, 100 over 5m.
	buckets := slices.Concat(
		minutes(t0.Add(-time.Hour), 40, 1000, 0),
		minutes(t0.Add(-20*time.Minute), 20, 1000, 100),
	)
	slow := slo.Rule{Name: "slow", Severity: "ticket", Long: time.Hour, Short: 5 * time.Minute, Factor: 1}
	never := slo.Rule{Name: "never", Severity: "page", Long: time.Hour, Short: 5 * time.Minute, Factor: 500}
	fastAlert := slo.Alert{Rule: "fast", Severity: "page", LongBurn: 100.0 / 3, ShortBurn: 100}
	slowAlert := slo.Alert{Rule: "slow", Severity: "ticket", LongBurn: 100.0 / 3, ShortBurn: 100}

	cases := []struct {
		name  string
		rules []slo.Rule
		want  []slo.Alert
	}{
		{"no rules: no alerts", nil, nil},
		{"alerts in the order of the rules", []slo.Rule{slow, fast}, []slo.Alert{slowAlert, fastAlert}},
		{"the other order", []slo.Rule{fast, slow}, []slo.Alert{fastAlert, slowAlert}},
		{"a rule that does not fire is skipped", []slo.Rule{fast, never, slow}, []slo.Alert{fastAlert, slowAlert}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			got := slo999.Evaluate(buckets, t0, tc.rules)
			checkAlerts(t, got, tc.want)
		})
	}
}

// TestEvaluateIncidentReplay replays an incident minute by minute with the
// default rules, the way you would backtest alert rules on yesterday's data.
//
// Traffic is 1000 requests per minute for 3 days before t0 and 90 minutes
// after it. The incident: from t0 to t0+30m, 100 of 1000 requests fail (10 %,
// burn rate 100). The README works out by hand when each rule fires:
//
//	1h/5m  page  fires from t0+9m  (9 bad minutes in the last hour: burn 15)
//	             until t0+35m      (the last bad minute leaves the 5m window)
//	6h/30m page  fires from t0+22m (22 bad minutes in the last 6h: burn 6.1)
//	             until t0+59m      (fewer than 2 bad minutes in the 30m window)
//	tickets      never fire        (3000 bad requests are too few for 1d and 3d)
//
// The buckets are newest first, and the slice also holds the buckets after
// now, as a replay does: only the buckets before now may count.
func TestEvaluateIncidentReplay(t *testing.T) {
	defer catchPanic(t)
	buckets := slices.Concat(
		minutes(t0.Add(-3*day), 3*1440, 1000, 0),
		minutes(t0, 30, 1000, 100),
		minutes(t0.Add(30*time.Minute), 60, 1000, 0),
	)
	slices.Reverse(buckets)
	before := slices.Clone(buckets)

	expected := func(m int) []string {
		var names []string
		if m >= 9 && m < 35 {
			names = append(names, "1h/5m")
		}
		if m >= 22 && m < 59 {
			names = append(names, "6h/30m")
		}
		return names
	}

	mismatches := 0
	for m := -10; m <= 90; m++ {
		now := t0.Add(time.Duration(m) * time.Minute)
		var got []string
		for _, a := range slo999.Evaluate(buckets, now, slo.DefaultRules()) {
			got = append(got, a.Rule)
		}
		if want := expected(m); !slices.Equal(got, want) {
			t.Errorf("at t0%+dm: rules that fired = %q, want %q", m, got, want)
			if mismatches++; mismatches == 5 {
				t.Fatal("stopping after 5 wrong minutes")
			}
		}
	}
	checkUnchanged(t, buckets, before)
}

// checkAlerts compares alerts field by field, the burn rates with a tolerance.
// A nil and an empty slice count as equal.
func checkAlerts(t *testing.T, got, want []slo.Alert) {
	t.Helper()
	ok := len(got) == len(want)
	for i := 0; ok && i < len(got); i++ {
		g, w := got[i], want[i]
		ok = g.Rule == w.Rule && g.Severity == w.Severity &&
			approxEqual(g.LongBurn, w.LongBurn) && approxEqual(g.ShortBurn, w.ShortBurn)
	}
	if !ok {
		t.Errorf("Evaluate() =\n %s\nwant\n %s", formatAlerts(got), formatAlerts(want))
	}
}

func formatAlerts(alerts []slo.Alert) string {
	if len(alerts) == 0 {
		return "no alerts"
	}
	s := ""
	for _, a := range alerts {
		s += fmt.Sprintf("{Rule: %q, Severity: %q, LongBurn: %.4f, ShortBurn: %.4f} ", a.Rule, a.Severity, a.LongBurn, a.ShortBurn)
	}
	return s
}
