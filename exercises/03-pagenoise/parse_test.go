package pagenoise_test

import (
	"fmt"
	"strings"
	"testing"

	pagenoise "github.com/yornik/go-practice/exercises/03-pagenoise"
)

func TestParsePageGood(t *testing.T) {
	tests := []struct {
		name string
		line string
		want pagenoise.Page
	}{
		{
			"a page at night",
			"2026-09-23 03:12 AirflowLow canary-h3-01",
			pagenoise.Page{Day: "2026-09-23", Hour: 3, Minute: 12, Alert: "AirflowLow", Source: "canary-h3-01"},
		},
		{
			"a page in the afternoon",
			"2026-09-18 14:05 CheckpointDiskFull chkpt-store-2",
			pagenoise.Page{Day: "2026-09-18", Hour: 14, Minute: 5, Alert: "CheckpointDiskFull", Source: "chkpt-store-2"},
		},
		{
			"the first minute of the day",
			"2026-09-19 00:00 NotifyQueueBacklog notify-1",
			pagenoise.Page{Day: "2026-09-19", Hour: 0, Minute: 0, Alert: "NotifyQueueBacklog", Source: "notify-1"},
		},
		{
			"the last minute of the day",
			"2026-09-19 23:59 NotifyQueueBacklog notify-1",
			pagenoise.Page{Day: "2026-09-19", Hour: 23, Minute: 59, Alert: "NotifyQueueBacklog", Source: "notify-1"},
		},
		{
			"the day is kept as written and not checked",
			"yesterday 07:30 CheckoutErrorRate checkout-1",
			pagenoise.Page{Day: "yesterday", Hour: 7, Minute: 30, Alert: "CheckoutErrorRate", Source: "checkout-1"},
		},
		{
			"alert and source keep their letter case",
			"2026-09-20 10:45 airflowLOW VL-GPU-0412",
			pagenoise.Page{Day: "2026-09-20", Hour: 10, Minute: 45, Alert: "airflowLOW", Source: "VL-GPU-0412"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkParse(t, tc.line, tc.want)
		})
	}
}

func TestParsePageWhitespace(t *testing.T) {
	want := pagenoise.Page{Day: "2026-09-25", Hour: 22, Minute: 0, Alert: "AirflowLow", Source: "canary-h3-01"}
	tests := []struct {
		name string
		line string
	}{
		{"tabs between the fields", "2026-09-25\t22:00\tAirflowLow\tcanary-h3-01"},
		{"several spaces between the fields", "2026-09-25   22:00  AirflowLow    canary-h3-01"},
		{"spaces before and after the line", "   2026-09-25 22:00 AirflowLow canary-h3-01   "},
		{"a trailing \\r from a Windows line ending", "2026-09-25 22:00 AirflowLow canary-h3-01\r"},
		{"a trailing \\n", "2026-09-25 22:00 AirflowLow canary-h3-01\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkParse(t, tc.line, want)
		})
	}
}

func TestParsePageFieldCount(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"an empty line", ""},
		{"a whitespace-only line", " \t \r"},
		{"1 field", "2026-09-23"},
		{"3 fields: no source", "2026-09-23 03:12 AirflowLow"},
		{"5 fields: a note after the source", "2026-09-23 03:12 AirflowLow canary-h3-01 acked"},
		{"5 fields: a space inside the alert name", "2026-09-23 03:12 Airflow Low canary-h3-01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkParseError(t, tc.line)
		})
	}
}

func TestParsePageBadTime(t *testing.T) {
	tests := []struct {
		name string
		time string
	}{
		{"hour without a leading zero", "3:12"},
		{"minute without a leading zero", "03:5"},
		{"hour with 3 digits", "003:12"},
		{"hour 24", "24:00"},
		{"hour 99", "99:30"},
		{"minute 60", "12:60"},
		{"a dash instead of a colon", "12-30"},
		{"no colon", "1230"},
		{"seconds", "12:30:00"},
		{"a plus sign in the hour", "+1:00"},
		{"a minus sign in the hour", "-1:00"},
		{"a plus sign in the minute", "12:+5"},
		{"letters", "ab:cd"},
		{"a letter in the minute", "12:3a"},
		{"2 colons", "12::0"},
		{"fullwidth digits", "１２:３０"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkParseError(t, fmt.Sprintf("2026-09-23 %s AirflowLow canary-h3-01", tc.time))
		})
	}
}

// checkParse fails the test unless ParsePage(line) returns want and no error.
func checkParse(t *testing.T, line string, want pagenoise.Page) {
	t.Helper()
	got, err := pagenoise.ParsePage(line)
	if err != nil {
		t.Fatalf("ParsePage(%q) error = %v, want nil", line, err)
	}
	if got != want {
		t.Errorf("ParsePage(%q)\n got  %+v\n want %+v", line, got, want)
	}
}

// checkParseError fails the test unless ParsePage(line) returns the zero Page
// and an error whose message contains the line quoted with %q.
func checkParseError(t *testing.T, line string) {
	t.Helper()
	got, err := pagenoise.ParsePage(line)
	if err == nil {
		t.Fatalf("ParsePage(%q) = %+v, nil; want an error", line, got)
	}
	if got != (pagenoise.Page{}) {
		t.Errorf("ParsePage(%q) returned the page %+v with the error, want the zero Page", line, got)
	}
	if quoted := fmt.Sprintf("%q", line); !strings.Contains(err.Error(), quoted) {
		t.Errorf("ParsePage(%q) error = %q, want it to contain the line quoted with %%q: %s", line, err, quoted)
	}
}
