package logfmt_test

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	logfmt "github.com/yornik/go-practice/exercises/26-logfmt"
)

// The tests in this file are named TestLevel... and TestRecord... (not
// TestParseLevel, TestParseRecord) so that each step has its own -run filter:
// -run TestParse selects only the Parse tests, -run TestLevel the Level tests,
// -run TestRecord the ParseRecord tests.

func TestLevelString(t *testing.T) {
	tests := []struct {
		level logfmt.Level
		want  string
	}{
		{logfmt.Debug, "debug"},
		{logfmt.Info, "info"},
		{logfmt.Warn, "warn"},
		{logfmt.Error, "error"},
		{logfmt.Level(-1), "Level(-1)"},
		{logfmt.Level(4), "Level(4)"},
		{logfmt.Level(42), "Level(42)"},
	}
	for _, tc := range tests {
		if got := tc.level.String(); got != tc.want {
			t.Errorf("Level(%d).String() = %q, want %q", int(tc.level), got, tc.want)
		}
	}

	// fmt calls String for %v and %s, so a Level prints as a word.
	if got := fmt.Sprintf("%v|%s", logfmt.Warn, logfmt.Error); got != "warn|error" {
		t.Errorf(`fmt.Sprintf("%%v|%%s", Warn, Error) = %q, want "warn|error"`, got)
	}

	// String and ParseLevel are inverses for the known levels.
	for _, l := range []logfmt.Level{logfmt.Debug, logfmt.Info, logfmt.Warn, logfmt.Error} {
		got, err := logfmt.ParseLevel(l.String())
		if err != nil || got != l {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v, nil", l.String(), got, err, l)
		}
	}
}

func TestLevelParse(t *testing.T) {
	good := []struct {
		in   string
		want logfmt.Level
	}{
		{"debug", logfmt.Debug},
		{"DEBUG", logfmt.Debug},
		{"info", logfmt.Info},
		{"Info", logfmt.Info},
		{"INFO", logfmt.Info},
		{"warn", logfmt.Warn},
		{"WARN", logfmt.Warn},
		{"warning", logfmt.Warn},
		{"Warning", logfmt.Warn},
		{"WARNING", logfmt.Warn},
		{"error", logfmt.Error},
		{"eRRoR", logfmt.Error},
	}
	for _, tc := range good {
		got, err := logfmt.ParseLevel(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v, nil", tc.in, got, err, tc.want)
		}
	}

	// Exactly the five words: no prefixes, no abbreviations, no other levels.
	bad := []string{"", "fatal", "trace", "err", "warnings", "information", "debug2"}
	for _, in := range bad {
		_, err := logfmt.ParseLevel(in)
		if err == nil {
			t.Errorf("ParseLevel(%q) error = nil, want an error wrapping ErrUnknownLevel", in)
			continue
		}
		if !errors.Is(err, logfmt.ErrUnknownLevel) {
			t.Errorf("ParseLevel(%q) error = %v, want it to wrap ErrUnknownLevel (errors.Is)", in, err)
		}
		if quoted := fmt.Sprintf("%q", in); !strings.Contains(err.Error(), quoted) {
			t.Errorf("ParseLevel(%q) error = %q, want it to contain the input as %s", in, err, quoted)
		}
	}
}

