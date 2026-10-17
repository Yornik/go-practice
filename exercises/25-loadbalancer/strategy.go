package loadbalancer

import "net/url"

// Backend is one upstream server behind the LB: its base URL and the number
// of requests in flight to it. The LB keeps its own Backends; NewBackend,
// Begin and End are exported so that a Strategy can be tested on its own.
type Backend struct {
	// TODO: add fields.
}

// NewBackend parses rawURL and returns a Backend with nothing in flight.
// rawURL must be an absolute http or https URL with a host, such as
// "http://10.0.0.7:8080"; anything else is an error.
func NewBackend(rawURL string) (*Backend, error) {
	panic("TODO: implement")
}

// URL returns the backend's base URL. The caller must not modify it.
func (b *Backend) URL() *url.URL {
	panic("TODO: implement")
}

// InFlight returns the number of requests to b that have begun and not yet
// ended. Safe for concurrent use.
func (b *Backend) InFlight() int64 {
	panic("TODO: implement")
}

// Begin records that a request to b has started: InFlight goes up by 1.
// Safe for concurrent use.
func (b *Backend) Begin() {
	panic("TODO: implement")
}

// End records that a request to b has finished: InFlight goes down by 1.
// Safe for concurrent use.
func (b *Backend) End() {
	panic("TODO: implement")
}

// Strategy chooses the backend for the next request.
type Strategy interface {
	// Pick returns one element of bs. bs holds only the backends that may
	// get a request right now, in the order of Config.Backends, and has at
	// least 1 element. Pick must not modify bs, and must be safe for
	// concurrent use: the LB calls it from many goroutines at once.
	Pick(bs []*Backend) *Backend
}

// RoundRobin returns a new Strategy that takes turns: its k-th call of Pick
// (counting from 0) returns bs[k % len(bs)]. Every RoundRobin() has its own
// counter.
func RoundRobin() Strategy {
	panic("TODO: implement")
}

// LeastConnections returns a Strategy that picks the backend with the fewest
// requests in flight. On a tie it picks the one that comes first in bs.
func LeastConnections() Strategy {
	panic("TODO: implement")
}

// PowerOfTwo returns a Strategy that draws two different backends at random
// and picks the one with fewer requests in flight ("the power of two random
// choices").
//
// With n = len(bs) > 1, Pick calls randIntN exactly twice. The first
// candidate is bs[randIntN(n)]. The second candidate is chosen from the n-1
// other backends, in list order without the first candidate: it is the
// randIntN(n-1)-th of them (counting from 0). On a tie the first candidate
// wins. With n == 1, Pick returns bs[0] and does not call randIntN.
//
// randIntN(n) must return a number in [0, n) and be safe for concurrent use.
// nil means math/rand/v2.IntN.
func PowerOfTwo(randIntN func(n int) int) Strategy {
	panic("TODO: implement")
}
