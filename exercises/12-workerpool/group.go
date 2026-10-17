package workerpool

import "context"

// Group runs functions in their own goroutines, optionally with a limit on
// how many run at the same time, waits for all of them and keeps the first
// error. It is a small version of golang.org/x/sync/errgroup.Group.
//
// The zero value is ready to use: it has no limit and no context to cancel.
// A Group is safe for concurrent use. Do not copy a Group after first use.
type Group struct {
	// TODO: add fields.
}

// WithContext returns a new Group and a context derived from ctx. The
// derived context is cancelled the first time a function started by the
// group returns a non-nil error or panics (with that error as the cause), or
// when Wait returns, whichever happens first. It is not cancelled before
// that, unless ctx itself ends.
func WithContext(ctx context.Context) (*Group, context.Context) {
	panic("TODO: implement")
}

// SetLimit allows at most n functions of the group to run at the same time.
// A negative n means no limit. With n == 0 no function can start: Go blocks
// forever and TryGo returns false.
//
// Call SetLimit before the first call to Go or TryGo. Changing the limit
// while functions are running is a programming error.
func (g *Group) SetLimit(n int) {
	panic("TODO: implement")
}

// Go runs f in a new goroutine. While the limit is reached, Go blocks until a
// running function returns.
//
// Go starts f even if another function has already failed: the group keeps
// only the first error and never skips work on its own. f can check the
// context from WithContext to skip its work.
//
// A panic in f is recovered in f's goroutine and counts as an error of type
// *PanicError.
func (g *Group) Go(f func() error) {
	panic("TODO: implement")
}

// TryGo runs f in a new goroutine only if the limit allows it right now. It
// never blocks, also when other goroutines call TryGo at the same time:
// checking for a free slot and taking it is one step. It reports whether f
// was started. Without a limit it always starts f. Errors and panics are
// handled as in Go.
func (g *Group) TryGo(f func() error) bool {
	panic("TODO: implement")
}

// Wait blocks until every function started with Go or TryGo has returned.
// It returns the first non-nil error any of them returned, as is (not
// wrapped; a panic counts as a *PanicError), or nil if all returned nil.
// When Wait returns, the context from WithContext is cancelled.
//
// Call Wait after your last call to Go or TryGo.
func (g *Group) Wait() error {
	panic("TODO: implement")
}
