// Package counters keeps named counters in memory, such as the number of
// photos uploaded per region. Many goroutines can count at the same time,
// and a scraper reads or resets all counters at once.
package counters

import "io"

// Registry holds named counters. Create it with NewRegistry. All methods are
// safe for concurrent use.
type Registry struct {
	// TODO: add fields.
}

// NewRegistry returns an empty Registry. Each Registry has its own counters.
func NewRegistry() *Registry {
	panic("TODO: implement")
}

// Counter returns the counter for name, created with the value 0 on first
// use. The same name always gives the same *Counter: also when many
// goroutines ask for a new name at the same moment, and also after Drain.
// Names are not checked: any string, "" included, is a name. A counter is
// never removed.
func (r *Registry) Counter(name string) *Counter {
	panic("TODO: implement")
}

// Counter is a value that only goes up, until Drain sets it to 0. Get one
// from Registry.Counter. All methods are safe for concurrent use.
type Counter struct {
	// TODO: add fields.
}

// Add adds n to the counter. Add(0) changes nothing. Add panics if n < 0,
// because a counter only goes up; the value does not change, and the
// counter keeps working after the panic.
func (c *Counter) Add(n int64) {
	panic("TODO: implement")
}

// Value returns the current value of the counter.
func (c *Counter) Value() int64 {
	panic("TODO: implement")
}

// Sample is the value of 1 counter at the moment it was read.
type Sample struct {
	Name  string
	Value int64
}

// Snapshot returns 1 Sample per counter, sorted by Name in Go's string
// order. Counters with the value 0 are included. The slice is a copy that
// belongs to the caller. For an empty registry the slice has length 0.
func (r *Registry) Snapshot() []Sample {
	panic("TODO: implement")
}

// Drain returns the same as Snapshot and sets every counter to 0. For each
// counter, reading the value and setting it to 0 are 1 step: an Add that
// runs at the same time counts either in the result of Drain or in the
// counter afterwards, never in both and never in neither. The counters stay
// in the registry.
func (r *Registry) Drain() []Sample {
	panic("TODO: implement")
}

// Write writes 1 line "<name> <value>\n" per counter to w, sorted by name,
// with the values of 1 snapshot taken when Write starts. It does not change
// any counter. It holds no lock while it calls w.Write, so a slow w never
// blocks other methods of the registry or its counters. When a call of
// w.Write fails, Write makes no more calls and returns that error (as is or
// wrapped with %w).
func (r *Registry) Write(w io.Writer) error {
	panic("TODO: implement")
}
