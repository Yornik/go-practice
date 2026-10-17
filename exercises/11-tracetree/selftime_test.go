package tracetree_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tracetree "github.com/yornik/go-practice/exercises/11-tracetree"
)

func TestSelfTime(t *testing.T) {
	// Unless the name says otherwise, the parent p runs from 0 to 100 ms.
	cases := []struct {
		name string
		n    *tracetree.Node
		want time.Duration
	}{
		{"leaf: all of its time is self time",
			node("p", "api", 0, 100), ms(100)},
		{"one child",
			node("p", "api", 0, 100, node("a", "db", 20, 50)), ms(70)},
		{"sequential children",
			node("p", "api", 0, 100, node("a", "db", 10, 30), node("b", "db", 30, 60)), ms(50)},
		{"gap between children",
			node("p", "api", 0, 100, node("a", "db", 10, 20), node("b", "db", 40, 50)), ms(80)},
		// 10-50 and 20-60 overlap: together they cover 10-60, which is 50 ms,
		// not 40+40 = 80 ms.
		{"parallel children: overlap counts once",
			node("p", "api", 0, 100, node("a", "db", 10, 50), node("b", "cache", 20, 60)), ms(50)},
		{"child within another child's time",
			node("p", "api", 0, 100, node("a", "db", 10, 80), node("b", "cache", 20, 40)), ms(30)},
		// Covered: 10-70 and 80-90. b lies inside a, and c overlaps a.
		{"short child inside a long one, then a child that overlaps the long one",
			node("p", "api", 0, 100,
				node("a", "db", 10, 40), node("b", "db", 20, 30), node("c", "db", 35, 70), node("d", "db", 80, 90)),
			ms(30)},
		// 4 calls start at once; together they cover 10-70.
		{"fan-out with the same start",
			node("p", "api", 0, 100,
				node("a", "db", 10, 60), node("b", "db", 10, 70), node("c", "db", 10, 50), node("d", "db", 10, 65)),
			ms(40)},
		{"child covers the whole parent",
			node("p", "api", 0, 100, node("a", "db", 0, 100)), ms(0)},
		// Only 80-100 counts: the rest of a is outside p.
		{"child ends after the parent (async work or clock skew)",
			node("p", "api", 0, 100, node("a", "queue", 80, 150)), ms(80)},
		// Only 0-30 counts.
		{"child starts before the parent (clock skew)",
			node("p", "api", 0, 100, node("a", "db", -20, 30)), ms(70)},
		{"children entirely outside the parent count for nothing",
			node("p", "api", 0, 100, node("b", "db", -50, -10), node("a", "queue", 120, 150)), ms(100)},
		{"children overflow on both sides: 0, never negative",
			node("p", "api", 0, 100, node("a", "db", -10, 60), node("b", "db", 50, 120)), ms(0)},
		{"zero-duration children take no time",
			node("p", "api", 0, 100, node("a", "db", 30, 30), node("b", "db", 60, 60)), ms(100)},
		{"zero-duration parent (50-50)",
			node("p", "api", 50, 50, node("a", "db", 40, 60)), ms(0)},
		// The grandchild a1 sticks out of a (clock skew). Only p's direct
		// children count for p's self time.
		{"only direct children count",
			node("p", "api", 0, 100, node("a", "db", 10, 20, node("a1", "disk", 10, 90))), ms(90)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tracetree.SelfTime(tc.n); got != tc.want {
				t.Errorf("SelfTime(%s) = %v, want %v\ntree, times in ms:\n%s", tc.n.Span.SpanID, got, tc.want, timeline(tc.n))
			}
		})
	}
}

func TestSelfTimeDoesNotModifyTree(t *testing.T) {
	// Children that stick out of their parent, and whose order by End
	// differs from their order by Start: clipping them in place or sorting
	// them in another order would show here.
	p := node("p", "api", 0, 100,
		node("a", "db", -10, 70),
		node("b", "db", -10, 60, node("b1", "disk", 20, 80)),
		node("c", "queue", 90, 130))
	before := dump(p)
	tracetree.SelfTime(p)
	tracetree.SelfTime(p.Children[1])
	if after := dump(p); after != before {
		t.Errorf("SelfTime modified the tree.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestServiceTimes(t *testing.T) {
	cases := []struct {
		name  string
		trace *tracetree.Trace
		want  map[string]time.Duration
	}{
		{
			name:  "single span",
			trace: &tracetree.Trace{Root: node("r", "api", 0, 100)},
			want:  map[string]time.Duration{"api": ms(100)},
		},
		{
			// Self times (ms): fe 14, gw 0, cart 50, pay 50, pdb 20, fe2 6,
			// mail 20, mpg 10. The root tree sums to 140 ms although the
			// request took 100 ms: gw and pay ran in parallel.
			name: "sums per service, keeps services with 0, includes orphans",
			trace: &tracetree.Trace{
				Root: node("fe", "frontend", 0, 100,
					node("gw", "gateway", 10, 60,
						node("cart", "cart", 10, 60)),
					node("pay", "payment", 20, 90,
						node("pdb", "postgres", 30, 50)),
					node("fe2", "frontend", 92, 98)),
				Orphans: []*tracetree.Node{
					node("mail", "mailer", 120, 150,
						node("mpg", "postgres", 125, 135)),
				},
			},
			want: map[string]time.Duration{
				"frontend": ms(20),
				"gateway":  0,
				"cart":     ms(50),
				"payment":  ms(50),
				"postgres": ms(30),
				"mailer":   ms(20),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tracetree.ServiceTimes(tc.trace)
			if !maps.Equal(got, tc.want) {
				t.Errorf("ServiceTimes() =\n%s\nwant\n%s", formatTimes(got), formatTimes(tc.want))
			}
		})
	}
}

// formatTimes prints a map of service times sorted by service, one per line.
func formatTimes(m map[string]time.Duration) string {
	if m == nil {
		return "  <nil map>"
	}
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(m)) {
		fmt.Fprintf(&b, "  %s: %v\n", k, m[k])
	}
	return strings.TrimSuffix(b.String(), "\n")
}
