package deployorder_test

import (
	"errors"
	"fmt"
	"testing"

	deployorder "github.com/yornik/go-practice/exercises/10-deployorder"
)

// Every case follows the search rule in the README by hand. The comments
// show the path at the moment the search finds the cycle.
func TestCycleReported(t *testing.T) {
	cases := []struct {
		name string
		deps map[string][]string
		want []string
	}{
		{"self-dependency", map[string][]string{"a": {"a"}}, []string{"a", "a"}},
		// path [a b], and b depends on b
		{
			"self-dependency next to other services",
			map[string][]string{"a": {"b"}, "b": {"b", "c"}, "c": nil},
			[]string{"b", "b"},
		},
		{"two services", map[string][]string{"a": {"b"}, "b": {"a"}}, []string{"a", "b", "a"}},
		// path [api db cache], and cache depends on api
		{
			"three services",
			map[string][]string{"api": {"db"}, "cache": {"api"}, "db": {"cache"}},
			[]string{"api", "db", "cache", "api"},
		},
		// The README example. path [a b d], and d depends on b.
		{
			"README example",
			map[string][]string{"a": {"b"}, "b": {"c", "d"}, "c": nil, "d": {"b"}},
			[]string{"b", "d", "b"},
		},
		// path [api auth config], and config depends on auth. api only leads
		// to the cycle, so it is not part of Path.
		{
			"services that only lead to the cycle are not in Path",
			map[string][]string{"web": {"api"}, "api": {"auth"}, "auth": {"config"}, "config": {"auth"}},
			[]string{"auth", "config", "auth"},
		},
		// path [a z y], and y depends on z. Path starts where the search
		// entered the cycle, not at its smallest name (that would be [y z y]).
		{
			"Path starts where the search entered the cycle",
			map[string][]string{"a": {"z"}, "y": {"z"}, "z": {"y"}},
			[]string{"z", "y", "z"},
		},
		// a's dependencies in sorted order: b first. path [a b], b depends on a.
		{
			"dependencies are followed in sorted order",
			map[string][]string{"a": {"c", "b"}, "b": {"a"}, "c": {"a"}},
			[]string{"a", "b", "a"},
		},
		// Searches start from b, c, m, n in that order.
		{
			"searches start from the services in sorted order",
			map[string][]string{"m": {"n"}, "n": {"m"}, "b": {"c"}, "c": {"b"}},
			[]string{"b", "c", "b"},
		},
		// b and c both depend on d. When the search reaches d the second time,
		// d is finished, not on the path: a shared dependency is not a cycle.
		{
			"a shared dependency is not a cycle",
			map[string][]string{"a": {"b", "c"}, "b": {"d"}, "c": {"d"}, "d": nil, "y": {"z"}, "z": {"y"}},
			[]string{"y", "z", "y"},
		},
		// path [a b c], and c depends on a. The shorter cycle [a c a] exists,
		// but the search finds the longer one first.
		{
			"the first cycle found, not the shortest",
			map[string][]string{"a": {"b", "c"}, "b": {"c"}, "c": {"a"}},
			[]string{"a", "b", "c", "a"},
		},
		// path [a b c d], and d depends on b. The longer cycle a b c d e a is
		// never reached: the search stops at the first cycle.
		{
			"the first cycle found, not the longest",
			map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"d"}, "d": {"b", "e"}, "e": {"a"}},
			[]string{"b", "c", "d", "b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustNew(t, tc.deps)
			got, err := waves(t, g)
			checkCycle(t, "Waves()", tc.deps, err, tc.want)
			if got != nil {
				t.Errorf("Waves() = %q together with a cycle error, want nil: no partial plan", got)
			}
		})
	}
}

// Go randomizes the iteration order of maps. With 20 separate cycles, a
// search that ranges over a map can pick a different cycle from run to run.
func TestCycleSameEveryTime(t *testing.T) {
	deps := make(map[string][]string)
	for i := range 20 {
		p, q := fmt.Sprintf("p%02d", i), fmt.Sprintf("q%02d", i)
		deps[p] = []string{q}
		deps[q] = []string{p}
	}
	want := []string{"p00", "q00", "p00"}
	g := mustNew(t, deps)
	for i := range 10 {
		_, err := waves(t, g)
		checkCycle(t, fmt.Sprintf("Waves() call %d", i+1), deps, err, want)
	}
}

// Order reports exactly the cycle that Waves reports.
func TestCycleFromOrder(t *testing.T) {
	deps := platform()
	deps["auth"] = append(deps["auth"], "config")
	deps["config"] = []string{"auth"}
	// The first search starts at api-gateway: path [api-gateway auth config],
	// and config depends on auth.
	want := []string{"auth", "config", "auth"}

	g := mustNew(t, deps)
	got, err := order(t, g)
	checkCycle(t, "Order()", deps, err, want)
	if got != nil {
		t.Errorf("Order() = %q together with a cycle error, want nil: no partial plan", got)
	}
	_, err = waves(t, g)
	checkCycle(t, "Waves()", deps, err, want)
}

// 1000 services in one cycle: svc-0000 depends on svc-0999, which depends on
// svc-0998, and so on down to svc-0001, which depends on svc-0000.
func TestCycleLong(t *testing.T) {
	const n = 1000
	deps := chain(n)
	deps[svc(0)] = []string{svc(n - 1)}
	want := []string{svc(0)}
	for i := n - 1; i >= 0; i-- {
		want = append(want, svc(i))
	}
	g := mustNew(t, deps)
	_, err := waves(t, g)
	checkCycle(t, "Waves()", deps, err, want)
}

// 30 stacked diamonds (2^30 paths, no cycle), and a cycle that the search only
// reaches after it has finished the diamonds. A search that does not skip
// finished services walks every one of the 2^30 paths first.
func TestCycleAfterManySharedDependencies(t *testing.T) {
	deps := ladder(30)
	deps["zz-a"] = []string{"zz-b"}
	deps["zz-b"] = []string{"zz-a"}
	g := mustNew(t, deps)
	_, err := waves(t, g)
	checkCycle(t, "Waves()", deps, err, []string{"zz-a", "zz-b", "zz-a"})
}

// The caller owns CycleError.Path.
func TestCyclePathIsNew(t *testing.T) {
	deps := map[string][]string{"a": {"b"}, "b": {"a"}}
	want := []string{"a", "b", "a"}
	g := mustNew(t, deps)

	_, err := waves(t, g)
	checkCycle(t, "Waves()", deps, err, want)
	var first *deployorder.CycleError
	if errors.As(err, &first) {
		first.Path[0] = "changed by the caller"
	}

	_, err = waves(t, g)
	checkCycle(t, "Waves() after the caller changed the Path of an earlier error", deps, err, want)
}
