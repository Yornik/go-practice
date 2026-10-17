package slo_test

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	slo "github.com/yornik/go-practice/exercises/05-slo"
)

func TestValidate(t *testing.T) {
	valid := []slo.SLO{
		{Objective: 0.999, Period: month},
		{Objective: 0.99999, Period: 7 * day},
		{Objective: 0.5, Period: time.Hour},
	}
	for _, s := range valid {
		t.Run(fmt.Sprintf("valid %v over %v", s.Objective, s.Period), func(t *testing.T) {
			defer catchPanic(t)
			if err := s.Validate(); err != nil {
				t.Errorf("%+v.Validate() = %v, want nil", s, err)
			}
		})
	}

	invalid := []struct {
		name string
		s    slo.SLO
	}{
		{"zero value", slo.SLO{}},
		{"objective 0", slo.SLO{Objective: 0, Period: month}},
		{"objective 1 leaves no error budget", slo.SLO{Objective: 1, Period: month}},
		{"negative objective", slo.SLO{Objective: -0.5, Period: month}},
		// A classic config mistake: 99.9 % must be written as 0.999.
		{"percentage instead of a fraction", slo.SLO{Objective: 99.9, Period: month}},
		// strconv.ParseFloat accepts "NaN" and "Inf", so a config file can contain them.
		{"NaN objective", slo.SLO{Objective: math.NaN(), Period: month}},
		{"infinite objective", slo.SLO{Objective: math.Inf(1), Period: month}},
		{"zero period", slo.SLO{Objective: 0.999, Period: 0}},
		{"negative period", slo.SLO{Objective: 0.999, Period: -time.Hour}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			err := tc.s.Validate()
			if !errors.Is(err, slo.ErrInvalidSLO) {
				t.Errorf("%+v.Validate() = %v, want an error that wraps ErrInvalidSLO", tc.s, err)
			}
		})
	}
}

func TestErrorRatio(t *testing.T) {
	cases := []struct {
		name    string
		buckets []slo.Bucket
		window  time.Duration
		now     time.Time
		want    float64
	}{
		{
			name: "no buckets means no traffic: 0, not NaN",
			want: 0, window: 5 * time.Minute, now: t0,
		},
		{
			name:    "buckets without requests: 0, not NaN",
			buckets: minutes(t0.Add(-5*time.Minute), 5, 0, 0),
			window:  5 * time.Minute, now: t0, want: 0,
		},
		{
			name:    "all requests good",
			buckets: minutes(t0.Add(-5*time.Minute), 5, 1000, 0),
			window:  5 * time.Minute, now: t0, want: 0,
		},
		{
			// Integer division would give 2/1000 = 0.
			name:    "2 bad in 1000",
			buckets: []slo.Bucket{bucket(t0.Add(-time.Minute), 1000, 2)},
			window:  5 * time.Minute, now: t0, want: 0.002,
		},
		{
			// 50 bad of 1000 requests is 0.05. The mean of the per-bucket
			// ratios, (0.5 + 0) / 2 = 0.25, is wrong: a quiet minute with
			// few requests must not weigh as much as a busy one.
			name: "sum of bad over sum of total, not the mean of per-bucket ratios",
			buckets: []slo.Bucket{
				bucket(t0.Add(-2*time.Minute), 100, 50),
				bucket(t0.Add(-1*time.Minute), 900, 0),
			},
			window: 5 * time.Minute, now: t0, want: 0.05,
		},
		{
			name: "a bucket that starts exactly at now-window is in the window",
			buckets: []slo.Bucket{
				bucket(t0.Add(-5*time.Minute), 1000, 10),
				bucket(t0.Add(-1*time.Minute), 1000, 0),
			},
			window: 5 * time.Minute, now: t0, want: 0.005,
		},
		{
			// That minute is not over yet, or it is in the future.
			name: "a bucket that starts exactly at now is not in the window",
			buckets: []slo.Bucket{
				bucket(t0.Add(-1*time.Minute), 1000, 0),
				bucket(t0, 1000, 1000),
			},
			window: 5 * time.Minute, now: t0, want: 0,
		},
		{
			name: "buckets older than the window or in the future are ignored",
			buckets: []slo.Bucket{
				bucket(t0.Add(-6*time.Minute), 1000, 1000),
				bucket(t0.Add(-5*time.Minute-time.Second), 1000, 1000),
				bucket(t0.Add(-3*time.Minute), 1000, 4),
				bucket(t0.Add(time.Minute), 1000, 1000),
				bucket(t0.Add(time.Hour), 1000, 1000),
			},
			window: 5 * time.Minute, now: t0, want: 0.004,
		},
		{
			// Only Start decides; with now = 12:00:30 the window is
			// [11:55:30, 12:00:30), so 11:55 is out and 12:00 is in.
			name: "now between two minutes",
			buckets: []slo.Bucket{
				bucket(t0.Add(-5*time.Minute), 1000, 1000),
				bucket(t0.Add(-4*time.Minute), 1000, 3),
				bucket(t0, 1000, 5),
				bucket(t0.Add(time.Minute), 1000, 1000),
			},
			window: 5 * time.Minute, now: t0.Add(30 * time.Second), want: 0.004,
		},
		{
			// Buckets come in any order. Two replicas report the same
			// minute: both count.
			name: "unsorted buckets and repeated Starts all count",
			buckets: []slo.Bucket{
				bucket(t0.Add(-1*time.Minute), 1000, 1),
				bucket(t0.Add(-4*time.Minute), 500, 2),
				bucket(t0.Add(-10*time.Minute), 1000, 1000),
				bucket(t0.Add(-1*time.Minute), 500, 3),
				bucket(t0.Add(-2*time.Minute), 1000, 0),
			},
			window: 5 * time.Minute, now: t0, want: 0.002,
		},
		{
			name:    "window of 1 hour",
			buckets: slices.Concat(minutes(t0.Add(-2*time.Hour), 60, 1000, 500), minutes(t0.Add(-time.Hour), 60, 1000, 3)),
			window:  time.Hour, now: t0, want: 0.003,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			before := slices.Clone(tc.buckets)
			got := slo999.ErrorRatio(tc.buckets, tc.now, tc.window)
			if !approxEqual(got, tc.want) {
				t.Errorf("ErrorRatio(now %s, window %v) = %v, want %v", tc.now.Format(time.TimeOnly), tc.window, got, tc.want)
			}
			checkUnchanged(t, tc.buckets, before)
		})
	}
}