func TestRecord(t *testing.T) {
	noon := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		line string
		want logfmt.Record
	}{
		{
			name: "service log line",
			line: `ts=2026-10-16T12:00:00Z level=error msg="db timeout" latency_ms=532 db=orders`,
			want: logfmt.Record{Time: noon, Level: logfmt.Error, Msg: "db timeout",
				Fields: map[string]string{"latency_ms": "532", "db": "orders"}},
		},
		{
			name: "keys in any order",
			line: `msg=started level=info port=8080 ts=2026-10-16T12:00:00Z`,
			want: logfmt.Record{Time: noon, Level: logfmt.Info, Msg: "started",
				Fields: map[string]string{"port": "8080"}},
		},
		{
			// 14:00:00.250 at +02:00 is the same instant as 12:00:00.250 UTC.
			name: "fractional seconds and offset, no msg, no other fields",
			line: `ts=2026-10-16T14:00:00.250+02:00 level=debug`,
			want: logfmt.Record{Time: noon.Add(250 * time.Millisecond), Level: logfmt.Debug, Msg: "",
				Fields: map[string]string{}},
		},
		{
			name: "upper-case level as log/slog writes it",
			line: `ts=2026-10-16T12:00:00Z level=WARN msg=slow`,
			want: logfmt.Record{Time: noon, Level: logfmt.Warn, Msg: "slow", Fields: map[string]string{}},
		},
		{
			name: "warning means warn",
			line: `ts=2026-10-16T12:00:00Z level=Warning`,
			want: logfmt.Record{Time: noon, Level: logfmt.Warn, Fields: map[string]string{}},
		},
		{
			name: "bare key goes to Fields with an empty value",
			line: `ts=2026-10-16T12:00:00Z level=info msg=retrying retry`,
			want: logfmt.Record{Time: noon, Level: logfmt.Info, Msg: "retrying",
				Fields: map[string]string{"retry": ""}},
		},
		{
			name: "last occurrence wins, also for ts, level and msg",
			line: `ts=2026-10-16T11:00:00Z level=info msg=a user=alice user=bob level=error msg=b ts=2026-10-16T12:00:00Z`,
			want: logfmt.Record{Time: noon, Level: logfmt.Error, Msg: "b",
				Fields: map[string]string{"user": "bob"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := logfmt.ParseRecord(tc.line)
			if err != nil {
				t.Fatalf("ParseRecord(%q) error = %v, want nil", tc.line, err)
			}
			// Compare instants with Equal: == also compares the time zone.
			if !got.Time.Equal(tc.want.Time) {
				t.Errorf("ParseRecord(%q).Time = %v, want %v", tc.line, got.Time, tc.want.Time)
			}
			if got.Level != tc.want.Level {
				t.Errorf("ParseRecord(%q).Level = %v, want %v", tc.line, got.Level, tc.want.Level)
			}
			if got.Msg != tc.want.Msg {
				t.Errorf("ParseRecord(%q).Msg = %q, want %q", tc.line, got.Msg, tc.want.Msg)
			}
			if got.Fields == nil {
				t.Errorf("ParseRecord(%q).Fields = nil, want a non-nil map (callers may add to it)", tc.line)
			}
			if !maps.Equal(got.Fields, tc.want.Fields) {
				t.Errorf("ParseRecord(%q).Fields = %v, want %v", tc.line, got.Fields, tc.want.Fields)
			}
		})
	}
}

func TestRecordSyntaxError(t *testing.T) {
	line := `ts=2026-10-16T12:00:00Z level=info msg="oops`
	_, err := logfmt.ParseRecord(line)
	var se *logfmt.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("ParseRecord(%q) error = %v (type %T), want one that errors.As finds a *logfmt.SyntaxError in", line, err, err)
	}
	if se.Col != 40 {
		t.Errorf("ParseRecord(%q) syntax error column = %d, want 40", line, se.Col)
	}
}

