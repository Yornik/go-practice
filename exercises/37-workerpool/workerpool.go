// Package workerpool runs many independent calls with bounded parallelism.
// Map applies a function to every item of a slice, returns the results in
// input order and stops at the first failure. Group is a small version of
// golang.org/x/sync/errgroup: run functions in goroutines, optionally with a
// limit, wait for all of them and keep the first error.
package workerpool

import "context"

// PanicError is a panic in a task (a call of fn in Map, or a function passed
// to Group.Go or Group.TryGo) that was recovered and turned into an error.
type PanicError struct {
	// Value is the value the task passed to panic.
	Value any
	// Stack is the stack trace of the goroutine that panicked, captured with
	// runtime/debug.Stack while recovering, so it shows where the panic
	// happened.
	Stack []byte
}

// Error returns "panic: " followed by Value formatted with %v, for example
// "panic: boom". It does not include the stack.
func (e *PanicError) Error() string {
	panic("TODO: implement")
}

// Unwrap returns Value if Value is an error, and nil otherwise. Runtime
// panics such as a write to a nil map are values of type runtime.Error, so
// this lets callers use errors.Is and errors.As on them.
func (e *PanicError) Unwrap() error {
	panic("TODO: implement")
}

// Map calls fn once for every item of in, with at most workers calls running
// at the same time, and returns the results in input order: result[i] is the
// value fn returned for in[i]. The order in which the calls start is not
// specified. workers < 1 is an error, also for empty input, and fn is not
// called.
//
// fn receives a context derived from ctx (same values and deadline). fn is
// called from several goroutines at once, so it must be safe for that.
//
// Map fails fast. When a call returns an error or panics, Map cancels the
// context it passes to fn with that error as the cause, starts no new calls,
// waits for the running calls to return, and returns a nil slice and that
// first error. A panic is returned as a *PanicError.
//
// If ctx ends before every item has been processed, Map also starts no new
// calls, waits for the running ones and returns a nil slice and an error: the
// first error of a failed call if there is one (even if ctx ended later),
// otherwise ctx.Err().
//
// Map returns a nil error only if fn returned nil for every item. Empty input
// gives an empty, non-nil slice and a nil error, even if ctx is already done.
//
// When Map returns, no call of fn is still running, no goroutine started by
// Map is left, and the context passed to fn is cancelled.
func Map[T, R any](ctx context.Context, in []T, workers int, fn func(ctx context.Context, item T) (R, error)) ([]R, error) {
	panic("TODO: implement")
}
