// Package circuitbreaker protects a caller from a dependency that is down:
// after too many consecutive failures it stops calling the dependency for a
// while, and then lets a few probe calls through to find out whether it has
// recovered.
package circuitbreaker

import (
	"errors"
	"fmt"
	"time"
)

// State is the state of a Breaker.
type State int

// The three states of a Breaker. The zero value is Closed.
const (
	Closed   State = iota // calls go through; failures are counted
	Open                  // calls are rejected with ErrOpen
	HalfOpen              // a limited number of probe calls go through
)

// String returns "closed", "open" or "half-open", or "State(N)" for any
// other value.
func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	}
	return fmt.Sprintf("State(%d)", int(s))
}

// Errors that Execute returns when it rejects a call without calling fn.
var (
	// ErrOpen means the breaker is Open.
	ErrOpen = errors.New("circuitbreaker: circuit is open")
	// ErrTooManyCalls means the breaker is HalfOpen and HalfOpenMaxCalls
	// probes are already running.
	ErrTooManyCalls = errors.New("circuitbreaker: too many probe calls in half-open state")
)

// Settings configures a Breaker. The four numbers are required; the three
// functions are optional.
type Settings struct {
	// FailureThreshold is the number of consecutive failures in Closed
	// that opens the breaker. Must be at least 1.
	FailureThreshold int

	// OpenTimeout is how long the breaker stays Open. Once at least
	// OpenTimeout has passed since it opened, it is HalfOpen. Must be
	// greater than 0.
	OpenTimeout time.Duration

	// HalfOpenMaxCalls is the number of probe calls that may run at the
	// same time in HalfOpen. Must be at least 1.
	HalfOpenMaxCalls int

	// SuccessThreshold is the number of consecutive probe successes in
	// HalfOpen that closes the breaker. Must be at least 1. It may be
	// larger than HalfOpenMaxCalls.
	SuccessThreshold int

	// IsFailure decides whether a non-nil error returned by fn counts as a
	// failure. An error for which it returns false counts as a success:
	// the dependency answered, the request was just bad (a 404, a
	// validation error). IsFailure is never called with a nil error; nil is
	// always a success. If IsFailure is nil, every non-nil error is a
	// failure.
	IsFailure func(err error) bool

	// Now returns the current time. If Now is nil, time.Now is used. Tests
	// set it to a fake clock.
	Now func() time.Time

	// OnStateChange, if not nil, is called once for every state change,
	// with the old and the new state. It is called without holding the
	// breaker's lock, so it may call State, Counts and Execute. Calls never
	// overlap and report the changes in the order they happened. When only
	// one goroutine uses the breaker, the call for a change has returned
	// before the Execute or State call that made the change returns (or
	// panics). A change made by a State or Execute call inside the callback
	// is reported right after the callback returns.
	OnStateChange func(from, to State)
}

// Counts are the numbers of calls since the last state change. Every state
// change resets all of them to 0.
type Counts struct {
	Requests             uint64 // calls admitted (fn called), counted before fn runs
	Successes            uint64 // admitted calls that returned a success
	Failures             uint64 // admitted calls that returned a failure or panicked
	ConsecutiveSuccesses uint64 // successes since the last failure
	ConsecutiveFailures  uint64 // failures since the last success
}

// Breaker is a circuit breaker. It is Closed (calls go through), Open (calls
// are rejected) or HalfOpen (a limited number of probe calls go through).
// Create one with New. A Breaker is safe for concurrent use.
type Breaker struct {
	// TODO: add fields.
}

// New returns a Closed breaker with zero Counts. It returns a nil *Breaker
// and an error if FailureThreshold, HalfOpenMaxCalls or SuccessThreshold is
// below 1, or if OpenTimeout is not positive. The error message names the
// bad field.
func New(s Settings) (*Breaker, error) {
	panic("TODO: implement")
}

// Execute calls fn if the breaker admits the call, and returns fn's error
// unchanged. It rejects the call without calling fn, and returns ErrOpen
// while the breaker is Open, or ErrTooManyCalls while it is HalfOpen and
// HalfOpenMaxCalls probes are running.
//
// The result of fn is recorded as a success or a failure (see
// Settings.IsFailure) and can change the state. A result is ignored if the
// state has changed since fn was called. If fn panics, the call counts as a
// failure and Execute panics with the same value.
//
// Execute does not hold the breaker's lock while fn runs.
func (b *Breaker) Execute(fn func() error) error {
	panic("TODO: implement")
}

// State returns the current state. If the breaker is Open and OpenTimeout
// has passed, State changes it to HalfOpen first (and reports that change to
// OnStateChange).
func (b *Breaker) State() State {
	panic("TODO: implement")
}

// Counts returns the counts since the last state change.
func (b *Breaker) Counts() Counts {
	panic("TODO: implement")
}
