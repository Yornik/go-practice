package deployorder_test

import (
	"fmt"
	"slices"
	"testing"
)

func TestWhy(t *testing.T) {
	cases := []struct {
		name     string
		deps     map[string][]string
		from, to string
		want     []string
		wantOK   bool
	}{
		{"direct dependency", platform(), "api-gateway", "auth", []string{"api-gateway", "auth"}, true},
		// Three chains of 4 services each: through auth, orders or payments.
		// They differ first at position 2, where auth is the smallest.
		{"frontend to database", platform(), "web", "postgres", []string{"web", "api-gateway", "auth", "postgres"}, true},
		{"frontend to message queue", platform(), "web", "kafka", []string{"web", "api-gateway", "orders", "kafka"}, true},
		{"frontend to certificates", platform(), "web", "cert-manager", []string{"web", "api-gateway", "ingress-nginx", "cert-manager"}, true},
		{"a service to itself", platform(), "auth", "auth", []string{"auth"}, true},

		// No chain.
		{"wrong direction: postgres does not depend on web", platform(), "postgres", "web", nil, false},
		{"unrelated services", platform(), "kafka", "redis", nil, false},
		{"unknown from", platform(), "ghost", "postgres", nil, false},
		{"unknown to", platform(), "web", "ghost", nil, false},
		{"unknown service to itself", platform(), "ghost", "ghost", nil, false},

		// a reaches t through b and c (4 services) or through z (3 services).
		// A depth-first search in name order finds the long chain first.
		{
			"fewest services, not the first found in name order",
			map[string][]string{"a": {"b", "z"}, "b": {"c"}, "c": {"t"}, "z": {"t"}, "t": nil},
			"a", "t",
			[]string{"a", "z", "t"},
			true,
		},
		// [a b d] and [a c d] are equally short. The input lists c before b
		// on purpose: the input order does not matter.
		{
			"tie: the lexicographically first chain",
			map[string][]string{"a": {"c", "b"}, "b": {"d"}, "c": {"d"}, "d": nil},
			"a", "d",
			[]string{"a", "b", "d"},
			true,
		},
		// [a b y z] and [a c x z] are equally short. They differ first at
		// position 1, where b < c. That x < y later on does not matter.
		{
			"tie: the first difference decides",
			map[string][]string{"a": {"b", "c"}, "b": {"y"}, "c": {"x"}, "x": {"z"}, "y": {"z"}, "z": nil},
			"a", "z",
			[]string{"a", "b", "y", "z"},
			true,
		},

		// Cycles: a, b and c depend on each other in a loop; c also needs d.
		{
			"through a cycle",
			map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a", "d"}, "d": nil},
			"a", "d",
			[]string{"a", "b", "c", "d"},
			true,
		},
		{
			"around a cycle",
			map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a", "d"}, "d": nil},
			"b", "a",
			[]string{"b", "c", "a"},
			true,
		},
		{
			"out of a cycle there is no way back",
			map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a", "d"}, "d": nil},
			"d", "a",
			nil,
			false,
		},
		// a and b depend on each other, and neither needs c. A search that
		// does not remember what it has seen goes round and round.
		{
			"a cycle that does not lead to the target",
			map[string][]string{"a": {"b"}, "b": {"a"}, "c": nil},
			"a", "c",
			nil,
			false,
		},
		{"self-dependency", map[string][]string{"a": {"a"}}, "a", "a", []string{"a"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustNew(t, tc.deps)
			got, ok := why(t, g, tc.from, tc.to)
			if ok != tc.wantOK || !slices.Equal(got, tc.want) {
				t.Errorf("Why(%q, %q) = %q, %v; want %q, %v", tc.from, tc.to, got, ok, tc.want, tc.wantOK)
			}
			if !ok && got != nil {
				t.Errorf("Why(%q, %q) = %q, false; want nil together with false", tc.from, tc.to, got)
			}
		})
	}
}

// A long chain: the answer has 1000 services.
func TestWhyLongChain(t *testing.T) {
	const n = 1000
	g := mustNew(t, chain(n))
	var want []string
	for i := n - 1; i >= 0; i-- {
		want = append(want, svc(i))
	}
	got, ok := why(t, g, svc(n-1), svc(0))
	if !ok || !slices.Equal(got, want) {
		t.Errorf("Why(%q, %q) = %d services, %v; want all %d services from %q down to %q, true",
			svc(n-1), svc(0), len(got), ok, n, svc(n-1), svc(0))
	}
}

// 30 stacked diamonds: 2^30 equally short chains lead from d00 to d30. Why
// must not try them one by one. At every diamond, the -l side wins the tie.
func TestWhyManySharedDependencies(t *testing.T) {
	const n = 30
	g := mustNew(t, ladder(n))
	from, to := "d00", fmt.Sprintf("d%02d", n)
	var want []string // [d00 d00-l d01 d01-l ... d29 d29-l d30]
	for i := range n {
		want = append(want, fmt.Sprintf("d%02d", i), fmt.Sprintf("d%02d-l", i))
	}
	want = append(want, to)
	got, ok := why(t, g, from, to)
	if !ok || !slices.Equal(got, want) {
		t.Errorf("Why(%q, %q) = %q, %v; want %q, true", from, to, got, ok, want)
	}
}

// The caller owns the slice that Why returns.
func TestWhyReturnsNewSlice(t *testing.T) {
	g := mustNew(t, platform())
	want := []string{"web", "api-gateway", "orders", "kafka"}
	first, _ := why(t, g, "web", "kafka")
	if len(first) > 0 {
		first[0] = "changed by the caller"
	}
	second, ok := why(t, g, "web", "kafka")
	if !ok || !slices.Equal(second, want) {
		t.Errorf("Why(%q, %q) after the caller changed the first result = %q, %v; want %q, true", "web", "kafka", second, ok, want)
	}
}
