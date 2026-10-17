// Package leaderelection makes sure that only one replica of a service acts
// at a time. Candidates compete for a lease in a shared store, the holder
// renews it, and a holder that cannot renew in time steps down before the
// lease expires. The lease epoch is a fencing token that protects resources
// from a leader that froze and woke up after it was replaced.
package leaderelection

import (
	"context"
	"errors"
	"time"
)

// ErrLeaseLost is wrapped by the error Run returns when it stopped leading
// because it could not renew the lease in time, or because another candidate
// holds it now.
var ErrLeaseLost = errors.New("leaderelection: lease lost")

// Config configures one candidate. Durations follow client-go: LeaseDuration
// > RenewDeadline > RetryPeriod > 0. The Kubernetes control plane uses 15 s,
// 10 s and 2 s.
type Config struct {
	// ID identifies this candidate, for example the pod name. It must be
	// unique among the candidates and not empty.
	ID string

	// Store holds the lease. Required.
	Store LeaseStore

	// LeaseDuration is how long a lease this candidate writes stays valid
	// after it was acquired or renewed. Other candidates wait that long
	// before they take over a lease that is no longer renewed.
	LeaseDuration time.Duration

	// RenewDeadline is how long the leader keeps trying to renew before it
	// gives up and stops leading, counted from its last successful renew.
	RenewDeadline time.Duration

	// RetryPeriod is the time between two attempts, both to acquire and to
	// renew. Run waits with real timers.
	RetryPeriod time.Duration

	// Now is the clock for every time Run writes into a lease or compares:
	// expiry and the renew deadline. nil means time.Now.
	Now func() time.Time

	// OnStartedLeading is called in a new goroutine when this candidate
	// acquires the lease. epoch is the lease Epoch, the fencing token for
	// everything the leader writes. ctx is derived from Run's ctx and is
	// cancelled when leadership ends; the callback must return soon after.
	// Required.
	OnStartedLeading func(ctx context.Context, epoch uint64)

	// OnStoppedLeading is called once when leadership ends, after
	// OnStartedLeading has returned. It is not called if this candidate
	// never led. May be nil.
	OnStoppedLeading func()

	// OnNewLeader is called when the holder this candidate observes in the
	// store differs from the last holder it reported, including the
	// candidate itself when it acquires. An empty holder is never reported.
	// Called from Run's goroutine; it must return quickly. May be nil.
	OnNewLeader func(id string)

	// ReleaseOnCancel makes Run release the lease (Holder "") when ctx is
	// cancelled while this candidate leads, so another candidate can take
	// over at once instead of waiting for the lease to expire.
	ReleaseOnCancel bool
}

// Validate returns nil if c can be used, or an error that names every bad
// field: ID is empty, Store is nil, OnStartedLeading is nil, RetryPeriod <=
// 0, RenewDeadline <= RetryPeriod, or LeaseDuration <= RenewDeadline. Now,
// OnStoppedLeading and OnNewLeader may be nil.
func (c Config) Validate() error {
	panic("TODO: implement")
}

// Run takes part in the election until this candidate's leadership ends or
// ctx is done, and then returns. A candidate leads at most once per call.
//
// As a candidate, Run tries to acquire the lease at once and then every
// RetryPeriod; store errors only mean "try again". As the leader, it renews
// the lease every RetryPeriod. It stops leading when another candidate holds
// the lease, or when RenewDeadline has passed since its last successful
// renew; store calls that hang do not delay this.
//
// When leadership ends, Run cancels the ctx it passed to OnStartedLeading,
// waits until OnStartedLeading has returned, calls OnStoppedLeading, releases
// the lease if ctx was cancelled and ReleaseOnCancel is set, and returns.
//
// Run returns the error of Validate for an invalid config (without touching
// the store), ctx.Err() when ctx is done, and an error that wraps
// ErrLeaseLost when it stopped leading for another reason.
func Run(ctx context.Context, c Config) error {
	panic("TODO: implement")
}
