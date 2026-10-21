package loglevels_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	loglevels "github.com/yornik/go-practice/exercises/01-loglevels"
)

// mixed has 1 line per level, a line without a level, a blank line and a
// line with an unknown level. The trailing spaces on the warn line must come
// back unchanged.
var mixed = []string{
	"service=gateway level=INFO msg=started",
	"node=vl-gpu-0412 lvl=err msg=ecc_error",
	"",
	"node=vl-gpu-0437 lvl=dbg msg=fan_event",
	"service=hushd severity=critical msg=model_oom",
	"node=vl-gpu-0451 msg=boot",
	"service=gateway level=WARN msg=slow_write  ",
	"service=gateway level=verbose msg=queue_depth",
}

func TestAtLeastLevels(t *testing.T) {
	tests := []struct {
		minLevel string
		want     []string
	}{
		{"debug", []string{mixed[0], mixed[1], mixed[3], mixed[4], mixed[6]}},
		{"info", []string{mixed[0], mixed[1], mixed[4], mixed[6]}},
		{"warn", []string{mixed[1], mixed[4], mixed[6]}},
		{"error", []string{mixed[1], mixed[4]}},
		{"fatal", []string{mixed[4]}},
	}
	for _, tc := range tests {
		t.Run(tc.minLevel, func(t *testing.T) {
			got, err := loglevels.AtLeast(mixed, tc.minLevel)
			if err != nil {
				t.Fatalf("AtLeast(mixed, %q) error = %v, want nil", tc.minLevel, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("AtLeast(%q, %q)\n got  %q\n want %q", mixed, tc.minLevel, got, tc.want)
			}
		})
	}
}

func TestAtLeastNoMatch(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		minLevel string
	}{
		{"nil input", nil, "debug"},
		{"only blank and unknown lines", []string{"", "  ", "msg=boot", "level=verbose"}, "debug"},
		{"nothing severe enough", []string{"level=info", "lvl=warn", "severity=err"}, "fatal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loglevels.AtLeast(tc.lines, tc.minLevel)
			if err != nil {
				t.Fatalf("AtLeast(%q, %q) error = %v, want nil", tc.lines, tc.minLevel, err)
			}
			if len(got) != 0 {
				t.Errorf("AtLeast(%q, %q) = %q, want no lines", tc.lines, tc.minLevel, got)
			}
		})
	}
}

func TestAtLeastBadMinimum(t *testing.T) {
	// Only the 5 lowercase names are valid. Spellings that Level accepts in a
	// line, such as WARNING or err, are not.
	for _, minLevel := range []string{"WARNING", "Warn", "", "warning", "err", "unknown", " warn"} {
		t.Run(fmt.Sprintf("%q", minLevel), func(t *testing.T) {
			got, err := loglevels.AtLeast(mixed, minLevel)
			if err == nil {
				t.Fatalf("AtLeast(mixed, %q) error = nil, want an error", minLevel)
			}
			if got != nil {
				t.Errorf("AtLeast(mixed, %q) = %q, want a nil slice with the error", minLevel, got)
			}
			if quoted := fmt.Sprintf("%q", minLevel); !strings.Contains(err.Error(), quoted) {
				t.Errorf("AtLeast(mixed, %q) error = %q, want it to contain %s", minLevel, err, quoted)
			}
		})
	}
}
