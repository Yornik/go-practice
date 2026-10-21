package tracetree

import "time"

// The functions below take a valid tree: what Build returns, or a tree built
// by hand with the same properties (a non-nil Root, End never before Start,
// Children sorted by Start, then by SpanID). They may rely on that, and they
// never modify the tree.

// SelfTime returns the part of n's duration that none of n's direct children
// covers: the time n spent on its own, not waiting for a child. A child counts
// only inside n's own Start..End, and time covered by several children at
// once counts once. The result is between 0 and n.Duration().
func SelfTime(n *Node) time.Duration {
	panic("TODO: implement")
}

// ServiceTimes returns the self time of every node of the trace, in the root
// tree and in the orphan trees, summed per Span.Service. Every service that
// appears in the trace is a key, also when its sum is 0.
func ServiceTimes(t *Trace) map[string]time.Duration {
	panic("TODO: implement")
}

// CriticalPath returns the nodes of the root tree that decided how long the
// trace took, root first, as defined in the README. Orphans are never on it.
// The returned pointers point to nodes of t.
func CriticalPath(t *Trace) []*Node {
	panic("TODO: implement")
}
