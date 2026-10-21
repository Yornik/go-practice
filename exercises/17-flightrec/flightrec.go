// Package flightrec keeps the most recent values in memory: a ring of fixed
// size for 1 stream of values, and a recorder that keeps 1 ring per key.
package flightrec

// Ring keeps the last values added to it, up to a fixed size. When it is
// full, each new value overwrites the oldest one. Create it with NewRing; the
// zero value of Ring is not ready for use. A Ring is not safe for concurrent
// use: a caller that shares one between goroutines must lock around it.
type Ring[T any] struct {
	// TODO: add fields.
}

// NewRing returns an empty ring that holds at most size values. It panics if
// size < 1: a ring that can hold nothing is a configuration bug, and it
// should fail at startup.
func NewRing[T any](size int) *Ring[T] {
	panic("TODO: implement")
}

// Add stores v as the newest value. If the ring already holds size values,
// v overwrites the oldest one, and Dropped grows by 1. Add takes O(1) time:
// its cost does not grow with the size of the ring.
func (r *Ring[T]) Add(v T) {
	panic("TODO: implement")
}

// Len returns the number of values the ring holds: the number of Add calls,
// but at most size.
func (r *Ring[T]) Len() int {
	panic("TODO: implement")
}

// Dropped returns the number of values that Add has overwritten so far.
// Len() + Dropped() is the number of Add calls.
func (r *Ring[T]) Dropped() int {
	panic("TODO: implement")
}

// Oldest returns the oldest value the ring holds and true. On an empty ring
// it returns the zero value of T and false.
func (r *Ring[T]) Oldest() (T, bool) {
	panic("TODO: implement")
}

// Newest returns the value added last and true. On an empty ring it returns
// the zero value of T and false.
func (r *Ring[T]) Newest() (T, bool) {
	panic("TODO: implement")
}

// Last returns the n newest values, oldest first, or all values if the ring
// holds fewer than n. For n <= 0, and on an empty ring, it returns no values
// (nil or an empty slice). The result is a new slice: changing it, appending
// to it or adding to the ring later never changes the other.
func (r *Ring[T]) Last(n int) []T {
	panic("TODO: implement")
}

// Recorder keeps the last values per key: each key has its own ring of the
// same size, so the values of one key never push out the values of another.
// Create it with NewRecorder. A Recorder is not safe for concurrent use.
type Recorder[K comparable, T any] struct {
	// TODO: add fields.
}

// NewRecorder returns an empty recorder that keeps at most size values per
// key. It panics if size < 1.
func NewRecorder[K comparable, T any](size int) *Recorder[K, T] {
	panic("TODO: implement")
}

// Add adds v to the ring of key. The first Add for a key creates its ring.
func (rec *Recorder[K, T]) Add(key K, v T) {
	panic("TODO: implement")
}

// Dump returns all values held for key, oldest first, in a new slice that
// does not share memory with the recorder. It returns nil if key has no
// values: it was never added, or it was forgotten.
func (rec *Recorder[K, T]) Dump(key K) []T {
	panic("TODO: implement")
}

// Forget removes key and its values, and reports whether key was there. A
// later Add for key starts with an empty ring.
func (rec *Recorder[K, T]) Forget(key K) bool {
	panic("TODO: implement")
}

// Len returns the number of keys that hold values.
func (rec *Recorder[K, T]) Len() int {
	panic("TODO: implement")
}
