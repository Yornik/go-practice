package waves_test

import (
	"fmt"
	"slices"
	"testing"

	waves "github.com/yornik/go-practice/exercises/04-waves"
)

// feeders returns feeder-1 ... feeder-n in a slice whose length and
// capacity are both n.
func feeders(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("feeder-%d", i+1)
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
		{"feeder-9 before feeder-10", []string{"feeder-10", "feeder-9"}, []string{"feeder-9", "feeder-10"}},
		{"already in order", []string{"feeder-9", "feeder-10"}, []string{"feeder-9", "feeder-10"}},
		{"1, 2, 10, 20, 100", []string{"feeder-100", "feeder-20", "feeder-2", "feeder-10", "feeder-1"},
			[]string{"feeder-1", "feeder-2", "feeder-10", "feeder-20", "feeder-100"}},
		{"only digits", []string{"10", "9", "1"}, []string{"1", "9", "10"}},
		{"9 digits", []string{"feeder-999999999", "feeder-100000000", "feeder-99999999"},
			[]string{"feeder-99999999", "feeder-100000000", "feeder-999999999"}},
		{"number 0", []string{"feeder-1", "feeder-0"}, []string{"feeder-0", "feeder-1"}},
		{"1 name", []string{"feeder-1"}, []string{"feeder-1"}},
		{"duplicates are kept", []string{"feeder-2", "feeder-1", "feeder-2"}, []string{"feeder-1", "feeder-2", "feeder-2"}},
	})
}

func TestOrderPrefixes(t *testing.T) {
	runOrderCases(t, []orderCase{
		{"the prefix decides first", []string{"web-1", "checkout-2"}, []string{"checkout-2", "web-1"}},
		{"the prefix decides before the number", []string{"feeder-2", "feed-10"}, []string{"feed-10", "feeder-2"}},
		{"a prefix that is the start of a longer prefix comes first", []string{"feeder-1", "feeder"}, []string{"feeder", "feeder-1"}},
		{"name without a number before name with a number", []string{"web1", "web"}, []string{"web", "web1"}},
		{"name without a number before every number", []string{"web10", "web2", "web"}, []string{"web", "web2", "web10"}},
		{"prefix with a dash", []string{"web-1", "web-", "web"}, []string{"web", "web-", "web-1"}},
		{"names without numbers", []string{"payments", "checkout", "orders"}, []string{"checkout", "orders", "payments"}},
		{"plain string order compares bytes", []string{"web-1", "Web-2"}, []string{"Web-2", "web-1"}},
		{"only the digits at the end are a number", []string{"rack2-feeder1", "rack10-feeder2", "rack2-feeder10", "rack2-feeder9"},
			[]string{"rack10-feeder2", "rack2-feeder1", "rack2-feeder9", "rack2-feeder10"}},
		{"digits followed by a letter are text", []string{"v10a", "v9a"}, []string{"v10a", "v9a"}},
	})
}

func TestOrderLeadingZeros(t *testing.T) {
	runOrderCases(t, []orderCase{
		{"feeder-007 before feeder-7", []string{"feeder-7", "feeder-007"}, []string{"feeder-007", "feeder-7"}},
		{"equal values in plain string order", []string{"feeder-7", "feeder-07", "feeder-007"}, []string{"feeder-007", "feeder-07", "feeder-7"}},
		{"the value decides before the zeros", []string{"feeder-8", "feeder-0412", "feeder-09"}, []string{"feeder-8", "feeder-09", "feeder-0412"}},
		{"hq-window-01 before hq-window-2", []string{"hq-window-2", "hq-window-01"}, []string{"hq-window-01", "hq-window-2"}},
	})
}

func TestOrderMixed(t *testing.T) {
	runOrderCases(t, []orderCase{
		{
			"12 names",
			[]string{
				"photo-store-10", "feeder-9", "hq-window-01", "feeder-0412",
				"photo-store-2", "feeder-10", "feeder", "feeder-412",
				"checkout", "feeder-1", "photo-store-1", "hq-window-1",
			},
			[]string{
				"checkout", "feeder", "feeder-1", "feeder-9", "feeder-10", "feeder-0412",
				"feeder-412", "hq-window-01", "hq-window-1",
				"photo-store-1", "photo-store-2", "photo-store-10",
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
		names := []string{"feeder-10", "feeder-9", "feeder-1"}
		got := waves.Order(names)
		if want := []string{"feeder-10", "feeder-9", "feeder-1"}; !slices.Equal(names, want) {
			t.Errorf("Order changed its input: after the call names is %q, want %q", names, want)
		}
		if want := []string{"feeder-1", "feeder-9", "feeder-10"}; !slices.Equal(got, want) {
			t.Errorf("Order(%q) = %q, want %q", []string{"feeder-10", "feeder-9", "feeder-1"}, got, want)
		}
	})

	t.Run("changing the result does not change the input", func(t *testing.T) {
		// Already in order, so a result that shares the input's array is
		// not visible from the order alone.
		names := []string{"feeder-1", "feeder-2", "feeder-3"}
		got := waves.Order(names)
		if len(got) != 3 {
			t.Fatalf("Order(%q) = %q, want 3 names", names, got)
		}
		got[0] = "changed"
		if want := []string{"feeder-1", "feeder-2", "feeder-3"}; !slices.Equal(names, want) {
			t.Errorf("after result[0] = %q, names is %q, want %q: the result shares the array of names", "changed", names, want)
		}
	})

	t.Run("appending to the result does not change the input", func(t *testing.T) {
		backing := []string{"feeder-1", "feeder-2", "feeder-3", "SPARE"}
		names := backing[:3]
		got := waves.Order(names)
		_ = append(got, "feeder-4")
		if backing[3] != "SPARE" {
			t.Errorf("after appending %q to the result, the spare capacity of names holds %q, want %q: the result shares the array of names", "feeder-4", backing[3], "SPARE")
		}
	})
}

func ExampleOrder() {
	names := []string{"feeder-10", "feeder-9", "web", "feeder-007", "feeder-7", "web1", "checkout-2"}
	fmt.Println(waves.Order(names))
	fmt.Println(names)
	// Output:
	// [checkout-2 feeder-007 feeder-7 feeder-9 feeder-10 web web1]
	// [feeder-10 feeder-9 web feeder-007 feeder-7 web1 checkout-2]
}
