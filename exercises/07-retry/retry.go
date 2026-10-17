// Package retry calls an operation again when it fails, with exponential
// backoff, optional full jitter, Retry-After support and context
// cancellation. The policy (how long to wait) is pure and testable; Do and
// DoValue run the loop.
package retry

import (
	"context"
	"errors"
	"time"
)

// ErrInvalidPolicy is wrapped by every error that Policy.Validate returns.
var ErrInvalidPolicy = errors.New("retry: invalid policy")

// Policy says how often and how long to retry. A Policy is a plain value: it
// is safe to share one Policy between goroutines as long as Sleep and Rand are
// safe for concurrent use (the defaults are).
type Policy struct {
	// MaxAttempts is the total number of calls of op, including the first
	// one. 1 means "no retries". It must be >= 1.
	MaxAttempts int

	// Initial is the backoff after the first failed attempt. It must be > 0.
	Initial time.Duration

	// Max is the upper limit for every delay, including a delay that op asks
	// for with a RetryAfterError. It must be >= Initial.
	Max time.Duration

	// Multiplier is the growth factor of the backoff from one attempt to the
	// next. It must be >= 1; 1 gives a constant backoff.
	Multiplier float64

	// Jitter turns on full jitter: the delay is a random duration in
	// [0, Backoff(n)) instead of Backoff(n).
	Jitter bool

	// Sleep waits for d or until ctx is done, whichever comes first. It
	// returns nil after a full wait and a non-nil error (ctx.Err() for the
	// default) when the wait was cut short. nil means a real timer that
	// honours ctx.
	Sleep func(ctx context.Context, d time.Duration) error

	// Rand returns a random number in [0, 1). It is only used with Jitter.
	// nil means the top-level functions of math/rand/v2.
	Rand func() float64
}

// Validate reports whether the policy can be used. It returns nil, or an
// error that wraps ErrInvalidPolicy when MaxAttempts < 1, Initial <= 0,
// Multiplier < 1 or Max < Initial. Sleep, Rand and Jitter are not checked.
func (p Policy) Validate() error {
	panic("TODO: implement")
}

// Backoff returns the delay after failed attempt number attempt (1-based),
// without jitter: Initial * Multiplier^(attempt-1), but never more than Max.
// The result is truncated to the nanosecond. It never overflows: a huge
// attempt number gives Max, not a negative or garbage duration. Backoff
// ignores Jitter; with Jitter, Do uses Backoff as the upper limit of the
// random delay. The result is only meaningful for a valid policy and
// attempt >= 1.
func (p Policy) Backoff(attempt int) time.Duration {
	panic("TODO: implement")
}

// Permanent marks err as not worth retrying. When op returns it (or an error
// that wraps it), Do stops at once and returns err itself, without the mark.
// The returned error has the same message as err and wraps it, so errors.Is
// and errors.As see err. Permanent(nil) returns nil.
func Permanent(err error) error {
	panic("TODO: implement")
}

// RetryAfterError is returned by op to ask for a specific delay before the
// next attempt, typically from an HTTP 429 or 503 response with a
// Retry-After header. Do uses After instead of the backoff, without jitter,
// clamped to [0, Policy.Max].
type RetryAfterError struct {
	Err   error         // the failure itself, for example "503 Service Unavailable"
	After time.Duration // how long the server asked us to wait
}

// Error returns Err's message followed by the delay in parentheses, for
// example "503 Service Unavailable (retry after 2s)". The delay is formatted
// with time.Duration's String method.
func (e *RetryAfterError) Error() string {
	panic("TODO: implement")
}

// Unwrap returns Err.
func (e *RetryAfterError) Unwrap() error {
	panic("TODO: implement")
}

// ExhaustedError is returned by Do when every attempt failed.
type ExhaustedError struct {
	Attempts int   // how many times op was called
	Last     error // the error of the last attempt, as op returned it
}

// Error returns "giving up after <Attempts> attempts: <Last>", for example
// "giving up after 3 attempts: connection refused".
func (e *ExhaustedError) Error() string {
	panic("TODO: implement")
}

// Unwrap returns Last.
func (e *ExhaustedError) Unwrap() error {
	panic("TODO: implement")
}

// Do calls op until it succeeds, returns a permanent error, the attempts run
// out or ctx is done. It waits between two attempts with p.Sleep: the delay
// after failed attempt n is the RetryAfterError's After if op asked for one,
// else Backoff(n), with full jitter if p.Jitter is set. It calls p.Sleep
// exactly once between two attempts, also for a delay of 0, and never before
// the first attempt or after the last one.
//
// Do returns:
//   - nil as soon as op returns nil;
//   - the error passed to Permanent, as soon as op returns a permanent error;
//   - a *ExhaustedError when MaxAttempts calls all failed;
//   - the error of p.Validate, before op is ever called, if p is invalid;
//   - when ctx is done before an attempt, or Sleep fails: an error that
//     matches the cause (ctx.Err() or Sleep's error) and the last error of op
//     with errors.Is. If op was never called, it matches ctx.Err().
//
// op receives ctx. op is never called once ctx is done.
func Do(ctx context.Context, p Policy, op func(ctx context.Context) error) error {
	panic("TODO: implement")
}

// DoValue is Do for an operation that returns a value. On success it returns
// the value of the successful call. On any error it returns the zero value of
// T, even if op returned a value together with its error.
func DoValue[T any](ctx context.Context, p Policy, op func(ctx context.Context) (T, error)) (T, error) {
	panic("TODO: implement")
}
