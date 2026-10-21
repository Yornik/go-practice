package statuspage_test

import (
	"errors"
	"fmt"
	"testing"

	statuspage "github.com/yornik/go-practice/exercises/09-statuspage"
)

func TestPageBannerEmpty(t *testing.T) {
	for _, e := range emptyPages() {
		if got, want := e.page.Banner(), "Status unknown"; got != want {
			t.Errorf("%s: Banner() = %q, want %q", e.name, got, want)
		}
	}
}

func TestPageBannerNewPage(t *testing.T) {
	p := statuspage.NewPage("Checkout", "API", "Inference API")
	want := "Status unknown: API, Checkout, Inference API"
	if got := p.Banner(); got != want {
		t.Errorf("NewPage(%q, %q, %q).Banner() = %q, want %q", "Checkout", "API", "Inference API", got, want)
	}
}

func TestPageBannerDuplicateNames(t *testing.T) {
	p := statuspage.NewPage("API", "Checkout", "API", "API")
	want := "Status unknown: API, Checkout"
	if got := p.Banner(); got != want {
		t.Errorf("NewPage(%q, %q, %q, %q).Banner() = %q, want %q", "API", "Checkout", "API", "API", got, want)
	}
}

func TestPageBannerCases(t *testing.T) {
	tests := []struct {
		name  string
		comps []comp
		want  string
	}{
		{
			"1 component, operational",
			[]comp{{"API", statuspage.Operational}},
			"All systems operational",
		},
		{
			"all operational",
			[]comp{{"API", statuspage.Operational}, {"Checkout", statuspage.Operational}, {"Push notifications", statuspage.Operational}},
			"All systems operational",
		},
		{
			"1 component nobody reported on",
			[]comp{
				{"API", statuspage.Operational},
				{"Checkout", statuspage.Operational},
				{"Inference API", statuspage.Unknown},
				{"Push notifications", statuspage.Operational},
				{"Training cluster", statuspage.Operational},
			},
			"Status unknown: Inference API",
		},
		{
			"under maintenance only",
			[]comp{{"API", statuspage.Operational}, {"Checkout", statuspage.Maintenance}},
			"Under maintenance: Checkout",
		},
		{
			"every status, most severe first, operational left out",
			[]comp{
				{"Console", statuspage.Operational},
				{"Checkout", statuspage.Maintenance},
				{"Training cluster", statuspage.Unknown},
				{"Push notifications", statuspage.Degraded},
				{"API", statuspage.PartialOutage},
				{"Inference API", statuspage.MajorOutage},
			},
			"Major outage: Inference API; Partial outage: API; Degraded performance: Push notifications; " +
				"Status unknown: Training cluster; Under maintenance: Checkout",
		},
		{
			"names in a group from A to Z",
			[]comp{
				{"Push notifications", statuspage.Degraded},
				{"API", statuspage.Operational},
				{"Checkout", statuspage.Degraded},
				{"Inference API", statuspage.Degraded},
			},
			"Degraded performance: Checkout, Inference API, Push notifications",
		},
		{
			"names sort in plain string order",
			[]comp{
				{"chkpt-store-2", statuspage.PartialOutage},
				{"chkpt-store-10", statuspage.PartialOutage},
				{"Push", statuspage.PartialOutage},
				{"api", statuspage.PartialOutage},
			},
			"Partial outage: Push, api, chkpt-store-10, chkpt-store-2",
		},
		{
			"several groups with several names",
			[]comp{
				{"chkpt-store-3", statuspage.MajorOutage},
				{"chkpt-store-1", statuspage.MajorOutage},
				{"chkpt-store-4", statuspage.Unknown},
				{"chkpt-store-2", statuspage.Unknown},
				{"API", statuspage.Operational},
			},
			"Major outage: chkpt-store-1, chkpt-store-3; Status unknown: chkpt-store-2, chkpt-store-4",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := build(t, tc.comps...)
			if got := p.Banner(); got != tc.want {
				t.Errorf("Banner() of %s\n got  %q\n want %q", describe(tc.comps), got, tc.want)
			}
		})
	}
}

func TestPageBannerFollowsUpdates(t *testing.T) {
	p := build(t, comp{"API", statuspage.Operational}, comp{"Inference API", statuspage.Operational})
	steps := []struct {
		name   string
		status statuspage.Status
		want   string
	}{
		{"Inference API", statuspage.MajorOutage, "Major outage: Inference API"},
		{"API", statuspage.MajorOutage, "Major outage: API, Inference API"},
		{"API", statuspage.Operational, "Major outage: Inference API"},
		{"Inference API", statuspage.Unknown, "Status unknown: Inference API"},
		{"Inference API", statuspage.Operational, "All systems operational"},
	}
	for _, st := range steps {
		if err := p.Set(st.name, st.status); err != nil {
			t.Fatalf("Set(%q, %s) = %v, want nil", st.name, constName(st.status), err)
		}
		if got := p.Banner(); got != st.want {
			t.Errorf("after Set(%q, %s): Banner() = %q, want %q", st.name, constName(st.status), got, st.want)
		}
	}
}

func ExamplePage() {
	p := statuspage.NewPage("Inference API", "API", "Checkout", "Push notifications")
	fmt.Println(p.Overall(), "|", p.Banner())

	for _, name := range []string{"API", "Checkout", "Inference API", "Push notifications"} {
		_ = p.Set(name, statuspage.Operational)
	}
	fmt.Println(p.Overall(), "|", p.Banner())

	_ = p.Set("Inference API", statuspage.MajorOutage)
	_ = p.Set("Push notifications", statuspage.Degraded)
	_ = p.Set("Checkout", statuspage.Maintenance)
	fmt.Println(p.Overall(), "|", p.Banner())

	err := p.Set("Payments", statuspage.Operational)
	fmt.Println(errors.Is(err, statuspage.ErrUnknownComponent))
	s, err := statuspage.ParseStatus("partial outage")
	fmt.Println(s, "|", errors.Is(err, statuspage.ErrUnknownStatus))
	// Output:
	// unknown | Status unknown: API, Checkout, Inference API, Push notifications
	// operational | All systems operational
	// major_outage | Major outage: Inference API; Degraded performance: Push notifications; Under maintenance: Checkout
	// true
	// unknown | true
}
