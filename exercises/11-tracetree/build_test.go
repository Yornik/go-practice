package tracetree_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	tracetree "github.com/yornik/go-practice/exercises/11-tracetree"
)

func TestBuildTree(t *testing.T) {
	cases := []struct {
		name        string
		spans       []tracetree.Span
		wantRoot    string   // shape of Root, see shape()
		wantOrphans []string // shape of each orphan, in order
	}{
		{
			name:     "single span",
			spans:    []tracetree.Span{span("r", "", "api", 0, 100)},
			wantRoot: "r",
		},
		{
			name:        "checkout trace, children before their parents",
			spans:       checkoutSpans(),
			wantRoot:    "fe1(auth1 cart1(redis1) inv1(pg1) price1 pay1(fraud1 psp1) fe2)",
			wantOrphans: []string{"mail1(smtp1)"},
		},
		{
			// Parents first is the order that breaks a map of Node values:
			// a copy put into a parent does not see children added later.
			name: "deep chain, parents first",
			spans: []tracetree.Span{
				span("r", "", "api", 0, 100),
				span("a", "r", "api", 10, 90),
				span("b", "a", "api", 20, 80),
				span("c", "b", "api", 30, 70),
				span("d", "c", "api", 40, 60),
			},
			wantRoot: "r(a(b(c(d))))",
		},
		{
			// Not by input order, not by End: by Start, and a tie on Start
			// is broken by SpanID.
			name: "children sorted by Start, then SpanID",
			spans: []tracetree.Span{
				span("x", "r", "api", 20, 30),
				span("b", "r", "api", 10, 50),
				span("a", "r", "api", 10, 15),
				span("c", "r", "api", 5, 60),
				span("r", "", "api", 0, 100),
			},
			wantRoot: "r(c a b x)",
		},
		{
			name: "zero-duration span is valid",
			spans: []tracetree.Span{
				span("r", "", "api", 0, 100),
				span("event", "r", "api", 50, 50),
			},
			wantRoot: "r(event)",
		},
		{
			// Build links by ID. Times do not decide who is whose child.
			name: "child that starts before its parent (clock skew) is still its child",
			spans: []tracetree.Span{
				span("r", "", "api", 10, 100),
				span("a", "r", "db", 0, 50),
			},
			wantRoot: "r(a)",
		},
		{
			// Each span with a missing parent starts its own orphan tree,
			// also when two of them name the same missing parent. The
			// children below an orphan are sorted like all other children.
			name: "orphans with their subtrees, sorted by Start, then SpanID",
			spans: []tracetree.Span{
				span("r", "", "api", 0, 100),
				span("a", "r", "api", 10, 20),
				span("o3", "lost1", "worker", 30, 35),
				span("o1", "lost1", "worker", 30, 40),
				span("o1y", "o1", "worker", 32, 38),
				span("o1x", "o1", "worker", 31, 39),
				span("o2", "lost2", "worker", 5, 50),
			},
			wantRoot:    "r(a)",
			wantOrphans: []string{"o2", "o1(o1x o1y)", "o3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := mustBuild(t, tc.spans)
			if got := shape(tr.Root); got != tc.wantRoot {
				t.Errorf("Build() root tree = %s, want %s", got, tc.wantRoot)
			}
			want := tc.wantOrphans
			if want == nil {
				want = []string{}
			}
			if got := shapes(tr.Orphans); !slices.Equal(got, want) {
				t.Errorf("Build() orphans = %q, want %q", got, want)
			}
		})
	}
}

