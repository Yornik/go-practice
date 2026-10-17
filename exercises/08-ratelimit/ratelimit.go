// Package ratelimit implements token-bucket rate limiters: Limiter for one
// stream of requests, and Keyed for one bucket per client, with a Sweep that
// keeps the map of clients from growing forever.
package ratelimit

import (
	"errors"
	"time"
)

// Errors returned by Limiter.Reserve. Both mean that waiting will never help:
// the caller should reject the request instead.
var (
	// ErrExceedsBurst: n is larger than the burst. The bucket never holds
	// that many tokens, so n tokens at once are never allowed.
	ErrExceedsBurst = errors.New("ratelimit: n exceeds burst")
	// ErrNoRefill: the rate is 0 and fewer than n tokens are left. The
	// bucket never refills, so the tokens never come.
	ErrNoRefill = errors.New("ratelimit: rate is 0 and too few tokens are left")
)

// Limiter is a token bucket. It holds up to burst tokens and gains rate tokens
// per second. A request takes tokens; when there are not enough, it is denied.
//
// A Limiter has no goroutine and no timer: every method computes the refill
// from the time that passed since the previous call. All methods are safe for
// concurrent use.
type Limiter struct {
	// TODO: add fields.
}

// New returns a Limiter that gains rate tokens per second and holds at most
// burst tokens. It starts full: Tokens() == burst.
//
// now is the clock. nil means time.Now. Tests pass a fake clock.
//
// rate must be a finite number >= 0 (0 means the bucket never refills) and
// burst must be >= 1. New panics otherwise: a wrong rate limit is a
// configuration error that should stop the service at startup.
func New(rate float64, burst int, now func() time.Time) *Limiter {
	panic("TODO: implement")
}

// Allow reports whether one request may happen now. It is AllowN(1).
func (l *Limiter) Allow() bool {
	panic("TODO: implement")
}

// AllowN reports whether n requests may happen now. If at least n tokens are
// available, it takes n tokens and returns true. Otherwise it takes nothing
// and returns false: all or nothing. n must be >= 1.
func (l *Limiter) AllowN(n int) bool {
	panic("TODO: implement")
}

// Tokens returns the number of tokens available now, after the refill. It
// has a fraction when the refill does, and it is negative while reservations
// are outstanding (see Reserve).
func (l *Limiter) Tokens() float64 {
	panic("TODO: implement")
}

// Reserve takes n tokens now, even if that makes the token count negative,
// and returns how long the caller must wait before it acts: 0 if the count is
// still >= 0 afterwards, otherwise the time the refill needs to bring the
// count back to 0. Later calls wait behind earlier ones.
//
// If n > burst, Reserve returns 0 and ErrExceedsBurst. If the rate is 0 and
// fewer than n tokens are available, it returns 0 and ErrNoRefill. In both
// cases it takes nothing. n must be >= 1.
func (l *Limiter) Reserve(n int) (time.Duration, error) {
	panic("TODO: implement")
}

// Cancel gives n tokens back, for example for a reservation that was not
// used. The count never goes above burst. n must be >= 1.
func (l *Limiter) Cancel(n int) {
	panic("TODO: implement")
}

// Keyed holds one token bucket per key (a client ID, an IP address), each
// with the same rate and burst. A bucket is created full on the first
// Allow(key). Sweep removes buckets that are no longer needed. All methods are
// safe for concurrent use.
type Keyed struct {
	// TODO: add fields.
}

// NewKeyed returns a Keyed whose buckets gain rate tokens per second and hold
// at most burst tokens. Sweep removes buckets that have not been used for at
// least idle. now is the clock; nil means time.Now.
//
// NewKeyed panics on the same rate and burst values as New, and if idle < 0.
func NewKeyed(rate float64, burst int, idle time.Duration, now func() time.Time) *Keyed {
	panic("TODO: implement")
}

// Allow reports whether one request for key may happen now, like
// Limiter.Allow on key's bucket. It creates the bucket, full, if key has none.
// Every call counts as a use of key, also when it returns false.
func (k *Keyed) Allow(key string) bool {
	panic("TODO: implement")
}

// Len returns the number of buckets that Keyed holds.
func (k *Keyed) Len() int {
	panic("TODO: implement")
}

// Sweep removes every bucket that is both
//
//   - idle: its key was last used at least idle ago, and
//   - full: its token count, refilled up to now, equals burst,
//
// and returns how many it removed. Sweep itself does not count as a use.
func (k *Keyed) Sweep() int {
	panic("TODO: implement")
}
