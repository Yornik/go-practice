package deployorder_test

import (
	"errors"
	"maps"
	"slices"
	"testing"

	deployorder "github.com/yornik/go-practice/exercises/10-deployorder"
)

func TestNewAcceptsValidGraphs(t *testing.T) {
	cases := []struct {
		name string
		deps map[string][]string
	}{
		{"nil map", nil},
		{"empty map", map[string][]string{}},
		{"nil and empty dependency lists", map[string][]string{"db": nil, "cache": {}}},
		{"realistic platform", platform()},
		{"duplicate dependencies", map[string][]string{"api": {"db", "db", "db"}, "db": nil}},
		// Cycles are accepted here. Waves and Order report them.
		{"self-dependency", map[string][]string{"a": {"a"}}},
		{"cycle", map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := deployorder.New(tc.deps)
			if err != nil {
				t.Fatalf("New(%v) error = %v, want nil", tc.deps, err)
			}
			if g == nil {
				t.Fatalf("New(%v) = nil, nil; want a non-nil *Graph", tc.deps)
			}
		})
	}
}

func TestNewUnknownDependency(t *testing.T) {
	cases := []struct {
		name string
		deps map[string][]string
		want deployorder.UnknownDependencyError
	}{
		{
			"typo",
			map[string][]string{"api": {"postgress"}, "postgres": nil},
			deployorder.UnknownDependencyError{Service: "api", Dependency: "postgress"},
		},
		{
			"dependency that is not a key",
			map[string][]string{"web": {"api"}},
			deployorder.UnknownDependencyError{Service: "web", Dependency: "api"},
		},
		// Services in sorted order: api comes before web.
		{
			"first service in sorted order",
			map[string][]string{"web": {"db"}, "api": {"queue"}, "ok": nil},
			deployorder.UnknownDependencyError{Service: "api", Dependency: "queue"},
		},
		{
			"first of many in sorted order",
			map[string][]string{
				"svc-f": {"ghost-f"}, "svc-c": {"ghost-c"}, "svc-h": {"ghost-h"}, "svc-a": {"ghost-a"},
				"svc-e": {"ghost-e"}, "svc-b": {"ghost-b"}, "svc-g": {"ghost-g"}, "svc-d": {"ghost-d"},
			},
			deployorder.UnknownDependencyError{Service: "svc-a", Dependency: "ghost-a"},
		},
		// The dependencies of one service in sorted order: cache comes before db.
		{
			"first dependency in sorted order",
			map[string][]string{"web": {"db", "cache", "ok"}, "ok": nil},
			deployorder.UnknownDependencyError{Service: "web", Dependency: "cache"},
		},
		// New checks for unknown dependencies. Cycles are found later, by Waves.
		{
			"unknown dependency inside a cycle",
			map[string][]string{"a": {"b"}, "b": {"a", "ghost"}},
			deployorder.UnknownDependencyError{Service: "b", Dependency: "ghost"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Several times: Go randomizes the iteration order of maps, so a
			// New that ranges over the map can report a different error each time.
			for range 5 {
				g, err := deployorder.New(tc.deps)
				if g != nil {
					t.Errorf("New(%v) returned a non-nil *Graph together with an error; want nil", tc.deps)
				}
				var ude *deployorder.UnknownDependencyError
				if !errors.As(err, &ude) {
					t.Fatalf("New(%v) error = %v (type %T), want an *UnknownDependencyError", tc.deps, err, err)
				}
				if *ude != tc.want {
					t.Fatalf("New(%v) error = %+v, want %+v", tc.deps, *ude, tc.want)
				}
			}
		})
	}
}

// New must not sort or deduplicate the caller's slices in place. The caller
// may still use them, for example to print the config as it was written.
func TestNewDoesNotModifyInput(t *testing.T) {
	deps := map[string][]string{
		"web":   {"cache", "api", "api", "auth"},
		"api":   {"db", "cache", "db"},
		"auth":  {"db"},
		"cache": nil,
		"db":    {},
	}
	before := cloneDeps(deps)
	mustNew(t, deps)
	if !maps.EqualFunc(deps, before, slices.Equal) {
		t.Errorf("New changed its input:\n got  %v\n want %v", deps, before)
	}
}

func TestErrorStrings(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			"unknown dependency",
			&deployorder.UnknownDependencyError{Service: "api", Dependency: "postgress"},
			`service "api" depends on unknown service "postgress"`,
		},
		{
			"cycle",
			&deployorder.CycleError{Path: []string{"a", "b", "c", "a"}},
			"dependency cycle: a -> b -> c -> a",
		},
		{
			"self-dependency",
			&deployorder.CycleError{Path: []string{"auth", "auth"}},
			"dependency cycle: auth -> auth",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}
