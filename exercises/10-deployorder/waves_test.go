package deployorder_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"
)

func TestWaves(t *testing.T) {
	cases := []struct {
		name string
		deps map[string][]string
		want [][]string
	}{
		{"empty graph", nil, nil},
		{"one service", map[string][]string{"db": nil}, [][]string{{"db"}}},
		{
			"independent services share wave 0, sorted",
			map[string][]string{"redis": nil, "postgres": nil, "kafka": nil},
			[][]string{{"kafka", "postgres", "redis"}},
		},
		{
			"chain",
			map[string][]string{"web": {"api"}, "api": {"db"}, "db": nil},
			[][]string{{"db"}, {"api"}, {"web"}},
		},
		{
			"diamond",
			map[string][]string{"web": {"api", "auth"}, "api": {"db"}, "auth": {"db"}, "db": nil},
			[][]string{{"db"}, {"api", "auth"}, {"web"}},
		},
		// app needs config (wave 0) and db (wave 2), so it waits for db.
		{
			"a service waits for its slowest dependency",
			map[string][]string{
				"app":    {"config", "db"},
				"config": nil,
				"db":     {"disk"},
				"disk":   {"volume"},
				"volume": nil,
			},
			[][]string{{"config", "volume"}, {"disk"}, {"db"}, {"app"}},
		},
		{
			"duplicate dependencies count once",
			map[string][]string{"api": {"db", "db", "db"}, "web": {"api", "db", "api"}, "db": nil},
			[][]string{{"db"}, {"api"}, {"web"}},
		},
		{
			"input order does not matter",
			map[string][]string{"web": {"cache", "auth", "api"}, "auth": nil, "cache": nil, "api": {"cache"}},
			[][]string{{"auth", "cache"}, {"api"}, {"web"}},
		},
		{
			"realistic platform",
			platform(),
			[][]string{
				{"cert-manager", "kafka", "postgres", "redis"},
				{"auth", "ingress-nginx"},
				{"orders", "payments"},
				{"api-gateway"},
				{"web"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustNew(t, tc.deps)
			got, err := waves(t, g)
			if err != nil {
				t.Fatalf("Waves() error = %v, want nil", err)
			}
			if !equalWaves(got, tc.want) {
				t.Errorf("Waves()\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}

func TestWavesLongChain(t *testing.T) {
	const n = 1000
	g := mustNew(t, chain(n))
	got, err := waves(t, g)
	if err != nil {
		t.Fatalf("Waves() error = %v, want nil", err)
	}
	if len(got) != n {
		t.Fatalf("Waves() returned %d waves, want %d (one service per wave)", len(got), n)
	}
	for i, w := range got {
		if want := []string{svc(i)}; !slices.Equal(w, want) {
			t.Fatalf("Waves()[%d] = %q, want %q", i, w, want)
		}
	}
}

// 30 diamonds stacked on top of each other: 2^30 paths lead from the top to
// the bottom. Waves must not walk every path.
func TestWavesManySharedDependencies(t *testing.T) {
	const n = 30
	g := mustNew(t, ladder(n))
	got, err := waves(t, g)
	if err != nil {
		t.Fatalf("Waves() error = %v, want nil", err)
	}
	// From the bottom up: [d30] [d29-l d29-r] [d29] [d28-l d28-r] [d28] ... [d00]
	want := [][]string{{fmt.Sprintf("d%02d", n)}}
	for i := n - 1; i >= 0; i-- {
		want = append(want,
			[]string{fmt.Sprintf("d%02d-l", i), fmt.Sprintf("d%02d-r", i)},
			[]string{fmt.Sprintf("d%02d", i)})
	}
	if !equalWaves(got, want) {
		t.Errorf("Waves()\n got  %q\n want %q", got, want)
	}
}

// A property test on random graphs. The rules fix the waves completely:
// every service is in exactly one wave, every dependency is in an earlier
// wave, and a service in wave k > 0 has a dependency in wave k-1 (otherwise
// it could have gone earlier).
func TestWavesAreAsEarlyAsPossible(t *testing.T) {
	for seed := range uint64(5) {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			deps := randomDAG(300, seed)
			g := mustNew(t, deps)
			got, err := waves(t, g)
			if err != nil {
				t.Fatalf("Waves() error = %v, want nil (the graph has no cycle)", err)
			}
			for _, problem := range wavesProblems(deps, got) {
				t.Error(problem)
			}
		})
	}
}

// wavesProblems lists (at most 10) ways in which got breaks the wave rules.
func wavesProblems(deps map[string][]string, got [][]string) []string {
	var problems []string
	report := func(format string, args ...any) {
		if len(problems) < 10 {
			problems = append(problems, fmt.Sprintf(format, args...))
		}
	}
	waveOf := make(map[string]int)
	for i, w := range got {
		if len(w) == 0 {
			report("wave %d is empty", i)
		}
		if !slices.IsSorted(w) {
			report("wave %d is not sorted: %q", i, w)
		}
		for _, s := range w {
			if _, known := deps[s]; !known {
				report("wave %d contains %q, which is not a service of the graph", i, s)
			}
			if prev, dup := waveOf[s]; dup {
				report("%q is in wave %d and in wave %d", s, prev, i)
			}
			waveOf[s] = i
		}
	}
	for _, s := range slices.Sorted(maps.Keys(deps)) {
		w, ok := waveOf[s]
		if !ok {
			report("%q is in no wave", s)
			continue
		}
		highest := -1 // the highest wave among the dependencies of s
		for _, d := range deps[s] {
			dw, ok := waveOf[d]
			if !ok {
				continue // reported above
			}
			if dw >= w {
				report("%q is in wave %d, but its dependency %q is in wave %d: dependencies go in an earlier wave", s, w, d, dw)
			}
			highest = max(highest, dw)
		}
		if w > highest+1 {
			report("%q is in wave %d, but its last dependency is in wave %d: it belongs in wave %d", s, w, highest, highest+1)
		}
	}
	return problems
}

// Changing the map or its slices after New must not change the Graph.
func TestWavesIgnoreLaterChangesToInput(t *testing.T) {
	deps := map[string][]string{"web": {"api"}, "api": {"db"}, "db": nil}
	g := mustNew(t, deps)

	deps["web"][0] = "db"        // web now says it depends on db only
	deps["db"] = []string{"api"} // this would be a cycle
	delete(deps, "api")
	deps["cache"] = nil

	got, err := waves(t, g)
	if err != nil {
		t.Fatalf("Waves() error = %v, want nil: the Graph must not see changes made to the map after New", err)
	}
	want := [][]string{{"db"}, {"api"}, {"web"}}
	if !equalWaves(got, want) {
		t.Errorf("Waves() after changing the input map\n got  %q\n want %q (the graph as it was passed to New)", got, want)
	}
}

// The caller owns the slices that Waves returns.
func TestWavesReturnsNewSlices(t *testing.T) {
	g := mustNew(t, platform())
	first, err := waves(t, g)
	if err != nil {
		t.Fatalf("Waves() error = %v, want nil", err)
	}
	want := [][]string{
		{"cert-manager", "kafka", "postgres", "redis"},
		{"auth", "ingress-nginx"},
		{"orders", "payments"},
		{"api-gateway"},
		{"web"},
	}
	first[0][0] = "changed by the caller"
	first[1] = nil
	second, err := waves(t, g)
	if err != nil {
		t.Fatalf("second Waves() error = %v, want nil", err)
	}
	if !equalWaves(second, want) {
		t.Errorf("Waves() after the caller changed the first result\n got  %q\n want %q", second, want)
	}
}

func TestOrder(t *testing.T) {
	cases := []struct {
		name string
		deps map[string][]string
		want []string
	}{
		{"empty graph", map[string][]string{}, nil},
		{"chain", map[string][]string{"web": {"api"}, "api": {"db"}, "db": nil}, []string{"db", "api", "web"}},
		{
			"waves concatenated",
			platform(),
			[]string{
				"cert-manager", "kafka", "postgres", "redis",
				"auth", "ingress-nginx",
				"orders", "payments",
				"api-gateway",
				"web",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustNew(t, tc.deps)
			got, err := order(t, g)
			if err != nil {
				t.Fatalf("Order() error = %v, want nil", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Order()\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}