func TestBurnRate(t *testing.T) {
	cases := []struct {
		name      string
		objective float64
		buckets   []slo.Bucket
		window    time.Duration
		want      float64
	}{
		{
			name:      "no traffic burns nothing",
			objective: 0.999, window: time.Hour, want: 0,
		},
		{
			name:      "error ratio equal to the budget burns at 1",
			objective: 0.999, window: time.Hour, want: 1,
			buckets: minutes(t0.Add(-time.Hour), 60, 1000, 1), // 0.1 % errors
		},
		{
			// 1.44 % errors against a 0.1 % budget: the 1h page threshold.
			name:      "99.9 % with 1.44 % errors burns at 14.4",
			objective: 0.999, window: time.Hour, want: 14.4,
			buckets: minutes(t0.Add(-time.Hour), 60, 5000, 72),
		},
		{
			name:      "everything fails at 99.9 % burns at 1000",
			objective: 0.999, window: 5 * time.Minute, want: 1000,
			buckets: minutes(t0.Add(-5*time.Minute), 5, 1000, 1000),
		},
		{
			// The same traffic burns 10 times slower against a 10 times
			// bigger budget.
			name:      "99 % with 0.1 % errors burns at 0.1",
			objective: 0.99, window: time.Hour, want: 0.1,
			buckets: minutes(t0.Add(-time.Hour), 60, 1000, 1),
		},
		{
			name:      "only the window counts",
			objective: 0.999, window: 5 * time.Minute, want: 6,
			buckets: slices.Concat(minutes(t0.Add(-time.Hour), 55, 1000, 1000), minutes(t0.Add(-5*time.Minute), 5, 1000, 6)),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			s := slo.SLO{Objective: tc.objective, Period: month}
			got := s.BurnRate(tc.buckets, t0, tc.window)
			if !approxEqual(got, tc.want) {
				t.Errorf("SLO{Objective: %v}.BurnRate(window %v) = %v, want %v", tc.objective, tc.window, got, tc.want)
			}
		})
	}
}

