package deployorder_test

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	deployorder "github.com/yornik/go-practice/exercises/10-deployorder"
)

func TestDependents(t *testing.T) {
	cases := []struct {
		name    string
		deps    map[string][]string
		service string
		want    []string // nil means: no dependents (a nil or an empty slice)
	}{
		{"database: almost everything breaks", platform(), "postgres", []string{"api-gateway", "auth", "orders", "payments", "web"}},
		{"message queue", platform(), "kafka", []string{"api-gateway", "orders", "web"}},
		{"certificates: the ingress and everything behind it", platform(), "cert-manager", []string{"api-gateway", "ingress-nginx", "web"}},
		{"service in the middle", platform(), "auth", []string{"api-gateway", "orders", "payments", "web"}},
		{"one direct dependent", platform(), "api-gateway", []string{"web"}},
		{"nothing depends on the frontend", platform(), "web", nil},
		{"direct and indirect dependents", chain(4), svc(0), []string{svc(1), svc(2), svc(3)}},
		{
			"each dependent once, also when reached twice",
			map[string][]string{"web": {"api", "auth"}, "api": {"db"}, "auth": {"db"}, "db": nil},
			"db",
			[]string{"api", "auth", "web"},
		},
		{"independent services", map[string][]string{"a": nil, "b": nil}, "a", nil},
		// a, b and c form a cycle; d depends on a. a depends on itself through
		// b and c, but the service itself is never in the result.
		{
			"cycle: the service itself is not in the result",
			map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}, "d": {"a"}},
			"a",
			[]string{"b", "c", "d"},
		},
		{"self-dependency", map[string][]string{"a": {"a"}, "b": {"a"}}, "a", []string{"b"}},
		// a and b depend on each other, and a needs db. The walk up from db
		// enters the cycle, which does not lead back to db.
		{
			"cycle among the dependents",
			map[string][]string{"a": {"b", "db"}, "b": {"a"}, "db": nil},
			"db",
			[]string{"a", "b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustNew(t, tc.deps)
			got, err := dependents(t, g, tc.service)
			if err != nil {
				t.Fatalf("Dependents(%q) error = %v, want nil", tc.service, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Dependents(%q)\n got  %q\n want %q", tc.service, got, tc.want)
			}
		})
	}
}

// 30 stacked diamonds: every other service depends on d30, through 2^30
// different paths from the top. Dependents must visit each service once, not
// each path.
func TestDependentsManySharedDependencies(t *testing.T) {
	const n = 30
	deps := ladder(n)
	bottom := fmt.Sprintf("d%02d", n)
	want := slices.DeleteFunc(slices.Sorted(maps.Keys(deps)), func(s string) bool { return s == bottom })
	g := mustNew(t, deps)
	got, err := dependents(t, g, bottom)
	if err != nil {
		t.Fatalf("Dependents(%q) error = %v, want nil", bottom, err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Dependents(%q)\n got  %q\n want all %d other services, sorted: %q", bottom, got, len(want), want)
	}
}

// A typo must not look like "nothing breaks".
func TestDependentsUnknownService(t *testing.T) {
	cases := []struct {
		name    string
		deps    map[string][]string
		service string
	}{
		{"typo", platform(), "postgress"},
		{"empty graph", nil, "postgres"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustNew(t, tc.deps)
			got, err := dependents(t, g, tc.service)
			if !errors.Is(err, deployorder.ErrUnknownService) {
				t.Fatalf("Dependents(%q) error = %v, want an error that wraps ErrUnknownService", tc.service, err)
			}
			if !strings.Contains(err.Error(), tc.service) {
				t.Errorf("Dependents(%q) error = %q, want it to contain the service name", tc.service, err)
			}
			if got != nil {
				t.Errorf("Dependents(%q) = %q together with an error, want nil", tc.service, got)
			}
		})
	}
}

// The caller owns the slice that Dependents returns.
func TestDependentsReturnsNewSlice(t *testing.T) {
	g := mustNew(t, platform())
	want := []string{"api-gateway", "orders", "web"}
	first, err := dependents(t, g, "kafka")
	if err != nil {
		t.Fatalf("Dependents(%q) error = %v, want nil", "kafka", err)
	}
	if len(first) > 0 {
		first[0] = "changed by the caller"
	}
	second, err := dependents(t, g, "kafka")
	if err != nil {
		t.Fatalf("second Dependents(%q) error = %v, want nil", "kafka", err)
	}
	if !slices.Equal(second, want) {
		t.Errorf("Dependents(%q) after the caller changed the first result\n got  %q\n want %q", "kafka", second, want)
	}
}
