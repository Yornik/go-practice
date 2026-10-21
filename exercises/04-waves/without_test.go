package waves_test

import (
	"fmt"
	"slices"
	"testing"

	waves "github.com/yornik/go-practice/exercises/04-waves"
)

type withoutCase struct {
	name        string
	names, skip []string
	want        []string
}

func TestWithout(t *testing.T) {
	cases := []withoutCase{
		{
			"order is kept",
			[]string{"node-3", "node-1", "canary-h3-01", "node-2"},
			[]string{"canary-h3-01"},
			[]string{"node-3", "node-1", "node-2"},
		},
		{
			"several names skipped",
			[]string{"node-1", "node-2", "node-3", "node-4", "node-5"},
			[]string{"node-4", "node-2"},
			[]string{"node-1", "node-3", "node-5"},
		},
		{
			"skip names that are not in names change nothing",
			[]string{"node-1", "node-2"},
			[]string{"node-404", "chkpt-store-1"},
			[]string{"node-1", "node-2"},
		},
		{
			"every copy of a skipped name is dropped",
			[]string{"node-1", "node-2", "node-1", "node-3", "node-1"},
			[]string{"node-1"},
			[]string{"node-2", "node-3"},
		},
		{
			"every copy of another name is kept",
			[]string{"node-2", "node-1", "node-2", "node-3"},
			[]string{"node-3"},
			[]string{"node-2", "node-1", "node-2"},
		},
		{
			"a name twice in skip",
			[]string{"node-1", "node-2"},
			[]string{"node-2", "node-2"},
			[]string{"node-1"},
		},
		{
			"letter case counts",
			[]string{"canary-h3-01", "node-1"},
			[]string{"CANARY-H3-01"},
			[]string{"canary-h3-01", "node-1"},
		},
		{
			"a name must match whole",
			[]string{"node-1", "node-10", "node-11"},
			[]string{"node-1"},
			[]string{"node-10", "node-11"},
		},
		{
			"empty skip gives the same names",
			[]string{"node-2", "node-1"},
			[]string{},
			[]string{"node-2", "node-1"},
		},
		{
			"nil skip gives the same names",
			[]string{"node-2", "node-1"},
			nil,
			[]string{"node-2", "node-1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := waves.Without(slices.Clone(tc.names), slices.Clone(tc.skip))
			if !slices.Equal(got, tc.want) {
				t.Errorf("Without(%q, %q)\n got  %q\n want %q", tc.names, tc.skip, got, tc.want)
			}
		})
	}
}

func TestWithoutEmptyResult(t *testing.T) {
	cases := []struct {
		name        string
		names, skip []string
	}{
		{"everything skipped", []string{"node-1", "node-2", "node-1"}, []string{"node-2", "node-1"}},
		{"nil names", nil, []string{"node-1"}},
		{"empty names", []string{}, []string{"node-1"}},
		{"nil names and nil skip", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := waves.Without(tc.names, tc.skip); len(got) != 0 {
				t.Errorf("Without(%#v, %#v) = %q, want an empty result", tc.names, tc.skip, got)
			}
		})
	}
}

func TestWithoutKeepsInput(t *testing.T) {
	t.Run("the input is unchanged", func(t *testing.T) {
		names := []string{"node-1", "node-2", "node-3", "node-2"}
		skip := []string{"node-1"}
		got := waves.Without(names, skip)
		if want := []string{"node-1", "node-2", "node-3", "node-2"}; !slices.Equal(names, want) {
			t.Errorf("Without(names, %q) changed names: it is %q after the call, want %q", skip, names, want)
		}
		if want := []string{"node-1"}; !slices.Equal(skip, want) {
			t.Errorf("Without changed skip: it is %q after the call, want %q", skip, want)
		}
		if want := []string{"node-2", "node-3", "node-2"}; !slices.Equal(got, want) {
			t.Errorf("Without(%q, %q) = %q, want %q", []string{"node-1", "node-2", "node-3", "node-2"}, skip, got, want)
		}
	})

	for _, skip := range [][]string{nil, {"node-404"}} {
		t.Run(fmt.Sprintf("changing the result does not change the input, skip %q", skip), func(t *testing.T) {
			names := []string{"node-1", "node-2"}
			got := waves.Without(names, skip)
			if len(got) != 2 {
				t.Fatalf("Without(%q, %q) = %q, want 2 names", names, skip, got)
			}
			got[0] = "changed"
			if want := []string{"node-1", "node-2"}; !slices.Equal(names, want) {
				t.Errorf("after result[0] = %q, names is %q, want %q: the result shares the array of names", "changed", names, want)
			}
		})

		t.Run(fmt.Sprintf("changing the input does not change the result, skip %q", skip), func(t *testing.T) {
			names := []string{"node-1", "node-2"}
			got := waves.Without(names, skip)
			names[1] = "changed"
			if want := []string{"node-1", "node-2"}; !slices.Equal(got, want) {
				t.Errorf("after names[1] = %q, the result is %q, want %q: the result shares the array of names", "changed", got, want)
			}
		})
	}
}

// TestWithoutSpareCapacity passes names with room behind them. A result
// that reuses the array of names writes into that room, or over names.
func TestWithoutSpareCapacity(t *testing.T) {
	cases := []struct {
		name string
		skip []string
		want []string
	}{
		{"skip b", []string{"b"}, []string{"a", "c"}},
		{"skip a", []string{"a"}, []string{"b", "c"}},
		{"skip nothing", nil, []string{"a", "b", "c"}},
		{"skip everything", []string{"a", "b", "c"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backing := []string{"a", "b", "c", "SENTINEL"}
			names := backing[:3]
			got := waves.Without(names, tc.skip)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Without(%q, %q) = %q, want %q", []string{"a", "b", "c"}, tc.skip, got, tc.want)
			}
			if want := []string{"a", "b", "c"}; !slices.Equal(names, want) {
				t.Errorf("Without(names, %q) wrote into names: it is %q after the call, want %q", tc.skip, names, want)
			}
			if backing[3] != "SENTINEL" {
				t.Errorf("Without(names, %q) wrote %q into the spare capacity of names, want it untouched", tc.skip, backing[3])
			}

			_ = append(got, "d", "e")
			if !slices.Equal(backing, []string{"a", "b", "c", "SENTINEL"}) {
				t.Errorf("after appending %q to the result of Without(names, %q), the array of names holds %q, want %q: the result shares the array of names",
					[]string{"d", "e"}, tc.skip, backing, []string{"a", "b", "c", "SENTINEL"})
			}
		})
	}
}

func ExampleWithout() {
	names := []string{"node-1", "canary-h3-01", "node-2", "node-1"}
	fmt.Println(waves.Without(names, []string{"canary-h3-01", "node-404"}))
	// Output:
	// [node-1 node-2 node-1]
}
