// Package logmerge turns the logs of many pods into one stream in time order.
// Merge merges streams that are each in time order, Reorder repairs a stream
// that is slightly out of order, and ParseKubectl reads the output of
// kubectl logs --timestamps. All three work lazily on sequences (package iter),
// so they also work on endless streams such as kubectl logs -f.
package logmerge

import (
	"iter"
	"time"
)

// Entry is one log line.
type Entry struct {
	Time   time.Time // when the line was written
	Source string    // where it comes from, for example the pod name
	Line   string    // the message, without the timestamp and without the newline
}

// Merge merges sources into one sequence in time order.
//
// Each source must yield its entries in time order (non-decreasing Time).
// Merge does not check this and does not repair it; Reorder does that.
//
// The result yields every entry of every source exactly once, ordered by Time
// (compared as instants). Entries with equal Time come out in the order of
// their sources in the argument list, and in their own order within one
// source. In other words: the result is a stable sort by Time of all sources
// concatenated in argument order.
//
// Merge is lazy. Calling it does not touch the sources. While you range over
// the result, at most one entry per source has been pulled from that source
// and not yet yielded, and Merge pulls the next entry of a source only after
// the loop body for its previous entry returned. Every range over the result
// ranges over the sources again.
//
// When the loop over the result stops early (break or return), Merge stops
// every source it started before the loop ends: the source's yield returns
// false, so the source returns and its deferred calls run.
func Merge(sources ...iter.Seq[Entry]) iter.Seq[Entry] {
	panic("TODO: implement")
}
