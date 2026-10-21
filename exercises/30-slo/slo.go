// Package slo computes error budgets and multi-window, multi-burn-rate alerts
// for a request-based availability SLO, as described in the Google SRE
// Workbook chapter "Alerting on SLOs".
package slo

import (
	"errors"
	"time"
)

// Bucket holds the request counts of one minute, starting at Start.
// Good <= Total, and both are >= 0.
type Bucket struct {
	Start time.Time
	Good  int64 // requests that met the SLO
	Total int64 // all requests
}

// SLO is a request-based availability objective over a rolling period, for
// example 99.9 % of requests are good over the last 30 days.
//
// Every method except Validate assumes that s is valid: call Validate once
// when you load the SLO. The methods take the buckets, then the evaluation
// time now, then what to compute (a window or rules). None reads the clock.
type SLO struct {
	Objective float64       // fraction of good requests, 0.999 for 99.9 %
	Period    time.Duration // length of the rolling period, 30 * 24 * time.Hour
}

// ErrInvalidSLO is wrapped by the error Validate returns.
var ErrInvalidSLO = errors.New("slo: invalid SLO")

// Validate returns nil if 0 < Objective < 1 and Period > 0. Otherwise it
// returns an error that wraps ErrInvalidSLO. NaN and infinite objectives are
// invalid.
func (s SLO) Validate() error {
	panic("TODO: implement")
}

// ErrorRatio returns the fraction of bad requests (Total - Good) among all
// requests in the buckets whose Start is in [now-window, now). It returns 0
// when there is no traffic in the window. It does not modify buckets.
func (s SLO) ErrorRatio(buckets []Bucket, now time.Time, window time.Duration) float64 {
	panic("TODO: implement")
}

// BurnRate returns how fast the error budget burns in the window
// [now-window, now): the error ratio divided by the allowed error ratio
// (1 - Objective). A burn rate of 1 spends exactly the whole budget in one
// Period. No traffic means a burn rate of 0.
func (s SLO) BurnRate(buckets []Bucket, now time.Time, window time.Duration) float64 {
	panic("TODO: implement")
}

// BudgetRemaining returns the fraction of the error budget of the period
// [now-Period, now) that is left: 1 when nothing is spent (also when there is
// no traffic), 0 when it is spent exactly, negative when it is overspent.
func (s SLO) BudgetRemaining(buckets []Bucket, now time.Time) float64 {
	panic("TODO: implement")
}

// Rule is one multi-window burn-rate alert: it fires when the burn rates over
// both the Long and the Short window reach Factor.
type Rule struct {
	Name     string
	Severity string // "page" or "ticket"
	Long     time.Duration
	Short    time.Duration
	Factor   float64
}

// DefaultRules returns the 4 rules for a 30-day SLO from the table in the
// README, in that order. Each call returns a new slice, so a caller can change
// its copy without affecting other callers.
func DefaultRules() []Rule {
	panic("TODO: implement")
}

// Alert is a rule that fires, with the burn rates that made it fire.
type Alert struct {
	Rule      string // Rule.Name
	Severity  string // Rule.Severity
	LongBurn  float64
	ShortBurn float64
}

// Evaluate returns an Alert for every rule whose long and short burn rates
// both reach its Factor (with the tolerance the README describes), in the
// order of rules. With no alerts it returns nil or an empty slice.
func (s SLO) Evaluate(buckets []Bucket, now time.Time, rules []Rule) []Alert {
	panic("TODO: implement")
}

// TimeToExhaustion estimates how long the remaining error budget lasts if the
// burn rate over window continues. It returns (0, false) when that burn rate
// is 0 or the budget is already spent. A result too large for a
// time.Duration is capped at math.MaxInt64 (about 292 years).
func (s SLO) TimeToExhaustion(buckets []Bucket, now time.Time, window time.Duration) (time.Duration, bool) {
	panic("TODO: implement")
}