func TestBudgetRemaining(t *testing.T) {
	const perMinute = 1000             // requests per minute in these tests
	const periodMinutes = 30 * 24 * 60 // 43200 minutes in 30 days
	// At 1000 requests per minute, 30 days hold 43.2 million requests and the
	// 0.1 % budget is 43200 bad requests.
	cases := []struct {
		name    string
		s       slo.SLO
		buckets []slo.Bucket
		want    float64
	}{
		{name: "no buckets: the whole budget is left", s: slo999, want: 1},
		{
			name: "no traffic: the whole budget is left", s: slo999, want: 1,
			buckets: minutes(t0.Add(-time.Hour), 60, 0, 0),
		},
		{
			name: "no errors", s: slo999, want: 1,
			buckets: minutes(t0.Add(-month), periodMinutes, perMinute, 0),
		},
		{
			// 108 minutes with 100 bad = 10800 bad = a quarter of the budget.
			// The bad minutes come first: buckets come in any order.
			name: "a quarter spent", s: slo999, want: 0.75,
			buckets: slices.Concat(
				minutes(t0.Add(-108*time.Minute), 108, perMinute, 100),
				minutes(t0.Add(-month), periodMinutes-108, perMinute, 0),
			),
		},
		{
			// 432 minutes with 100 bad = 43200 bad = the whole budget.
			name: "exactly spent", s: slo999, want: 0,
			buckets: slices.Concat(
				minutes(t0.Add(-month), periodMinutes-432, perMinute, 0),
				minutes(t0.Add(-432*time.Minute), 432, perMinute, 100),
			),
		},
		{
			// 864 minutes with 100 bad = twice the budget.
			name: "overspent is negative", s: slo999, want: -1,
			buckets: slices.Concat(
				minutes(t0.Add(-month), periodMinutes-864, perMinute, 0),
				minutes(t0.Add(-864*time.Minute), 864, perMinute, 100),
			),
		},
		{
			// The period rolls: a bad day 31 days ago no longer counts.
			name: "errors older than the period no longer count", s: slo999, want: 1,
			buckets: slices.Concat(
				minutes(t0.Add(-31*day), 1440, perMinute, 100),
				minutes(t0.Add(-month), periodMinutes, perMinute, 0),
			),
		},
		{
			// With a 1-hour period: the bucket at now-1h counts (2000
			// requests, 1 bad, half the budget of 2 bad requests); the
			// bucket 1 minute before it and the bucket at now do not.
			name: "the period is [now-Period, now)",
			s:    slo.SLO{Objective: 0.999, Period: time.Hour}, want: 0.5,
			buckets: []slo.Bucket{
				bucket(t0.Add(-61*time.Minute), 1000, 1000),
				bucket(t0.Add(-60*time.Minute), 2000, 1),
				bucket(t0, 1000, 1000),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			before := slices.Clone(tc.buckets)
			got := tc.s.BudgetRemaining(tc.buckets, t0)
			if !approxEqual(got, tc.want) {
				t.Errorf("BudgetRemaining() = %v, want %v", got, tc.want)
			}
			checkUnchanged(t, tc.buckets, before)
		})
	}
}

func TestTimeToExhaustion(t *testing.T) {
	// halfSpent is 30 days at 5000 requests per minute (216 million requests,
	// a budget of 216000 bad ones). One bad day 20 days ago and the last hour
	// both fail 72 of 5000 requests per minute (1.44 %): 1500 minutes * 72 =
	// 108000 bad requests, half the budget. The last hour burns at 14.4.
	halfSpent := slices.Concat(
		minutes(t0.Add(-month), 10*1440, 5000, 0),
		minutes(t0.Add(-20*day), 1440, 5000, 72),
		minutes(t0.Add(-19*day), 19*1440-60, 5000, 0),
		minutes(t0.Add(-time.Hour), 60, 5000, 72),
	)
	// quietNow has the same bad day, but the last hour is clean.
	quietNow := slices.Concat(
		minutes(t0.Add(-month), 10*1440, 5000, 0),
		minutes(t0.Add(-20*day), 1440, 5000, 72),
		minutes(t0.Add(-19*day), 19*1440, 5000, 0),
	)
	// overspent has twice the budget spent, and the last hour burns at 100.
	overspent := slices.Concat(
		minutes(t0.Add(-month), 30*1440-864, 1000, 0),
		minutes(t0.Add(-864*time.Minute), 864, 1000, 100),
	)

	cases := []struct {
		name    string
		buckets []slo.Bucket
		window  time.Duration
		want    time.Duration
		wantOK  bool
	}{
		{
			// 0.5 * 720h / 14.4 = 25h.
			name: "half the budget left, burning at 14.4 over 1h", buckets: halfSpent,
			window: time.Hour, want: 25 * time.Hour, wantOK: true,
		},
		{
			// Over 6h the same errors are a burn rate of 2.4: 0.5 * 720h / 2.4 = 150h.
			name: "same data, burn rate over 6h", buckets: halfSpent,
			window: 6 * time.Hour, want: 150 * time.Hour, wantOK: true,
		},
		{
			name: "no errors in the window: burn rate 0", buckets: quietNow,
			window: time.Hour, want: 0, wantOK: false,
		},
		{
			name: "budget already spent", buckets: overspent,
			window: time.Hour, want: 0, wantOK: false,
		},
		{
			name: "no traffic at all", buckets: nil,
			window: time.Hour, want: 0, wantOK: false,
		},
		{
			// 1 bad in 10 million is a burn rate of 0.0001: about 820 years,
			// which does not fit in a time.Duration (at most about 292 years).
			name:    "too far away for a time.Duration: capped at math.MaxInt64",
			buckets: []slo.Bucket{bucket(t0.Add(-time.Minute), 10_000_000, 1)},
			window:  time.Hour, want: math.MaxInt64, wantOK: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			got, ok := slo999.TimeToExhaustion(tc.buckets, t0, tc.window)
			if ok != tc.wantOK || !durationClose(got, tc.want) {
				t.Errorf("TimeToExhaustion(window %v) = (%v, %v), want (%v, %v)", tc.window, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