func TestRecordErrors(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		prefix   string // the error message must start with this
		is       error  // errors.Is(err, is) must be true (nil: not checked)
		isNot    error  // errors.Is(err, isNot) must be false (nil: not checked)
		asTime   bool   // errors.As must find a *time.ParseError
		contains string // the error message must contain this ("": not checked)
	}{
		{name: "missing ts", line: `level=info msg=x`, prefix: "ts: ", is: logfmt.ErrMissingField},
		{name: "missing level", line: `ts=2026-10-16T12:00:00Z msg=x`, prefix: "level: ", is: logfmt.ErrMissingField},
		{name: "both missing: ts is checked first", line: `msg=x`, prefix: "ts: ", is: logfmt.ErrMissingField},
		{name: "empty line", line: ``, prefix: "ts: ", is: logfmt.ErrMissingField},

		{name: "ts without a time", line: `ts=2026-10-16 level=info`, prefix: "ts: ", asTime: true, isNot: logfmt.ErrMissingField},
		{name: "ts as Unix seconds", line: `ts=1760616000 level=info`, prefix: "ts: ", asTime: true},
		// A key with an empty value is present, so this is a bad ts, not a missing one.
		{name: "empty ts is invalid, not missing", line: `ts= level=info`, prefix: "ts: ", asTime: true, isNot: logfmt.ErrMissingField},
		{name: "bad ts is reported before a missing level", line: `ts=yesterday msg=x`, prefix: "ts: ", asTime: true},
		// The order of the checks is fixed; the order of the keys in the line does not change it.
		{name: "bad ts is reported before a bad level that comes first in the line", line: `level=fatal ts=yesterday`, prefix: "ts: ", asTime: true},
		{name: "missing ts is reported before a bad level", line: `level=fatal msg=x`, prefix: "ts: ", is: logfmt.ErrMissingField},

		{name: "unknown level", line: `ts=2026-10-16T12:00:00Z level=fatal`, prefix: "level: ", is: logfmt.ErrUnknownLevel, contains: `"fatal"`},
		{name: "empty level is unknown, not missing", line: `ts=2026-10-16T12:00:00Z level=`, prefix: "level: ", is: logfmt.ErrUnknownLevel, isNot: logfmt.ErrMissingField},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := logfmt.ParseRecord(tc.line)
			if err == nil {
				t.Fatalf("ParseRecord(%q) error = nil, want an error starting with %q", tc.line, tc.prefix)
			}
			if !strings.HasPrefix(err.Error(), tc.prefix) {
				t.Errorf("ParseRecord(%q) error = %q, want it to start with %q", tc.line, err, tc.prefix)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("ParseRecord(%q) error = %q, want errors.Is(err, %q) to be true", tc.line, err, tc.is)
			}
			if tc.isNot != nil && errors.Is(err, tc.isNot) {
				t.Errorf("ParseRecord(%q) error = %q, want errors.Is(err, %q) to be false", tc.line, err, tc.isNot)
			}
			var pe *time.ParseError
			if tc.asTime && !errors.As(err, &pe) {
				t.Errorf("ParseRecord(%q) error = %q, want it to wrap the *time.ParseError (errors.As)", tc.line, err)
			}
			if tc.contains != "" && !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("ParseRecord(%q) error = %q, want it to contain %s", tc.line, err, tc.contains)
			}
		})
	}
}

// A tiny log filter: print warnings and errors, report broken lines.
func ExampleParseRecord() {
	lines := []string{
		`ts=2026-10-16T12:00:00Z level=info msg="request served" path=/healthz`,
		`ts=2026-10-16T12:00:01Z level=error msg="db timeout" latency_ms=532`,
		`ts=2026-10-16T12:00:02Z level=WARNING msg="slow query" latency_ms=210`,
		`ts=2026-10-16T12:00:03Z level=error msg="truncated`,
	}
	for i, line := range lines {
		rec, err := logfmt.ParseRecord(line)
		var se *logfmt.SyntaxError
		if errors.As(err, &se) {
			fmt.Printf("line %d: syntax error at column %d\n", i+1, se.Col)
			continue
		}
		if err != nil {
			fmt.Printf("line %d: %v\n", i+1, err)
			continue
		}
		if rec.Level >= logfmt.Warn {
			fmt.Println(rec.Time.Format(time.TimeOnly), rec.Level, rec.Msg, rec.Fields["latency_ms"])
		}
	}
	// Output:
	// 12:00:01 error db timeout 532
	// 12:00:02 warn slow query 210
	// line 4: syntax error at column 41
}
