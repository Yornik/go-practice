package rollcall_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	rollcall "github.com/yornik/go-practice/exercises/07-rollcall"
)

func TestParseHeartbeatValid(t *testing.T) {
	tests := []struct {
		name string
		line string
		want rollcall.Heartbeat
	}{
		{
			name: "UTC with Z",
			line: "canary-h3-01 2026-10-15T08:55:00Z",
			want: hb("canary-h3-01", time.Date(2026, 10, 15, 8, 55, 0, 0, time.UTC)),
		},
		{
			name: "offset +02:00",
			line: "vl-gpu-0412 2026-10-15T10:31:12+02:00",
			want: hb("vl-gpu-0412", time.Date(2026, 10, 15, 10, 31, 12, 0, cest)),
		},
		{
			name: "offset -04:00",
			line: "vl-gpu-0412 2026-10-15T04:31:12-04:00",
			want: hb("vl-gpu-0412", time.Date(2026, 10, 15, 8, 31, 12, 0, time.UTC)),
		},
		{
			name: "fractional seconds",
			line: "vl-gpu-0501 2026-10-15T08:58:03.5Z",
			want: hb("vl-gpu-0501", time.Date(2026, 10, 15, 8, 58, 3, 500_000_000, time.UTC)),
		},
		{
			name: "nanoseconds",
			line: "vl-gpu-0311 2026-10-15T08:29:59.999999999Z",
			want: hb("vl-gpu-0311", time.Date(2026, 10, 15, 8, 29, 59, 999_999_999, time.UTC)),
		},
		{
			name: "tab between the fields",
			line: "harbour-01\t2026-10-15T08:30:00Z",
			want: hb("harbour-01", time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC)),
		},
		{
			name: "whitespace around and between the fields",
			line: "  harbour-01   2026-10-15T08:30:00Z \r",
			want: hb("harbour-01", time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rollcall.ParseHeartbeat(tc.line)
			if err != nil {
				t.Fatalf("ParseHeartbeat(%q) error = %v, want nil", tc.line, err)
			}
			// Equal compares instants: 10:31:12+02:00 is 08:31:12 UTC.
			if got.Node != tc.want.Node || !got.At.Equal(tc.want.At) {
				t.Errorf("ParseHeartbeat(%q)\n got  {%q %s}\n want {%q %s} (or the same instant in another zone)",
					tc.line, got.Node, got.At.Format(time.RFC3339Nano), tc.want.Node, tc.want.At.Format(time.RFC3339Nano))
			}
		})
	}
}

func TestParseHeartbeatMalformed(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"empty line", ""},
		{"whitespace only", " \t "},
		{"1 field: no time", "vl-gpu-0033"},
		{"1 field: no node", "2026-10-15T08:55:00Z"},
		{"3 fields: date and time apart", "vl-gpu-0101 2026-10-15 08:52:00Z"},
		{"3 fields: extra word", "canary-h3-01 2026-10-15T08:55:00Z ok"},
		{"no zone", "vl-gpu-0100 2026-10-15T08:51:00"},
		{"date only", "canary-h3-01 2026-10-15"},
		{"a word, not a time", "canary-h3-01 yesterday"},
		{"Unix seconds", "vl-gpu-0102 1760518320"},
		{"month 13", "canary-h3-01 2026-13-15T08:55:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rollcall.ParseHeartbeat(tc.line)
			if err == nil {
				t.Fatalf("ParseHeartbeat(%q) = {%q %s}, nil; want an error wrapping ErrMalformed",
					tc.line, got.Node, got.At.Format(time.RFC3339Nano))
			}
			if !errors.Is(err, rollcall.ErrMalformed) {
				t.Errorf("ParseHeartbeat(%q) error = %v, want it to wrap ErrMalformed (errors.Is)", tc.line, err)
			}
			if quoted := fmt.Sprintf("%q", tc.line); !strings.Contains(err.Error(), quoted) {
				t.Errorf("ParseHeartbeat(%q) error = %q, want it to contain the line as %s", tc.line, err, quoted)
			}
			if got != (rollcall.Heartbeat{}) {
				t.Errorf("ParseHeartbeat(%q) = {%q %s} with an error, want the zero Heartbeat",
					tc.line, got.Node, got.At.Format(time.RFC3339Nano))
			}
		})
	}
}
