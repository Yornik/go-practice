package statuspage_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	statuspage "github.com/yornik/go-practice/exercises/09-statuspage"
)

// all holds the 6 constants in their numeric order.
var all = []statuspage.Status{
	statuspage.Unknown,
	statuspage.Operational,
	statuspage.Maintenance,
	statuspage.Degraded,
	statuspage.PartialOutage,
	statuspage.MajorOutage,
}

// constName returns the Go name of a constant, so that a failure message
// does not depend on the String method under test.
func constName(s statuspage.Status) string {
	names := []string{"Unknown", "Operational", "Maintenance", "Degraded", "PartialOutage", "MajorOutage"}
	if s >= 0 && int(s) < len(names) {
		return names[s]
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

func TestStatusString(t *testing.T) {
	var zero statuspage.Status
	tests := []struct {
		s    statuspage.Status
		want string
	}{
		{statuspage.Unknown, "unknown"},
		{statuspage.Operational, "operational"},
		{statuspage.Maintenance, "under_maintenance"},
		{statuspage.Degraded, "degraded_performance"},
		{statuspage.PartialOutage, "partial_outage"},
		{statuspage.MajorOutage, "major_outage"},
		{zero, "unknown"},
		{statuspage.Status(-1), "Status(-1)"},
		{statuspage.Status(6), "Status(6)"},
		{statuspage.Status(42), "Status(42)"},
	}
	for _, tc := range tests {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("%s.String() = %q, want %q", constName(tc.s), got, tc.want)
		}
		// fmt calls String for %v and %s, because Status is a fmt.Stringer.
		if got := fmt.Sprint(tc.s); got != tc.want {
			t.Errorf("fmt.Sprint(%s) = %q, want %q", constName(tc.s), got, tc.want)
		}
	}
}

func TestStatusParse(t *testing.T) {
	tests := []struct {
		in   string
		want statuspage.Status
	}{
		{"unknown", statuspage.Unknown},
		{"operational", statuspage.Operational},
		{"under_maintenance", statuspage.Maintenance},
		{"degraded_performance", statuspage.Degraded},
		{"partial_outage", statuspage.PartialOutage},
		{"major_outage", statuspage.MajorOutage},

		// The 2 aliases.
		{"maintenance", statuspage.Maintenance},
		{"degraded", statuspage.Degraded},

		// Any letter case.
		{"UNKNOWN", statuspage.Unknown},
		{"Operational", statuspage.Operational},
		{"Under_Maintenance", statuspage.Maintenance},
		{"dEgRaDeD_pErFoRmAnCe", statuspage.Degraded},
		{"PARTIAL_OUTAGE", statuspage.PartialOutage},
		{"Major_Outage", statuspage.MajorOutage},
		{"MAINTENANCE", statuspage.Maintenance},
		{"Degraded", statuspage.Degraded},
	}
	for _, tc := range tests {
		got, err := statuspage.ParseStatus(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseStatus(%q) = %s, %v; want %s, nil", tc.in, constName(got), err, constName(tc.want))
		}
	}
}

func TestStatusParseErrors(t *testing.T) {
	tests := []struct {
		in  string
		why string
	}{
		{"", "empty"},
		{"fine", "not a status word"},
		{"down", "not a status word"},
		{"operational ", "trailing space"},
		{" operational", "leading space"},
		{"operational\n", "trailing newline"},
		{"partial outage", "space instead of _"},
		{"partial-outage", "- instead of _"},
		{"partialoutage", "no _"},
		{"under maintenance", "space instead of _"},
		{"major", "half a word"},
		{"outage", "half a word"},
		{"degraded_", "trailing _"},
		{"Status(1)", "the String form of an invalid value"},
		{"1", "a number"},
	}
	for _, tc := range tests {
		got, err := statuspage.ParseStatus(tc.in)
		if err == nil {
			t.Errorf("ParseStatus(%q) = %s, nil; want an error wrapping ErrUnknownStatus (%s)", tc.in, constName(got), tc.why)
			continue
		}
		if !errors.Is(err, statuspage.ErrUnknownStatus) {
			t.Errorf("ParseStatus(%q) error = %v; want it to wrap ErrUnknownStatus (errors.Is)", tc.in, err)
		}
		if quoted := fmt.Sprintf("%q", tc.in); !strings.Contains(err.Error(), quoted) {
			t.Errorf("ParseStatus(%q) error = %q; want it to contain the input as %s", tc.in, err, quoted)
		}
		if got != statuspage.Unknown {
			t.Errorf("ParseStatus(%q) = %s with an error; want Unknown", tc.in, constName(got))
		}
	}
}

func TestStatusRoundTrip(t *testing.T) {
	for _, s := range all {
		word := s.String()
		got, err := statuspage.ParseStatus(word)
		if err != nil || got != s {
			t.Errorf("ParseStatus(%q) = %s, %v; want %s, nil (the String of %s)", word, constName(got), err, constName(s), constName(s))
		}
		upper := strings.ToUpper(word)
		got, err = statuspage.ParseStatus(upper)
		if err != nil || got != s {
			t.Errorf("ParseStatus(%q) = %s, %v; want %s, nil", upper, constName(got), err, constName(s))
		}
	}
}
