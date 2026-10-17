// Package tracetree turns the flat list of spans of one distributed trace
// into a tree and answers where the time went: self time per span and per
// service, the critical path, and a plain-text rendering for a terminal.
package tracetree

import (
	"errors"
	"time"
)

// Span is one timed operation of a trace, as exported by Jaeger, Tempo or an
// OpenTelemetry collector.
type Span struct {
	TraceID  string // the same for every span of one trace
	SpanID   string // unique within the trace; never empty
	ParentID string // SpanID of the span that called this one; "" for the root span
	Service  string // the service that recorded the span, for example "payment"
	Name     string // the operation, for example "POST /checkout"
	Start    time.Time
	End      time.Time
}

// Node is a span in the tree.
type Node struct {
	Span Span
	// Children are the nodes whose ParentID is Span.SpanID, sorted by
	// Span.Start, then by Span.SpanID. Nil or empty for a leaf.
	Children []*Node
}

// Duration returns how long the span took: End minus Start.
func (n *Node) Duration() time.Duration {
	return n.Span.End.Sub(n.Span.Start)
}

// Trace is a trace as a tree.
type Trace struct {
	// Root is the node of the span with an empty ParentID. Build always
	// sets it.
	Root *Node
	// Orphans are the nodes of spans whose ParentID is not empty but names
	// no span of the trace, each with its own subtree, sorted by Span.Start,
	// then by Span.SpanID. Nil or empty if there are none.
	Orphans []*Node
}

// The errors of Build. Build wraps them with details, so test for them with
// errors.Is.
var (
	ErrNoSpans          = errors.New("tracetree: no spans")
	ErrMixedTraces      = errors.New("tracetree: spans of more than one trace")
	ErrDuplicateSpanID  = errors.New("tracetree: duplicate span ID")
	ErrNegativeDuration = errors.New("tracetree: span ends before it starts")
	ErrNoRoot           = errors.New("tracetree: no root span")
	ErrMultipleRoots    = errors.New("tracetree: more than one root span")
)

// Build links the spans of one trace into a tree. The spans may come in any
// order; children often come before their parents. Every span becomes exactly
// one Node: the root, a node below it, an orphan, or a node below an orphan.
// Node.Span is the input span, unchanged. Build does not modify spans.
//
// On invalid input Build returns a nil *Trace and an error that wraps one of
// the Err* values above. The README lists the cases and what the message must
// name.
func Build(spans []Span) (*Trace, error) {
	panic("TODO: implement")
}
