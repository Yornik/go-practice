// Package alertstate tracks the state of 1 alert rule from evaluation to
// evaluation: inactive, pending or firing. A rule waits before it fires
// (For) and waits before it resolves (KeepFiringFor), like the for and
// keep_firing_for fields of a Prometheus alerting rule.
package alertstate

import (
	"errors"
	"time"
)

// State is the state of an alert. The zero value is Inactive.
type State int

// The 3 states of an alert.
const (
	Inactive State = iota // the condition is false, or has been false for KeepFiringFor
	Pending               // the condition is true, but not yet for the duration For
	Firing                // the condition has been true for For; a human is paged
)

// String returns "inactive", "pending" or "firing". For any other value it
// returns "State(N)", for example "State(7)".
func (s State) String() string {
	panic("TODO: implement")
}

// Rule holds the 2 durations of an alert rule. Both must be 0 or more.
type Rule struct {
	// For is how long the condition must stay true before a Pending alert
	// becomes Firing. 0 means the alert fires at the first true evaluation.
	For time.Duration
	// KeepFiringFor is how long a Firing alert keeps firing after the last
	// evaluation that saw the condition true. 0 means it resolves at the
	// first false evaluation.
	KeepFiringFor time.Duration
}

// Transition is 1 change of state.
type Transition struct {
	At       time.Time // the time of the evaluation that changed the state
	From, To State
}

// ErrInvalidRule is wrapped by the error New returns for a rule with a
// negative duration. Check for it with errors.Is.
var ErrInvalidRule = errors.New("alertstate: invalid rule")

// Alert is 1 alert rule and its state. It starts Inactive.
//
// An Alert is not safe for concurrent use: 1 evaluation loop owns it.
type Alert struct {
	// TODO: add fields.
}

// New returns an Alert for the rule r, in the state Inactive, with no
// history. now is the clock that Eval reads; nil means time.Now.
//
// If r.For or r.KeepFiringFor is negative, New returns a nil *Alert and an
// error that wraps ErrInvalidRule.
func New(r Rule, now func() time.Time) (*Alert, error) {
	panic("TODO: implement")
}

// Eval runs 1 evaluation of the rule at the time now() returns. active
// tells whether the alert condition is true at that time. Eval returns the
// state after the evaluation.
//
// The README gives the transitions as a table. An evaluation at a time
// before the latest evaluation that was not ignored is ignored: it changes
// nothing and returns the current state.
func (a *Alert) Eval(active bool) State {
	panic("TODO: implement")
}

// State returns the current state, the one the latest Eval returned.
func (a *Alert) State() State {
	panic("TODO: implement")
}

// ActiveAt returns the start of the current Pending or Firing episode: the
// time of the evaluation that moved the alert out of Inactive. While the
// alert is Inactive, it returns the zero time.Time.
func (a *Alert) ActiveAt() time.Time {
	panic("TODO: implement")
}

// History returns every change of state, oldest first. An evaluation that
// does not change the state adds nothing. The caller may change the
// returned slice; that does not change the Alert.
func (a *Alert) History() []Transition {
	panic("TODO: implement")
}
