package semver_test

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	semver "github.com/yornik/go-practice/exercises/06-semver"
)

// mustParse parses s or stops the test. Steps 2 and 3 build their versions
// with it, so Parse must work first.
func mustParse(t *testing.T, s string) semver.Version {
	t.Helper()
	v, err := semver.Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v, want nil (finish step 1 first)", s, err)
	}
	return v
}

func TestCompareCases(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		// The incident: 10 is more than 9, even though "1" < "9" as text.
		{"v2.9.3", "v2.10.0", -1},
		{"v2.10.0", "v2.9.3", +1},
		{"v1.10.0", "v1.9.0", +1},
		{"v1.0.10", "v1.0.9", +1},
		{"v10.0.0", "v9.0.0", +1},
		// Major decides before Minor, Minor before Patch.
		{"v1.99.99", "v2.0.0", -1},
		{"v2.0.99", "v2.1.0", -1},
		{"v2.10.0", "v2.10.0", 0},
		{"2.10.0", "v2.10.0", 0},
		// A release is newer than its pre-releases.
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"v1.0.0", "v1.0.0-rc.1", +1},
		{"v1.0.0", "v1.0.0-0", +1},
		// Pre-releases of a newer version are newer than an older release.
		{"v2.10.0-rc.1", "v2.9.3", +1},
		{"v1.0.1-alpha", "v1.0.0", +1},
		// 2 pre-releases of the same version: plain string order.
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
		{"v1.0.0-beta", "v1.0.0-alpha", +1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.0.0-rc.1", "v1.0.0-rc.2", -1},
		{"v1.0.0-RC.1", "v1.0.0-rc.1", -1},
		{"v1.0.0-rc.1", "v1.0.0-rc.1", 0},
	}
	for _, tc := range tests {
		a, b := mustParse(t, tc.a), mustParse(t, tc.b)
		if got := semver.Compare(a, b); got != tc.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// ordered holds 20 distinct versions, oldest first. The pre-releases of
// v1.0.0 are the order from the SemVer 2.0.0 spec, without beta.11, the one
// where plain string order and SemVer disagree.
var ordered = []string{
	"v0.0.0",
	"v0.9.0",
	"v0.10.0",
	"v1.0.0-alpha",
	"v1.0.0-alpha.1",
	"v1.0.0-alpha.beta",
	"v1.0.0-beta",
	"v1.0.0-beta.2",
	"v1.0.0-rc.1",
	"v1.0.0",
	"v1.0.1",
	"v1.2.0",
	"v1.10.0",
	"v2.0.0-rc.1",
	"v2.0.0",
	"v2.9.3",
	"v2.10.0-rc.1",
	"v2.10.0",
	"v9.0.0",
	"v10.0.0",
}

func parseAll(t *testing.T, tags []string) []semver.Version {
	t.Helper()
	vs := make([]semver.Version, len(tags))
	for i, s := range tags {
		vs[i] = mustParse(t, s)
	}
	return vs
}

// TestCompareAllPairs compares every pair of the 20 versions, both ways.
// Compare(x, y) must agree with the positions of x and y in ordered, so it is
// antisymmetric (Compare(x, y) == -Compare(y, x)) and transitive.
func TestCompareAllPairs(t *testing.T) {
	vs := parseAll(t, ordered)
	for i := range vs {
		for j := range vs {
			want := cmp.Compare(i, j)
			if got := semver.Compare(vs[i], vs[j]); got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
}

func TestCompareSortFunc(t *testing.T) {
	want := parseAll(t, ordered)
	reversed := slices.Clone(want)
	slices.Reverse(reversed)
	type input struct {
		name string
		vs   []semver.Version
	}
	inputs := []input{
		{"sorted", slices.Clone(want)},
		{"reversed", reversed},
	}
	// A fixed seed makes every run shuffle the same way.
	rng := rand.New(rand.NewPCG(6, 2026))
	for i := 1; i <= 3; i++ {
		vs := slices.Clone(want)
		rng.Shuffle(len(vs), func(i, j int) { vs[i], vs[j] = vs[j], vs[i] })
		inputs = append(inputs, input{fmt.Sprintf("shuffle %d", i), vs})
	}
	for _, in := range inputs {
		vs := in.vs
		before := slices.Clone(vs)
		slices.SortFunc(vs, semver.Compare)
		if !slices.Equal(vs, want) {
			t.Errorf("%s: slices.SortFunc(%v, semver.Compare)\n got  %v\n want %v", in.name, before, vs, want)
		}
	}
}
