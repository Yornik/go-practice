package reconciler

import (
	"context"
	"time"
)

// Controller keeps the objects in Store equal to the desired state, by
// running Reconcile for every name it finds in a Queue.
type Controller struct {
	// Desired returns the current desired state: name -> spec. Run calls it
	// for every reconcile and from several goroutines at once, so it must
	// be safe for that.
	Desired func() map[string]map[string]string
	// Store holds the actual state.
	Store Store
	// Queue holds the names to reconcile. Run owns it and shuts it down
	// when it stops. Other code may Add names to it while Run runs, like a
	// watch event handler does.
	Queue *Queue[string]
	// Workers is the number of goroutines that reconcile at the same time.
	Workers int
	// Resync is how often Run adds every desired and every actual name to
	// the queue again.
	Resync time.Duration
}

// Run starts Workers goroutines that take names from the queue and reconcile
// them. A failed reconcile is retried with AddRateLimited, a successful one
// resets the backoff with Forget.
//
// At the start and then every Resync, Run adds the name of every desired
// object and of every object in Store.List to the queue. If List fails, Run
// adds the desired names and tries List again at the next resync.
//
// When ctx is done, Run starts no new reconcile, shuts the queue down, waits
// until every goroutine it started has returned and returns nil. Run returns
// an error at once if Desired, Store or Queue is nil, Workers < 1 or
// Resync <= 0.
func (c *Controller) Run(ctx context.Context) error {
	panic("TODO: implement")
}