// Exporters send spans in batches, from many services, so the input order is
// arbitrary. The tree must not depend on it.
func TestBuildInputOrderDoesNotMatter(t *testing.T) {
	const wantRoot = "fe1(auth1 cart1(redis1) inv1(pg1) price1 pay1(fraud1 psp1) fe2)"
	wantOrphans := []string{"mail1(smtp1)"}
	// A fixed seed: the same 50 orders on every run.
	rng := rand.New(rand.NewPCG(11, 2026))
	for range 50 {
		spans := checkoutSpans()
		rng.Shuffle(len(spans), func(i, j int) { spans[i], spans[j] = spans[j], spans[i] })
		order := strings.Join(spanIDs(spans), " ")
		tr, err := tracetree.Build(spans)
		if err != nil {
			t.Fatalf("Build() with input order [%s]: error = %v, want nil", order, err)
		}
		if tr == nil || tr.Root == nil {
			t.Fatalf("Build() with input order [%s] = %+v, want a trace with a Root", order, tr)
		}
		if got := shape(tr.Root); got != wantRoot {
			t.Fatalf("Build() with input order [%s]:\nroot tree = %s\nwant        %s", order, got, wantRoot)
		}
		if got := shapes(tr.Orphans); !slices.Equal(got, wantOrphans) {
			t.Fatalf("Build() with input order [%s]: orphans = %q, want %q", order, got, wantOrphans)
		}
	}
}

func TestBuildKeepsSpans(t *testing.T) {
	in := checkoutSpans()
	before := slices.Clone(in)
	tr := mustBuild(t, in)

	// The caller's slice is not reordered or changed, for example by sorting
	// it in place.
	for i := range in {
		if !sameSpan(in[i], before[i]) {
			t.Errorf("Build() modified its input: spans[%d] = %+v, was %+v", i, in[i], before[i])
		}
	}

	// Every span is in the result exactly once, unchanged.
	nodes := map[string]*tracetree.Node{}
	total := 0
	for _, n := range append([]*tracetree.Node{tr.Root}, tr.Orphans...) {
		total += countNodes(n)
		index(n, nodes)
	}
	if total != len(in) || len(nodes) != len(in) {
		t.Errorf("Build() returned %d nodes with %d distinct span IDs, want %d of each (every span exactly once)",
			total, len(nodes), len(in))
	}
	for _, s := range before {
		n, ok := nodes[s.SpanID]
		if !ok {
			t.Errorf("Build(): span %q is not in the tree", s.SpanID)
			continue
		}
		if !sameSpan(n.Span, s) {
			t.Errorf("Build(): node %q has Span %+v, want the input span %+v", s.SpanID, n.Span, s)
		}
	}
}

func TestBuildErrors(t *testing.T) {
	root := span("root", "", "api", 0, 100)
	stray := span("stray-3", "root", "api", 10, 20)
	stray.TraceID = "0af7651916cd43dd8448eb211c80319c"

	cases := []struct {
		name    string
		spans   []tracetree.Span
		want    error
		mention []string // span IDs the error message must contain
	}{
		{"nil input", nil, tracetree.ErrNoSpans, nil},
		{"empty input", []tracetree.Span{}, tracetree.ErrNoSpans, nil},
		{
			"span of another trace",
			[]tracetree.Span{root, span("a", "root", "api", 5, 50), stray},
			tracetree.ErrMixedTraces, []string{"stray-3"},
		},
		{
			"duplicate span ID",
			[]tracetree.Span{root, span("dup-7", "root", "db", 10, 20), span("dup-7", "root", "db", 30, 40)},
			tracetree.ErrDuplicateSpanID, []string{"dup-7"},
		},
		{
			"span ends before it starts",
			[]tracetree.Span{root, span("neg-5", "root", "db", 50, 40)},
			tracetree.ErrNegativeDuration, []string{"neg-5"},
		},
		{
			// The root span was dropped: every span has a parent ID.
			"no root",
			[]tracetree.Span{span("a", "lost", "api", 10, 90), span("b", "a", "db", 20, 30)},
			tracetree.ErrNoRoot, nil,
		},
		{
			"two roots",
			[]tracetree.Span{span("root-1", "", "api", 0, 100), span("c", "root-1", "db", 10, 20), span("root-2", "", "api", 5, 50)},
			tracetree.ErrMultipleRoots, []string{"root-1", "root-2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := tracetree.Build(tc.spans)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Build() error = %v, want an error wrapping %q", err, tc.want)
			}
			if tr != nil {
				t.Errorf("Build() trace = %+v, want nil on error", tr)
			}
			for _, id := range tc.mention {
				if !strings.Contains(err.Error(), id) {
					t.Errorf("Build() error = %q, want it to name span %q", err, id)
				}
			}
		})
	}
}
