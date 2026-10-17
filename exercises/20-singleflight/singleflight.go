// Package singleflight coalesces concurrent calls for the same key: while a
// call for a key is in flight, other calls with that key wait for it and
// share its result instead of doing the same work again.
package singleflight

import (
	"context"
	"fmt"
)

// PanicError is a panic in fn that was recovered and passed on to the
// callers. It is given ready-made: you create it, Error and Unwrap are done.
type PanicError struct {
	// Value is the value fn passed to panic.
	Value any
	// Stack is the stack trace of the goroutine in which fn panicked,
	// captured with runtime/debug.Stack inside the deferred function that
	// recovers, so it shows where the panic happened.
	Stack []byte
}

// Error returns "panic: " followed by Value formatted with %v. It does not
// include the stack.
func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

// Unwrap returns Value if it is an error, and nil otherwise, so errors.Is
// and errors.As see through a PanicError (for example to a runtime.Error).
func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// Result is what DoChan delivers: the values Do would return.
type Result[V any] struct {
	Val    V
	Err    error
	Shared bool
}

// Group coalesces calls by key. One run of fn for a key is an execution.
// While an execution for a key is in flight, a call with the same key joins
// it instead of running its own fn, and receives the execution's result.
// Calls with different keys never wait for each other.
//
// The zero value is ready to use. A Group is safe for concurrent use. Do not
// copy a Group after first use.
type Group[K comparable, V any] struct {
	// TODO: add fields.
}

// Do runs fn and returns what it returned, unless an execution for key is
// already in flight: then Do waits for that execution and returns its
// result, and fn is never called. shared is true for every caller of an
// execution that had more than one caller.
//
// The execution leaves the group before any caller receives its result, so
// a call that starts after that runs fn again: nothing is cached.
//
// If fn panics, every Do and DoContext caller of that execution panics with
// the same *PanicError.
//
// fn must not call the group with its own key: it would wait for itself.
func (g *Group[K, V]) Do(key K, fn func() (V, error)) (v V, err error, shared bool) {
	panic("TODO: implement")
}

// DoChan is like Do, but it returns at once with a channel that receives the
// result when the execution finishes. It starts or joins the execution for
// key before it returns, so a call with key after DoChan has returned joins
// the same execution while it is in flight. The channel receives exactly one
// Result. The result is delivered even if nobody ever reads the channel, so
// a caller may stop waiting on it at any time without leaking a goroutine or
// blocking other callers.
//
// If fn panics, the Result has Err set to the *PanicError and a zero Val.
func (g *Group[K, V]) DoChan(key K, fn func() (V, error)) <-chan Result[V] {
	panic("TODO: implement")
}

// Forget makes the group forget the execution in flight for key, if there
// is one. The next call with key starts a new execution. Callers that
// already wait for the old execution still receive its result. Forget on a
// key with no execution in flight does nothing.
func (g *Group[K, V]) Forget(key K) {
	panic("TODO: implement")
}

// DoContext is like Do, but every caller waits only as long as its own ctx
// allows, and fn gets a context of its own.
//
// If ctx is done before the result arrives, DoContext returns the zero V,
// ctx.Err() and false at once, even if fn is still running. If ctx is
// already done when DoContext is called, it returns that way without
// starting or joining an execution.
//
// fn's context has the values of the ctx of the caller that started the
// execution, but not its deadline or cancellation. It is cancelled when every
// caller of the execution has left (Do and DoChan callers never leave), and
// when fn returns. When the last caller leaves, the execution also leaves
// the group, so the next call starts a new execution; the old execution's
// result, or panic, is dropped.
func (g *Group[K, V]) DoContext(ctx context.Context, key K, fn func(ctx context.Context) (V, error)) (v V, err error, shared bool) {
	panic("TODO: implement")
}
