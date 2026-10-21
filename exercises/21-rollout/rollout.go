// Package rollout waits for a rollout to finish. It polls the rollout once
// per tick, returns as soon as it is done, and fails as soon as it stops
// making progress, the polls keep failing, the ticks stop or the context
// ends.
package rollout

import (
	"context"
	"errors"
	"time"
)

// Status is the state of a rollout at one poll, in the terms of a
// Kubernetes Deployment.
type Status struct {
	Desired int // the number of replicas the new version must have (.spec.replicas)
	Updated int // replicas that run the new version (.status.updatedReplicas)
	Ready   int // replicas that pass their readiness probe (.status.readyReplicas)
}

// Errors that Wait wraps. Compare with errors.Is.
var (
	// ErrStalled: StallTicks polls in a row made no progress.
	ErrStalled = errors.New("rollout: no progress")
	// ErrStopped: the Ticks channel was closed.
	ErrStopped = errors.New("rollout: ticks stopped")
	// ErrInvalid: the Options cannot be used.
	ErrInvalid = errors.New("rollout: invalid options")
)

// Options configures Wait.
type Options struct {
	// Poll reads the current status of the rollout. Wait passes it the ctx
	// it received, or a context derived from it. Required.
	Poll func(ctx context.Context) (Status, error)

	// Ticks paces the polls: after the first poll, Wait polls once for
	// every tick it receives. In production this is the C field of a
	// time.Ticker. Required.
	Ticks <-chan time.Time

	// StallTicks is the number of polls in a row without progress after
	// which Wait gives up with ErrStalled. 0 means never. Must not be
	// negative.
	StallTicks int

	// MaxErrors is the number of failed polls in a row that Wait
	// tolerates. One more failed poll ends Wait. Must not be negative.
	MaxErrors int
}

// Wait polls the rollout right away and then once per tick, until the
// rollout is done or Wait gives up.
//
// After each poll Wait checks, in this order:
//   - errors: more than MaxErrors failed polls in a row return an error
//     that wraps the error of the last poll;
//   - done: a successful poll with Updated == Desired and Ready == Desired
//     returns that Status and nil;
//   - stall: StallTicks polls in a row without progress (StallTicks > 0)
//     return an error that wraps ErrStalled.
//
// A poll makes progress when it succeeds and its Updated or its Ready is
// higher than the highest value seen in any earlier successful poll. The
// first successful poll always makes progress. A failed poll never does,
// and Wait ignores the Status that comes with a poll error.
//
// Wait checks ctx before every poll and while it waits for a tick. When ctx
// is done, Wait returns an error that wraps ctx.Err(), and it never polls
// again. A closed Ticks channel returns an error that wraps ErrStopped.
// Invalid options (a nil Poll or Ticks, a negative StallTicks or
// MaxErrors) return an error that wraps ErrInvalid, before any poll.
//
// The returned Status is the Status of the last successful poll, or the
// zero Status if no poll succeeded. Wait starts no goroutine that outlives
// it.
func Wait(ctx context.Context, opt Options) (Status, error) {
	panic("TODO: implement")
}
