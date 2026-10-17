package leaderelection

import (
	"context"
	"errors"
	"time"
)

// Lease is the record in the store that says who leads. Its fields mirror
// the spec of a Kubernetes Lease object (coordination.k8s.io/v1).
type Lease struct {
	// Holder is the ID of the candidate that holds the lease. "" means
	// nobody holds it: the store is new, or the last holder released it.
	Holder string

	// Epoch goes up by 1 every time a candidate acquires the lease, and only
	// then: renewing and releasing keep it. The holder uses it as its
	// fencing token.
	Epoch uint64

	// AcquireTime is when the current holder acquired the lease.
	AcquireTime time.Time

	// RenewTime is when the holder last acquired or renewed the lease.
	RenewTime time.Time

	// Duration is how long the lease is valid after RenewTime. The lease has
	// expired at time now when now >= RenewTime + Duration.
	Duration time.Duration

	// Version is the store's version of the record. The store sets it, and
	// LeaseStore.Update succeeds only if it is unchanged since the read.
	Version int64
}

// Next decides what candidate id writes at time now, with lease duration d,
// given the current lease cur. It is a pure function: no clock, no store.
//
//   - cur.Holder == id: a renew. The result is cur with RenewTime = now and
//     Duration = d. Holder, Epoch, AcquireTime and Version stay. This holds
//     also when cur has expired.
//   - cur.Holder == "", or cur has expired (now >= cur.RenewTime +
//     cur.Duration): an acquire. The result has Holder = id, Epoch =
//     cur.Epoch + 1, AcquireTime = RenewTime = now, Duration = d and Version =
//     cur.Version.
//   - otherwise another candidate holds a lease that is still valid: Next
//     returns cur unchanged and false.
//
// The bool is true for a renew and an acquire. The result keeps cur.Version,
// so the caller can pass it to LeaseStore.Update as is. id must not be empty.
func Next(cur Lease, id string, now time.Time, d time.Duration) (Lease, bool) {
	panic("TODO: implement")
}

// ErrConflict is wrapped by the error LeaseStore.Update returns when the
// stored lease has changed since the caller read it.
var ErrConflict = errors.New("leaderelection: lease was modified since it was read")

// LeaseStore keeps one Lease, like one Lease object in the Kubernetes API.
// Implementations must be safe for concurrent use.
type LeaseStore interface {
	// Get returns the stored lease. A store that was never written returns
	// the zero Lease (Holder "", Version 0) and nil.
	Get(ctx context.Context) (Lease, error)

	// Update is a compare-and-swap on Version. If l.Version equals the
	// stored Version, Update stores l with Version + 1 and returns the stored
	// lease. Otherwise it changes nothing and returns an error that wraps
	// ErrConflict.
	Update(ctx context.Context, l Lease) (Lease, error)
}

// MemoryStore is a LeaseStore in memory. It is safe for concurrent use.
//
// Like a client of a real API, it refuses to work for a caller that has
// already given up: if ctx is done, Get and Update change nothing and return
// ctx.Err().
type MemoryStore struct {
	// TODO: add fields.
}

// NewMemoryStore returns an empty store: Get returns the zero Lease.
func NewMemoryStore() *MemoryStore {
	panic("TODO: implement")
}

// Get returns the stored lease, or ctx.Err() if ctx is done.
func (s *MemoryStore) Get(ctx context.Context) (Lease, error) {
	panic("TODO: implement")
}

// Update stores l with Version + 1 and returns the stored lease if
// l.Version equals the stored Version. If it does not, Update returns an
// error that wraps ErrConflict. If ctx is done, it returns ctx.Err(). Both
// errors leave the store unchanged. Comparing and storing are one atomic
// step.
func (s *MemoryStore) Update(ctx context.Context, l Lease) (Lease, error) {
	panic("TODO: implement")
}
