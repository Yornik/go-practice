package tracetree_test

import (
	"slices"
	"testing"
	"time"

	tracetree "github.com/yornik/go-practice/exercises/36-tracetree"
)

func TestCriticalPath(t *testing.T) {
	cases := []struct {
		name string
		root *tracetree.Node
		want []string // span IDs, in the order CriticalPath returns them
	}{
		{
			name: "single span",
			root: node("r", "api", 0, 100),
			want: []string{"r"},
		},
		{
			// Cursor 100: c. Cursor 60: b. Cursor 30: a. Cursor 0: nothing.
			name: "sequential children",
			root: node("r", "api", 0, 100,
				node("a", "auth", 0, 30), node("b", "cart", 30, 60), node("c", "payment", 60, 100)),
			want: []string{"r", "a", "b", "c"},
		},
		{
			// r waited for b, which ended last. When the cursor moves to b's
			// start (10), a does not start before 10.
			name: "parallel children: only the one that ends last",
			root: node("r", "api", 0, 100,
				node("a", "pricing", 10, 50), node("b", "inventory", 10, 90)),
			want: []string{"r", "b"},
		},
		{
			name: "nested: the path goes down into the picked child",
			root: node("r", "api", 0, 100,
				node("a", "payment", 10, 90,
					node("a1", "fraud", 20, 40), node("a2", "psp", 40, 80))),
			want: []string{"r", "a", "a1", "a2"},
		},
		{
			// The README example.
			name: "checkout: sequential, parallel and nested calls",
			root: node("checkout", "frontend", 0, 500,
				node("auth", "auth", 5, 25),
				node("cart", "cart", 30, 80),
				node("inventory", "inventory", 85, 200,
					node("db", "postgres", 90, 190)),
				node("pricing", "pricing", 85, 150),
				node("payment", "payment", 210, 480,
					node("fraud", "fraud", 215, 300),
					node("psp", "payment", 300, 470))),
			want: []string{"checkout", "auth", "cart", "inventory", "db", "payment", "fraud", "psp"},
		},
		{
			// Cursor 100: b. Cursor 10: a starts at 0, before the cursor, so
			// it is a candidate; its End counts as 10.
			name: "a child that overlaps the picked one counts up to the cursor",
			root: node("r", "api", 0, 100,
				node("a", "db", 0, 90), node("b", "cache", 10, 100)),
			want: []string{"r", "a", "b"},
		},
		{
			// Both end at 80: "a" < "b", so a is picked, although b started
			// first. Cursor 20: b starts at 10, so b is picked next.
			name: "tie on End: smallest SpanID, even if it started later",
			root: node("r", "api", 0, 100,
				node("b", "db", 10, 80), node("a", "db", 20, 80)),
			want: []string{"r", "b", "a"},
		},
		{
			// Both end at 80: a is picked. Cursor 10: b starts at 20, too late.
			name: "tie on End: smallest SpanID, even if it comes first in Children",
			root: node("r", "api", 0, 100,
				node("a", "db", 10, 80), node("b", "db", 20, 80)),
			want: []string{"r", "a"},
		},
		{
			// b ends at 130, after r. Its End counts as 100 (the cursor), so
			// b is picked first, and the path inside b ends at 100: b2 starts
			// at 105, which is not before 100.
			name: "a child that ends after its parent is cut at the cursor",
			root: node("r", "api", 0, 100,
				node("a", "auth", 10, 60),
				node("b", "queue", 50, 130,
					node("b1", "worker", 60, 90), node("b2", "worker", 105, 125))),
			want: []string{"r", "a", "b", "b1"},
		},
		{
			// a ends at 50, before the cursor (100), so the path inside a
			// stops at 50. a2 is work that a started but did not wait for:
			// it starts at 60, after a returned, so it is not on the path.
			name: "the path inside a child stops at the child's End",
			root: node("r", "api", 0, 100,
				node("a", "api", 10, 50,
					node("a1", "db", 15, 40), node("a2", "queue", 60, 80))),
			want: []string{"r", "a", "a1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := &tracetree.Trace{Root: tc.root}
			path := criticalPath(t, tr)
			if got := ids(path); !slices.Equal(got, tc.want) {
				t.Fatalf("CriticalPath() = %q, want %q\ntree, times in ms:\n%s", got, tc.want, timeline(tc.root))
			}
			// The path holds the tree's own nodes, not copies.
			nodes := index(tc.root, nil)
			for i, n := range path {
				if n != nodes[n.Span.SpanID] {
					t.Errorf("CriticalPath()[%d] is a copy of node %q, want a pointer to the node in the tree", i, n.Span.SpanID)
				}
			}
		})
	}
}

func TestCriticalPathIgnoresOrphans(t *testing.T) {
	// The orphan ends last of all, but its parent is unknown, so nobody knows
	// whether the root waited for it.
	tr := &tracetree.Trace{
		Root: node("r", "api", 0, 100, node("a", "db", 10, 90)),
		Orphans: []*tracetree.Node{
			node("o", "worker", 20, 150, node("o1", "worker", 30, 140)),
		},
	}
	want := []string{"r", "a"}
	if got := ids(criticalPath(t, tr)); !slices.Equal(got, want) {
		t.Errorf("CriticalPath() = %q, want %q", got, want)
	}
}

func TestCriticalPathDoesNotModifyTree(t *testing.T) {
	// Cutting b at the cursor must not change b's End.
	tr := &tracetree.Trace{Root: node("r", "api", 0, 100,
		node("a", "auth", 10, 60),
		node("b", "queue", 50, 130,
			node("b1", "worker", 60, 90), node("b2", "worker", 105, 125)))}
	before := dump(tr.Root)
	criticalPath(t, tr)
	if after := dump(tr.Root); after != before {
		t.Errorf("CriticalPath modified the tree.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// criticalPath calls CriticalPath in its own goroutine and gives it 5 s. A
// loop that picks the same child again never ends and appends forever, so
// instead of waiting until it has eaten all memory, the test binary stops
// with a message. The stack trace below the message shows where it loops.
func criticalPath(t *testing.T, tr *tracetree.Trace) []*tracetree.Node {
	t.Helper()
	done := make(chan []*tracetree.Node, 1)
	go func() { done <- tracetree.CriticalPath(tr) }()
	select {
	case path := <-done:
		return path
	case <-time.After(5 * time.Second):
		// t.Fatal would end only this test and leave the loop running.
		panic("CriticalPath did not return within 5s: does it pick the same child again? " +
			"A candidate must start strictly before the cursor, and after a pick the cursor " +
			"moves back to that child's Start.")
	}
}
