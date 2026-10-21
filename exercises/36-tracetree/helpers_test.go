package tracetree_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tracetree "github.com/yornik/go-practice/exercises/36-tracetree"
)

// t0 is when every test trace starts. The tests give all times in
// milliseconds after t0.
var t0 = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// testTrace is the TraceID of every span the helpers create.
const testTrace = "4bf92f3577b34da6a3ce929d0e0e4736"

// at returns the time n milliseconds after t0. n may be negative.
func at(n int) time.Time { return t0.Add(ms(n)) }

// ms returns n milliseconds as a time.Duration.
func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

// span returns a span of the test trace that runs from start to end, in
// milliseconds after t0. Its Name is "call <id>".
func span(id, parent, service string, start, end int) tracetree.Span {
	return tracetree.Span{
		TraceID:  testTrace,
		SpanID:   id,
		ParentID: parent,
		Service:  service,
		Name:     "call " + id,
		Start:    at(start),
		End:      at(end),
	}
}

// node builds a tree by hand, without Build, so that the tests of SelfTime,
// ServiceTimes and CriticalPath do not depend on your Build. It sets each
// child's ParentID to id. Like Build, it wants the children sorted by Start,
// then by SpanID; it panics if a test passes them in another order.
func node(id, service string, start, end int, children ...*tracetree.Node) *tracetree.Node {
	n := &tracetree.Node{Span: span(id, "", service, start, end)}
	sorted := slices.IsSortedFunc(children, func(a, b *tracetree.Node) int {
		if c := a.Span.Start.Compare(b.Span.Start); c != 0 {
			return c
		}
		return strings.Compare(a.Span.SpanID, b.Span.SpanID)
	})
	if !sorted {
		panic("test bug: children of " + id + " are not sorted by Start, then SpanID")
	}
	for _, c := range children {
		c.Span.ParentID = id
		n.Children = append(n.Children, c)
	}
	return n
}

// shape describes the tree under n by span IDs, children in order:
// "root(a b(b1 b2) c)".
func shape(n *tracetree.Node) string {
	if n == nil {
		return "<nil>"
	}
	if len(n.Children) == 0 {
		return n.Span.SpanID
	}
	parts := make([]string, len(n.Children))
	for i, c := range n.Children {
		parts[i] = shape(c)
	}
	return n.Span.SpanID + "(" + strings.Join(parts, " ") + ")"
}

// shapes returns the shape of every node in nodes.
func shapes(nodes []*tracetree.Node) []string {
	out := []string{}
	for _, n := range nodes {
		out = append(out, shape(n))
	}
	return out
}

// dump describes every field of every span in the tree under n, one line
// per node. Tests compare dumps from before and after a call to check that
// the call did not modify the tree.
func dump(n *tracetree.Node) string {
	var b strings.Builder
	var walk func(n *tracetree.Node, depth int)
	walk = func(n *tracetree.Node, depth int) {
		fmt.Fprintf(&b, "%s%+v\n", strings.Repeat("  ", depth), n.Span)
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(n, 0)
	return b.String()
}

// timeline describes the tree under n for failure messages: one node per
// line, indented by depth, with its times in milliseconds after t0:
//
//	p 0..100
//	  a 10..40
func timeline(n *tracetree.Node) string {
	var b strings.Builder
	var walk func(n *tracetree.Node, depth int)
	walk = func(n *tracetree.Node, depth int) {
		fmt.Fprintf(&b, "\t%s%s %d..%d\n", strings.Repeat("  ", depth), n.Span.SpanID,
			n.Span.Start.Sub(t0).Milliseconds(), n.Span.End.Sub(t0).Milliseconds())
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(n, 0)
	return b.String()
}

// index maps the SpanID of every node in the tree under n to the node.
func index(n *tracetree.Node, into map[string]*tracetree.Node) map[string]*tracetree.Node {
	if into == nil {
		into = map[string]*tracetree.Node{}
	}
	into[n.Span.SpanID] = n
	for _, c := range n.Children {
		index(c, into)
	}
	return into
}

// ids returns the SpanID of every node in nodes.
func ids(nodes []*tracetree.Node) []string {
	out := []string{}
	for _, n := range nodes {
		out = append(out, n.Span.SpanID)
	}
	return out
}

// spanIDs returns the SpanID of every span in spans.
func spanIDs(spans []tracetree.Span) []string {
	out := []string{}
	for _, s := range spans {
		out = append(out, s.SpanID)
	}
	return out
}

// countNodes returns the number of nodes in the tree under n.
func countNodes(n *tracetree.Node) int {
	total := 1
	for _, c := range n.Children {
		total += countNodes(c)
	}
	return total
}

// sameSpan reports whether a and b have the same fields. Times are compared
// with Equal.
func sameSpan(a, b tracetree.Span) bool {
	return a.TraceID == b.TraceID && a.SpanID == b.SpanID && a.ParentID == b.ParentID &&
		a.Service == b.Service && a.Name == b.Name &&
		a.Start.Equal(b.Start) && a.End.Equal(b.End)
}

// checkoutSpans is a slow checkout request, in the order an exporter sends
// spans: a span is sent when it ends, so children come before their parents.
// The tests use readable span IDs instead of 16 hex digits. As a tree, with
// times in ms after t0:
//
//	fe1       frontend   POST /checkout        0-1250
//	  auth1     auth       Authorize            10-40
//	  cart1     cart       GetCart              45-120
//	    redis1    redis      GET                  50-60
//	  inv1      inventory  Reserve             125-400   same Start as price1,
//	    pg1       postgres   UPDATE stock        130-390   and "inv1" < "price1"
//	  price1    pricing    Quote               125-300
//	  pay1      payment    Charge              410-1200
//	    fraud1    fraud      Score               415-500
//	    psp1      payment    POST /v1/capture    500-1190
//	  fe2       frontend   publish receipt    1210-1215
//
//	orphan (its parent notify1 is not in the list):
//	mail1     mailer     SendReceipt        1300-1450
//	  smtp1     mailer     smtp DATA          1310-1440
func checkoutSpans() []tracetree.Span {
	s := func(id, parent, service, name string, start, end int) tracetree.Span {
		sp := span(id, parent, service, start, end)
		sp.Name = name
		return sp
	}
	return []tracetree.Span{
		s("auth1", "fe1", "auth", "Authorize", 10, 40),
		s("redis1", "cart1", "redis", "GET", 50, 60),
		s("cart1", "fe1", "cart", "GetCart", 45, 120),
		s("price1", "fe1", "pricing", "Quote", 125, 300),
		s("pg1", "inv1", "postgres", "UPDATE stock", 130, 390),
		s("inv1", "fe1", "inventory", "Reserve", 125, 400),
		s("fraud1", "pay1", "fraud", "Score", 415, 500),
		s("psp1", "pay1", "payment", "POST /v1/capture", 500, 1190),
		s("pay1", "fe1", "payment", "Charge", 410, 1200),
		s("fe2", "fe1", "frontend", "publish receipt", 1210, 1215),
		s("fe1", "", "frontend", "POST /checkout", 0, 1250),
		s("smtp1", "mail1", "mailer", "smtp DATA", 1310, 1440),
		s("mail1", "notify1", "mailer", "SendReceipt", 1300, 1450),
	}
}

// mustBuild calls Build and fails the test if it returns an error.
func mustBuild(t *testing.T, spans []tracetree.Span) *tracetree.Trace {
	t.Helper()
	tr, err := tracetree.Build(spans)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if tr == nil || tr.Root == nil {
		t.Fatalf("Build() = %+v, want a trace with a Root", tr)
	}
	return tr
}
