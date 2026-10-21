// Package deploynote checks a deploy note against the team's deploy policy
// and reports every problem at once, as errors that a human can read and a
// program can inspect.
package deploynote

import (
	"errors"
	"time"
)

// Note is the deploy note that the deploy bot receives before a deploy.
type Note struct {
	Service  string
	Version  string
	Ticket   string // "VL-123", or "INC-45" for an incident fix
	Author   string
	Rollback string    // how to undo the deploy
	At       time.Time // planned time, in the team's zone
}

// The sentinel errors that a FieldError wraps. Check for them with
// errors.Is.
var (
	// ErrMissing: a required field is empty or only whitespace, a Rollback
	// says "n/a", "none" or "-", or At is the zero time.
	ErrMissing = errors.New("missing")
	// ErrBadFormat: Ticket is not "VL-" or "INC-" followed by 1 to 6
	// ASCII digits.
	ErrBadFormat = errors.New("bad format")
	// ErrFreeze: At falls in the change freeze, from Friday 15:00 to Monday
	// 09:00, and the ticket is not a valid "INC-" ticket.
	ErrFreeze = errors.New("change freeze")
)

// FieldError is 1 problem with 1 field of a Note.
type FieldError struct {
	Field string // "Service", "Version", "Ticket", "Author", "Rollback" or "At"
	Err   error  // the problem; it is or wraps ErrMissing, ErrBadFormat or ErrFreeze
}

// Error returns the field name, a colon, a space and the message of Err,
// for example "Rollback: missing".
func (e *FieldError) Error() string {
	panic("TODO: implement")
}

// Unwrap returns Err, so that errors.Is and errors.As look inside a
// FieldError.
func (e *FieldError) Unwrap() error {
	panic("TODO: implement")
}

// Check checks every field of n against the deploy policy.
//
// It returns nil when n has no problem. Otherwise it returns errors.Join of
// 1 *FieldError per field with a problem, in the order Service, Version,
// Ticket, Author, Rollback, At. Each field reports at most 1 problem.
//
// The freeze uses the weekday and the hour of At in At's own location.
func Check(n Note) error {
	panic("TODO: implement")
}
