package deployorder_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	deployorder "github.com/yornik/go-practice/exercises/35-deployorder"
)

// platform returns the services of a small but realistic platform. It is the
// graph of the README examples. Every call returns a new map, so a test may
// change it.
//
//	wave 0: cert-manager kafka postgres redis
//	wave 1: auth ingress-nginx
//	wave 2: orders payments
//	wave 3: api-gateway
//	wave 4: web
func platform() map[string][]string {
	return map[string][]string{
		"cert-manager":  nil,
		"ingress-nginx": {"cert-manager"},
		"postgres":      nil,
		"redis":         nil,
		"kafka":         nil,
		"auth":          {"postgres", "redis"},
		"orders":        {"postgres", "kafka", "auth"},
		"payments":      {"postgres", "auth"},
		"api-gateway":   {"auth", "orders", "payments", "ingress-nginx"},
		"web":           {"api-gateway"},
	}
}

// svc returns a service name that sorts in numeric order: svc(7) is "svc-0007".
func svc(i int) string { return fmt.Sprintf("svc-%04d", i) }

// chain returns n services in one long line: svc-0001 depends on svc-0000,
// svc-0002 on svc-0001, and so on.
func chain(n int) map[string][]string {
	deps := make(map[string][]string, n)
	for i := range n {
		if i == 0 {
			deps[svc(i)] = nil
			continue
		}
		deps[svc(i)] = []string{svc(i - 1)}
	}
	return deps
}

// ladder returns n diamonds stacked on top of each other:
//
//	d00 depends on d00-l and d00-r, both of which depend on d01,
//	d01 depends on d01-l and d01-r, both of which depend on d02, ...
//
// and d<n> has no dependencies. There are 2^n different paths from d00 down
// to d<n>, so a search that walks the same services again and again never
// finishes, while one that remembers what it has seen visits 3n+1 services.
func ladder(n int) map[string][]string {
	deps := make(map[string][]string, 3*n+1)
	for i := range n {
		top, left, right, next := fmt.Sprintf("d%02d", i), fmt.Sprintf("d%02d-l", i), fmt.Sprintf("d%02d-r", i), fmt.Sprintf("d%02d", i+1)
		deps[top] = []string{left, right}
		deps[left] = []string{next}
		deps[right] = []string{next}
	}
	deps[fmt.Sprintf("d%02d", n)] = nil
	return deps
}

// randomDAG returns n services with 0 to 4 random dependencies each
// (duplicates possible) and no cycle. The names are shuffled, so the sorted
// order of the names says nothing about who depends on whom. The same seed
// always gives the same graph.
func randomDAG(n int, seed uint64) map[string][]string {
	r := rand.New(rand.NewPCG(seed, 2026))
	names := make([]string, n)
	for i, p := range r.Perm(n) {
		names[i] = svc(p)
	}
	deps := make(map[string][]string, n)
	for i := range n {
		var ds []string
		if i > 0 {
			// Only services with a lower index: that is what rules out cycles.
			for range r.IntN(5) {
				ds = append(ds, names[r.IntN(i)])
			}
		}
		deps[names[i]] = ds
	}
	return deps
}

// cloneDeps returns a deep copy of deps.
func cloneDeps(deps map[string][]string) map[string][]string {
	out := make(map[string][]string, len(deps))
	for k, v := range deps {
		out[k] = slices.Clone(v)
	}
	return out
}

// mustNew builds a Graph and stops the test if New fails.
func mustNew(t *testing.T, deps map[string][]string) *deployorder.Graph {
	t.Helper()
	g, err := deployorder.New(deps)
	if err != nil {
		t.Fatalf("New(%v) error = %v, want nil", deps, err)
	}
	if g == nil {
		t.Fatalf("New(%v) = nil, nil; want a non-nil *Graph", deps)
	}
	return g
}

// within runs f in a goroutine and fails the test if f has not returned after
// 5 seconds. A graph search that does not remember which services it has seen
// loops forever on a cycle, or takes exponential time when many services share
// dependencies. within turns that into a failure with a message, instead of a
// test run that hangs until the 10-minute timeout.
func within(t *testing.T, call string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not return within 5s. Does the search walk the same services again and again? "+
			"It has to remember which services it has already seen.", call)
	}
}

// The helpers below call one method of g, guarded by within.

func waves(t *testing.T, g *deployorder.Graph) (got [][]string, err error) {
	t.Helper()
	within(t, "Waves()", func() { got, err = g.Waves() })
	return got, err
}

func order(t *testing.T, g *deployorder.Graph) (got []string, err error) {
	t.Helper()
	within(t, "Order()", func() { got, err = g.Order() })
	return got, err
}

func dependents(t *testing.T, g *deployorder.Graph, service string) (got []string, err error) {
	t.Helper()
	within(t, fmt.Sprintf("Dependents(%q)", service), func() { got, err = g.Dependents(service) })
	return got, err
}

func why(t *testing.T, g *deployorder.Graph, from, to string) (got []string, ok bool) {
	t.Helper()
	within(t, fmt.Sprintf("Why(%q, %q)", from, to), func() { got, ok = g.Why(from, to) })
	return got, ok
}

// equalWaves compares two wave lists. A nil and an empty list are equal.
func equalWaves(a, b [][]string) bool {
	return slices.EqualFunc(a, b, func(x, y []string) bool { return slices.Equal(x, y) })
}

// checkCycle checks that err is a *CycleError whose Path is want. When the
// Path is wrong, the message also says whether it is a real cycle of deps at
// all, so you can tell "not a cycle" from "a cycle, but not the one the rule
// picks".
func checkCycle(t *testing.T, call string, deps map[string][]string, err error, want []string) {
	t.Helper()
	var ce *deployorder.CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("%s error = %v (type %T), want a *CycleError with Path %q", call, err, err, want)
	}
	if !slices.Equal(ce.Path, want) {
		t.Fatalf("%s: CycleError.Path = %q, want %q\n%s", call, ce.Path, want, cycleProblem(deps, ce.Path))
	}
}

// cycleProblem explains what is wrong with path as a cycle of deps.
func cycleProblem(deps map[string][]string, path []string) string {
	if len(path) < 2 {
		return "A Path has at least 2 entries: [a a] for a service that depends on itself."
	}
	if path[0] != path[len(path)-1] {
		return "A Path starts and ends with the same service."
	}
	seen := make(map[string]bool)
	for i := range len(path) - 1 {
		from, to := path[i], path[i+1]
		if seen[from] {
			return fmt.Sprintf("%q appears twice in Path; only the first and the last entry are the same service.", from)
		}
		seen[from] = true
		if !slices.Contains(deps[from], to) {
			return fmt.Sprintf("Not a cycle: %q does not depend on %q.", from, to)
		}
	}
	return "Your Path is a real cycle, but not the one that the search rule in the README finds first."
}
