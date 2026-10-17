package retry

import "time"

// ParseRetryAfter parses the value of an HTTP Retry-After header (RFC 9110,
// section 10.2.3) and returns how long to wait, measured from now.
//
// The value is either a number of seconds (digits only, for example "120") or
// an HTTP-date in any of the 3 formats HTTP/1.1 allows, for example
// "Wed, 21 Oct 2026 07:28:00 GMT". A date in the past gives 0 and true: retry
// now. Anything else, including a number of seconds too large for a
// time.Duration, gives 0 and false.
func ParseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	panic("TODO: implement")
}
