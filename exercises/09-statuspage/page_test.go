package statuspage_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	statuspage "github.com/yornik/go-practice/exercises/09-statuspage"
)

// comp is 1 component and the status a test sets for it.
type comp struct {
	name   string
	status statuspage.Status
}

// build returns a page with the given components, each set to its status
// with Set.
func build(t *testing.T, comps ...comp) *statuspage.Page {
	t.Helper()
	names := make([]string, len(comps))
	for i, c := range comps {
		names[i] = c.name
	}
	p := statuspage.NewPage(names...)
	for _, c := range comps {
		if err := p.Set(c.name, c.status); err != nil {
			t.Fatalf("Set(%q, %s) = %v, want nil", c.name, constName(c.status), err)
		}
	}
	return p
}

// describe prints components for a failure message: [API=Operational].
func describe(comps []comp) string {
	parts := make([]string, len(comps))
	for i, c := range comps {
		parts[i] = c.name + "=" + constName(c.status)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// emptyPages returns the 2 ways to get a page without components.
func emptyPages() []struct {
	name string
	page *statuspage.Page
} {
	return []struct {
		name string
		page *statuspage.Page
	}{
		{"NewPage()", statuspage.NewPage()},
		{"&Page{}", &statuspage.Page{}},
	}
}

func checkGet(t *testing.T, p *statuspage.Page, name string, want statuspage.Status, wantOK bool) {
	t.Helper()
	got, ok := p.Get(name)
	if got != want || ok != wantOK {
		t.Errorf("Get(%q) = %s, %v; want %s, %v", name, constName(got), ok, constName(want), wantOK)
	}
}

func TestPageSetGet(t *testing.T) {
	p := statuspage.NewPage("API", "Checkout", "Inference API")

	// Every component starts Unknown.
	checkGet(t, p, "API", statuspage.Unknown, true)
	checkGet(t, p, "Checkout", statuspage.Unknown, true)
	checkGet(t, p, "Inference API", statuspage.Unknown, true)

	// A name that is not on the page.
	checkGet(t, p, "Payments", statuspage.Unknown, false)
	checkGet(t, p, "", statuspage.Unknown, false)

	if err := p.Set("Inference API", statuspage.MajorOutage); err != nil {
		t.Fatalf("Set(%q, MajorOutage) = %v, want nil", "Inference API", err)
	}
	checkGet(t, p, "Inference API", statuspage.MajorOutage, true)
	checkGet(t, p, "API", statuspage.Unknown, true)
	checkGet(t, p, "Checkout", statuspage.Unknown, true)

	// The last Set wins.
	if err := p.Set("Inference API", statuspage.Operational); err != nil {
		t.Fatalf("Set(%q, Operational) = %v, want nil", "Inference API", err)
	}
	checkGet(t, p, "Inference API", statuspage.Operational, true)

	// Every constant is a valid status, Unknown included.
	for _, s := range all {
		if err := p.Set("API", s); err != nil {
			t.Errorf("Set(%q, %s) = %v, want nil", "API", constName(s), err)
			continue
		}
		checkGet(t, p, "API", s, true)
	}
}

func TestPageSetDuplicateNames(t *testing.T) {
	p := statuspage.NewPage("API", "API", "Checkout")
	if err := p.Set("API", statuspage.Degraded); err != nil {
		t.Fatalf("Set(%q, Degraded) = %v, want nil", "API", err)
	}
	checkGet(t, p, "API", statuspage.Degraded, true)
	checkGet(t, p, "Checkout", statuspage.Unknown, true)
}

func TestPageSetUnknownComponent(t *testing.T) {
	tests := []struct {
		name   string
		status statuspage.Status
		why    string
	}{
		{"Payments", statuspage.Operational, "not on the page"},
		{"api", statuspage.Operational, "letter case differs"},
		{"API ", statuspage.Operational, "trailing space"},
		{"", statuspage.Operational, "empty name"},
		// Both are wrong: the component is checked first.
		{"Payments", statuspage.Status(42), "unknown component and invalid status"},
	}
	for _, tc := range tests {
		p := statuspage.NewPage("API", "Checkout")
		if err := p.Set("API", statuspage.Operational); err != nil {
			t.Fatalf("Set(%q, Operational) = %v, want nil", "API", err)
		}

		err := p.Set(tc.name, tc.status)
		if err == nil {
			t.Errorf("Set(%q, %s) = nil; want an error wrapping ErrUnknownComponent (%s)", tc.name, constName(tc.status), tc.why)
			continue
		}
		if !errors.Is(err, statuspage.ErrUnknownComponent) {
			t.Errorf("Set(%q, %s) error = %v; want it to wrap ErrUnknownComponent (%s)", tc.name, constName(tc.status), err, tc.why)
		}
		if quoted := fmt.Sprintf("%q", tc.name); !strings.Contains(err.Error(), quoted) {
			t.Errorf("Set(%q, %s) error = %q; want it to contain the name as %s", tc.name, constName(tc.status), err, quoted)
		}

		// A failed Set changes nothing.
		checkGet(t, p, tc.name, statuspage.Unknown, false)
		checkGet(t, p, "API", statuspage.Operational, true)
		checkGet(t, p, "Checkout", statuspage.Unknown, true)
	}
}

func TestPageSetInvalidStatus(t *testing.T) {
	for _, s := range []statuspage.Status{-1, 6, 42} {
		p := statuspage.NewPage("API")
		if err := p.Set("API", statuspage.PartialOutage); err != nil {
			t.Fatalf("Set(%q, PartialOutage) = %v, want nil", "API", err)
		}

		err := p.Set("API", s)
		if !errors.Is(err, statuspage.ErrUnknownStatus) {
			t.Errorf("Set(%q, %s) = %v; want an error wrapping ErrUnknownStatus", "API", constName(s), err)
		}
		// A failed Set changes nothing.
		checkGet(t, p, "API", statuspage.PartialOutage, true)
	}
}

func TestPageSetEmptyPage(t *testing.T) {
	for _, e := range emptyPages() {
		got, ok := e.page.Get("API")
		if got != statuspage.Unknown || ok {
			t.Errorf("%s: Get(%q) = %s, %v; want Unknown, false", e.name, "API", constName(got), ok)
		}
		err := e.page.Set("API", statuspage.Operational)
		if !errors.Is(err, statuspage.ErrUnknownComponent) {
			t.Errorf("%s: Set(%q, Operational) = %v; want an error wrapping ErrUnknownComponent", e.name, "API", err)
		}
	}
}

func TestPageOverallEmpty(t *testing.T) {
	for _, e := range emptyPages() {
		if got := e.page.Overall(); got != statuspage.Unknown {
			t.Errorf("%s: Overall() = %s, want Unknown", e.name, constName(got))
		}
	}
}

func TestPageOverallNewPage(t *testing.T) {
	p := statuspage.NewPage("API", "Checkout")
	if got := p.Overall(); got != statuspage.Unknown {
		t.Errorf("NewPage(%q, %q).Overall() = %s, want Unknown (nobody has reported)", "API", "Checkout", constName(got))
	}
}

func TestPageOverallWorstWins(t *testing.T) {
	tests := []struct {
		name  string
		comps []comp
		want  statuspage.Status
	}{
		{
			"1 component",
			[]comp{{"API", statuspage.Degraded}},
			statuspage.Degraded,
		},
		{
			"all operational",
			[]comp{{"API", statuspage.Operational}, {"Checkout", statuspage.Operational}, {"Push", statuspage.Operational}},
			statuspage.Operational,
		},
		{
			"1 unknown among operational",
			[]comp{{"API", statuspage.Operational}, {"Inference API", statuspage.Unknown}, {"Push", statuspage.Operational}},
			statuspage.Unknown,
		},
		{
			"maintenance among operational",
			[]comp{{"API", statuspage.Operational}, {"Checkout", statuspage.Maintenance}},
			statuspage.Maintenance,
		},
		{
			"unknown is more severe than maintenance",
			[]comp{{"Checkout", statuspage.Maintenance}, {"Inference API", statuspage.Unknown}},
			statuspage.Unknown,
		},
		{
			"degraded is more severe than unknown",
			[]comp{{"Inference API", statuspage.Unknown}, {"Push", statuspage.Degraded}},
			statuspage.Degraded,
		},
		{
			"partial outage is more severe than degraded",
			[]comp{{"API", statuspage.PartialOutage}, {"Push", statuspage.Degraded}},
			statuspage.PartialOutage,
		},
		{
			"1 major outage beats everything",
			[]comp{
				{"API", statuspage.PartialOutage},
				{"Checkout", statuspage.Maintenance},
				{"Inference API", statuspage.MajorOutage},
				{"Push", statuspage.Degraded},
				{"Training cluster", statuspage.Unknown},
				{"Console", statuspage.Operational},
			},
			statuspage.MajorOutage,
		},
		{
			"all under maintenance",
			[]comp{{"API", statuspage.Maintenance}, {"Checkout", statuspage.Maintenance}},
			statuspage.Maintenance,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := build(t, tc.comps...)
			if got := p.Overall(); got != tc.want {
				t.Errorf("Overall() of %s = %s, want %s", describe(tc.comps), constName(got), constName(tc.want))
			}
		})
	}
}

func TestPageOverallSeverityOrder(t *testing.T) {
	// From least to most severe.
	order := []statuspage.Status{
		statuspage.Operational,
		statuspage.Maintenance,
		statuspage.Unknown,
		statuspage.Degraded,
		statuspage.PartialOutage,
		statuspage.MajorOutage,
	}
	for i, a := range order {
		for j, b := range order {
			comps := []comp{{"API", a}, {"Checkout", b}}
			want := order[max(i, j)]
			p := build(t, comps...)
			if got := p.Overall(); got != want {
				t.Errorf("Overall() of %s = %s, want %s", describe(comps), constName(got), constName(want))
			}
		}
	}
}

func TestPageOverallFollowsUpdates(t *testing.T) {
	p := build(t, comp{"API", statuspage.Operational}, comp{"Inference API", statuspage.Operational})
	steps := []struct {
		name   string
		status statuspage.Status
		want   statuspage.Status
	}{
		{"Inference API", statuspage.MajorOutage, statuspage.MajorOutage},
		{"API", statuspage.Degraded, statuspage.MajorOutage},
		{"Inference API", statuspage.Operational, statuspage.Degraded},
		{"API", statuspage.Operational, statuspage.Operational},
	}
	for _, st := range steps {
		if err := p.Set(st.name, st.status); err != nil {
			t.Fatalf("Set(%q, %s) = %v, want nil", st.name, constName(st.status), err)
		}
		if got := p.Overall(); got != st.want {
			t.Errorf("after Set(%q, %s): Overall() = %s, want %s", st.name, constName(st.status), constName(got), constName(st.want))
		}
	}
}
