package waves_test

import (
	"fmt"
	"slices"
	"testing"

	waves "github.com/yornik/go-practice/exercises/04-waves"
)

// nodes returns node-1 ... node-n in a slice whose length and
// capacity are both n.
func nodes(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("node-%d", i+1)
	}
	return names
}

type orderCase struct {
	name  string
	names []string
	want  []string
}

func runOrderCases(t *testing.T, cases []orderCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := waves.Order(slices.Clone(tc.names))
			if !slices.Equal(got, tc.want) {
				t.Errorf("Order(%q)\n got  %q\n want %q", tc.names, got, tc.want)
			}
		})
	}
}

func TestOrderNumbers(t *testing.T) {
	runOrderCases(t, []orderCase{
		{"node-9 before node-10", []string{"node-10", "node-9"}, []string{"node-9", "node-10"}},
		{"already in order", []string{"node-9", "node-10"}, []string{"node-9", "node-10"}},
		{"1, 2, 10, 20, 100", []string{"node-100", "node-20", "node-2", "node-10", "node-1"},
			[]string{"node-1", "node-2", "node-10", "node-20", "node-100"}},
		{"only digits", []string{"10", "9", "1"}, []string{"1", "9", "10"}},
		{"9 digits", []string{"node-999999999", "node-100000000", "node-99999999"},
			[]string{"node-99999999", "node-100000000", "node-999999999"}},
		{"number 0", []string{"node-1", "node-0"}, []string{"node-0", "node-1"}},
		{"1 name", []string{"node-1"}, []string{"node-1"}},
		{"duplicates are kept", []string{"node-2", "node-1", "node-2"}, []string{"node-1", "node-2", "node-2"}},
	})
}

func TestOrderPrefixes(t *testing.T) {
	runOrderCases(t, []orderCase{
		{"the prefix decides first", []string{"web-1", "checkout-2"}, []string{"checkout-2", "web-1"}},
		{"the prefix decides before the number", []string{"nodes-2", "node-10"}, []string{"node-10", "nodes-2"}},
		{"a prefix that is the start of a longer prefix comes first", []string{"node-1", "node"}, []string{"node", "node-1"}},
		{"name without a number before name with a number", []string{"web1", "web"}, []string{"web", "web1"}},
		{"name without a number before every number", []string{"web10", "web2", "web"}, []string{"web", "web2", "web10"}},
		{"prefix with a dash", []string{"web-1", "web-", "web"}, []string{"web", "web-", "web-1"}},
		{"names without numbers", []string{"payments", "checkout", "orders"}, []string{"checkout", "orders", "payments"}},
		{"plain string order compares bytes", []string{"web-1", "Web-2"}, []string{"Web-2", "web-1"}},
		{"only the digits at the end are a number", []string{"rack2-node1", "rack10-node2", "rack2-node10", "rack2-node9"},
			[]string{"rack10-node2", "rack2-node1", "rack2-node9", "rack2-node10"}},
		{"digits followed by a letter are text", []string{"v10a", "v9a"}, []string{"v10a", "v9a"}},
	})
}

func TestOrderLeadingZeros(t *testing.T) {
	runOrderCases(t, []orderCase{
		{"node-007 before node-7", []string{"node-7", "node-007"}, []string{"node-007", "node-7"}},
		{"equal values in plain string order", []string{"node-7", "node-07", "node-007"}, []string{"node-007", "node-07", "node-7"}},
		{"the value decides before the zeros", []string{"node-8", "node-0412", "node-09"}, []string{"node-8", "node-09", "node-0412"}},
		{"canary-h3-01 before canary-h3-2", []string{"canary-h3-2", "canary-h3-01"}, []string{"canary-h3-01", "canary-h3-2"}},
	})
}

func TestOrderMixed(t *testing.T) {
	runOrderCases(t, []orderCase{
		{
			"12 names",
			[]string{
				"chkpt-store-10", "node-9", "canary-h3-01", "node-0412",
				"chkpt-store-2", "node-10", "node", "node-412",
				"checkout", "node-1", "chkpt-store-1", "canary-h3-1",
			},
			[]string{
				"canary-h3-01", "canary-h3-1", "checkout",
				"chkpt-store-1", "chkpt-store-2", "chkpt-store-10",
				"node", "node-1", "node-9", "node-10", "node-0412", "node-412",
			},
		},
	})
}

func TestOrderEmpty(t *testing.T) {
	for _, names := range [][]string{nil, {}} {
		if got := waves.Order(names); len(got) != 0 {
			t.Errorf("Order(%#v) = %q, want an empty result", names, got)
		}
	}
}

func TestOrderKeepsInput(t *testing.T) {
	t.Run("the input order is unchanged", func(t *testing.T) {
		names := []string{"node-10", "node-9", "node-1"}
		got := waves.Order(names)
		if want := []string{"node-10", "node-9", "node-1"}; !slices.Equal(names, want) {
			t.Errorf("Order changed its input: after the call names is %q, want %q", names, want)
		}
		if want := []string{"node-1", "node-9", "node-10"}; !slices.Equal(got, want) {
			t.Errorf("Order(%q) = %q, want %q", []string{"node-10", "node-9", "node-1"}, got, want)
		}
	})

	t.Run("changing the result does not change the input", func(t *testing.T) {
		// Already in order, so a result that shares the input's array is
		// not visible from the order alone.
		names := []string{"node-1", "node-2", "node-3"}
		got := waves.Order(names)
		if len(got) != 3 {
			t.Fatalf("Order(%q) = %q, want 3 names", names, got)
		}
		got[0] = "changed"
		if want := []string{"node-1", "node-2", "node-3"}; !slices.Equal(names, want) {
			t.Errorf("after result[0] = %q, names is %q, want %q: the result shares the array of names", "changed", names, want)
		}
	})

	t.Run("appending to the result does not change the input", func(t *testing.T) {
		backing := []string{"node-1", "node-2", "node-3", "SPARE"}
		names := backing[:3]
		got := waves.Order(names)
		_ = append(got, "node-4")
		if backing[3] != "SPARE" {
			t.Errorf("after appending %q to the result, the spare capacity of names holds %q, want %q: the result shares the array of names", "node-4", backing[3], "SPARE")
		}
	})
}

func ExampleOrder() {
	names := []string{"node-10", "node-9", "web", "node-007", "node-7", "web1", "checkout-2"}
	fmt.Println(waves.Order(names))
	fmt.Println(names)
	// Output:
	// [checkout-2 node-007 node-7 node-9 node-10 web web1]
	// [node-10 node-9 web node-007 node-7 web1 checkout-2]
}
