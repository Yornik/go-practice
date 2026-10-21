// Package nines does the arithmetic behind an availability objective such as
// "99.95%": the downtime the objective allows in a window, the real downtime
// that a list of outage reports adds up to, and whether the objective was met.
package nines

import (
	"errors"
	"time"
)

// ErrInvalid is wrapped by every error that this package returns, so callers
// can test for it with errors.Is.
var ErrInvalid = errors.New("nines: invalid input")

// ParseObjective reads an availability objective written as a percentage,
// such as "99.95%" or "99.95", and returns it as a fraction: 0.9995.
//
// Whitespace around the input is ignored, and 1 "%" may follow the number
// directly. The number must be above 0 and below 100. NaN and infinities are
// errors in any spelling. On error the value is 0, and the error wraps
// ErrInvalid and contains s as the caller passed it, before trimming,
// formatted with %q.
func ParseObjective(s string) (float64, error) {
	panic("TODO: implement")
}

// Budget returns the downtime that objective allows in window:
// window × (1 − objective), rounded to the nearest second.
//
// objective is a fraction such as 0.9995. Budget does not check its input;
// callers pass an objective above 0 and below 1.
func Budget(objective float64, window time.Duration) time.Duration {
	panic("TODO: implement")
}

// Outage is a period in which the service was down. It is half-open: it
// includes Start and ends just before End, so an outage [a, b) and an outage
// [b, c) touch but do not overlap.
type Outage struct {
	Start, End time.Time
}

// Downtime returns how much of the window [from, to) at least 1 outage
// covers. Time that several outages cover counts once, and an outage counts
// only for its part inside the window. An outage whose End is not after its
// Start covers nothing. The outages may come in any order, and Downtime does
// not change the slice or its elements. If to is not after from, Downtime
// returns 0.
func Downtime(outages []Outage, from, to time.Time) time.Duration {
	panic("TODO: implement")
}

// Availability returns the fraction of window in which the service was up:
// 1 − down/window. It returns NaN when window is 0 or negative.
//
// It does not clamp: callers pass a down between 0 and window.
func Availability(down, window time.Duration) float64 {
	panic("TODO: implement")
}

// Report is the result of Check for 1 window.
type Report struct {
	Window       time.Duration // to − from
	Downtime     time.Duration // Downtime(outages, from, to)
	Availability float64       // Availability(Downtime, Window)
	Budget       time.Duration // Budget(objective, Window)
	Remaining    time.Duration // Budget − Downtime; negative when Downtime exceeds Budget
	Met          bool          // Downtime <= Budget
}

// Check measures the window [from, to) against objective (a fraction such
// as 0.9995) and returns the Report.
//
// It returns an error that wraps ErrInvalid, and the zero Report, when
// objective is NaN or not above 0 and below 1, when to is not after from, or
// when an outage ends before it starts. The last error names the first such
// outage by its 0-based index in the slice, for example "outage 1". An outage
// whose End equals its Start is fine: it covers nothing.
func Check(objective float64, from, to time.Time, outages []Outage) (Report, error) {
	panic("TODO: implement")
}
