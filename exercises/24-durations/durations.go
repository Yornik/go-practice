// Package durations parses and formats durations with days and weeks, such as
// "2d" or "1w2d3h", which time.ParseDuration rejects.
package durations

import (
	"errors"
	"time"
)

// The sentinel errors. A *SyntaxError from Parse wraps exactly 1 of them in
// its Err field. Check for them with errors.Is.
var (
	ErrNumber      = errors.New("expected a number")
	ErrMissingUnit = errors.New("missing unit")
	ErrUnknownUnit = errors.New("unknown unit")
	ErrUnitOrder   = errors.New("units out of order")
	ErrOverflow    = errors.New("duration out of range")
)

// SyntaxError reports an input that Parse rejects, and where.
type SyntaxError struct {
	// Input is the whole input that Parse received.
	Input string
	// Col is the 1-based byte column of the problem: the byte at
	// Input[Col-1], or len(Input)+1 when the problem is the end of the input.
	Col int
	// Err is 1 of the sentinel errors: ErrNumber, ErrMissingUnit,
	// ErrUnknownUnit, ErrUnitOrder or ErrOverflow.
	Err error
}

// Error returns `duration <Input quoted with %q>: column <Col>: <Err>`, for
// example `duration "2x": column 2: unknown unit`.
func (e *SyntaxError) Error() string {
	panic("TODO: implement")
}

// Unwrap returns Err, so that errors.Is(err, ErrUnknownUnit) works on a
// *SyntaxError, also when it is wrapped with %w.
func (e *SyntaxError) Unwrap() error {
	panic("TODO: implement")
}

// Parse converts a duration such as "2d", "1w2d3h", "90m", "1500ms" or "0"
// to a time.Duration.
//
// The input is "0", or 1 or more terms. A term is 1 or more ASCII digits
// followed by a unit: w (7 days), d (24 hours), h, m, s or ms. The units
// appear from largest to smallest, each at most once. Spaces, signs and
// decimal points are not allowed. The README holds the grammar.
//
// On malformed input Parse returns 0 and a *SyntaxError that points at the
// first problem. Parse never panics.
func Parse(s string) (time.Duration, error) {
	panic("TODO: implement")
}

// Format writes d with the largest units first, for example "1w2d3h" or
// "1h30m". It truncates d toward zero to whole milliseconds and writes only
// the units that are not zero. It writes "0s" when nothing is left, which is
// the case for any d above -1 ms and below 1 ms, and puts "-" before a
// negative value. For every d >= 0,
// Parse(Format(d)) == d.Truncate(time.Millisecond).
func Format(d time.Duration) string {
	panic("TODO: implement")
}
