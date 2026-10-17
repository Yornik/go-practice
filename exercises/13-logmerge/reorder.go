package logmerge

import (
	"iter"
	"time"
)

// ReorderStats counts what Reorder did.
type ReorderStats struct {
	// Late is the number of entries that came out after an entry with a later
	// Time: they arrived too late for the window to put them in order.
	Late int
}

// Reorder puts a stream that is slightly out of order back into time order,
// with a bounded window.
//
// Reorder buffers the entries it pulls from src. Let newest be the latest Time
// of all entries pulled so far. After each pull, Reorder yields, oldest first,
// every buffered entry whose Time is at least window before newest. Then it
// pulls the next entry. When src ends, it yields the rest of the buffer, oldest
// first. Entries with equal Time come out in the order they arrived. With
// window <= 0, every entry is yielded as soon as it is pulled.
//
// An entry is late if its Time is before the Time of an entry that Reorder
// already yielded. Reorder still yields it. If stats is not nil, Reorder adds 1
// to stats.Late before it yields a late entry. stats may be nil.
//
// Reorder is lazy: calling it does not touch src. When the loop over the result
// stops early, Reorder stops src (src's deferred calls run before the loop
// ends) and drops the entries it still buffers.
func Reorder(src iter.Seq[Entry], window time.Duration, stats *ReorderStats) iter.Seq[Entry] {
	panic("TODO: implement")
}
