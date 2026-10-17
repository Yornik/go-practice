// Package reconciler is the core of a Kubernetes-style controller: a work
// queue with the semantics of client-go's workqueue, a store with optimistic
// concurrency, an idempotent level-triggered Reconcile, and a controller loop
// that keeps running it until the actual state matches the desired state.
package reconciler

import "time"

// Clock schedules a function to run later. The queue uses it for AddAfter
// and AddRateLimited, so that tests can control time.
type Clock interface {
	// AfterFunc arranges for f to be called once d has passed. It never
	// calls f itself: f runs later, possibly in another goroutine.
	AfterFunc(d time.Duration, f func())
}

// Queue is a work queue of keys, with the semantics of client-go's
// workqueue:
//
//   - A key is at most once in the queue. Adding a key that is already
//     waiting does nothing.
//   - Get hands out the oldest waiting key and marks it as processing until
//     Done is called for it.
//   - A key that is processing is never handed out again before Done. If it
//     is added while processing, it is queued again when Done is called.
//
// So one key is never processed by two workers at the same time, and a
// change that happens while a key is processing is never lost.
//
// Every method is safe for concurrent use.
type Queue[K comparable] struct {
	// TODO: add fields.
}

// NewQueue returns an empty queue that uses real time. baseDelay is the
// delay of the first AddRateLimited of a key and maxDelay the largest delay.
// Both must be > 0, and maxDelay >= baseDelay.
func NewQueue[K comparable](baseDelay, maxDelay time.Duration) *Queue[K] {
	panic("TODO: implement")
}

// NewQueueWithClock is NewQueue with a custom clock for AddAfter and
// AddRateLimited.
func NewQueueWithClock[K comparable](baseDelay, maxDelay time.Duration, clock Clock) *Queue[K] {
	panic("TODO: implement")
}

// Add queues k unless it is already waiting. If k is processing, it is not
// queued now: it is queued again, once, when Done(k) is called. Add does
// nothing after ShutDown.
func (q *Queue[K]) Add(k K) {
	panic("TODO: implement")
}

// Get removes the oldest waiting key from the queue, marks it as processing
// and returns it with true. While the queue is empty, Get blocks until a key
// is added or ShutDown is called. After ShutDown, Get still returns the keys
// that are waiting; once there are none, it returns the zero K and false
// without blocking.
func (q *Queue[K]) Get() (K, bool) {
	panic("TODO: implement")
}

// Done marks k as no longer processing. If k was added while it was
// processing, Done queues it again.
func (q *Queue[K]) Done(k K) {
	panic("TODO: implement")
}

// AddAfter adds k once d has passed, as Add would. If d <= 0, it adds k at
// once. It never blocks: for d > 0 it schedules the add on the queue's
// Clock and returns.
func (q *Queue[K]) AddAfter(k K, d time.Duration) {
	panic("TODO: implement")
}

// AddRateLimited adds k after a per-key exponential backoff:
// baseDelay * 2^n, at most maxDelay, where n is NumRequeues(k) before the
// call. Like AddAfter it does not block, and it increments NumRequeues(k)
// right away.
func (q *Queue[K]) AddRateLimited(k K) {
	panic("TODO: implement")
}

// Forget resets the backoff of k: NumRequeues(k) becomes 0. It does not
// remove k from the queue and does not cancel a pending retry.
func (q *Queue[K]) Forget(k K) {
	panic("TODO: implement")
}

// NumRequeues returns how often AddRateLimited was called for k since the
// last Forget(k).
func (q *Queue[K]) NumRequeues(k K) int {
	panic("TODO: implement")
}

// Len returns the number of keys waiting in the queue. Keys that are
// processing or wait for an AddAfter are not counted.
func (q *Queue[K]) Len() int {
	panic("TODO: implement")
}

// ShutDown makes the queue ignore every later Add, wakes up every blocked
// Get, and lets Get drain the keys that are still waiting. Calling it more
// than once is fine.
func (q *Queue[K]) ShutDown() {
	panic("TODO: implement")
}
