package loglevels_test

import (
	"strings"
	"testing"

	loglevels "github.com/yornik/go-practice/exercises/01-loglevels"
)

type levelCase struct {
	line string
	want string
}

func runLevelCases(t *testing.T, cases []levelCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			if got := loglevels.Level(tc.line); got != tc.want {
				t.Errorf("Level(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// spellings maps every known spelling to its normalized level, as the
// README's table lists them.
var spellings = map[string]string{
	"debug": "debug", "dbg": "debug", "trace": "debug",
	"info": "info", "information": "info", "notice": "info",
	"warn": "warn", "warning": "warn",
	"error": "error", "err": "error",
	"fatal": "fatal", "crit": "fatal", "critical": "fatal", "panic": "fatal", "emerg": "fatal",
}

// title returns s with its first letter in upper case: "warning" becomes
// "Warning". The spellings are ASCII, so a byte is a letter.
func title(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

func TestLevelSpellings(t *testing.T) {
	if len(spellings) != 15 {
		t.Fatalf("the test table has %d spellings, want 15", len(spellings))
	}
	var cases []levelCase
	for spelling, want := range spellings {
		for _, value := range []string{spelling, strings.ToUpper(spelling), title(spelling)} {
			cases = append(cases, levelCase{"service=upload level=" + value + " msg=x", want})
		}
	}
	runLevelCases(t, cases)
}

func TestLevelKeys(t *testing.T) {
	runLevelCases(t, []levelCase{
		{"level=error", "error"},
		{"lvl=error", "error"},
		{"severity=error", "error"},
		{"msg=x lvl=WARNING feeder=feeder-0412", "warn"},
		{"severity=Critical service=labeller", "fatal"},
		{"ts=2026-09-01T10:00:00Z level=eRr", "error"},
		// The last occurrence of a key wins, as in Fields.
		{"level=info level=error", "error"},
	})
}

func TestLevelKeyOrder(t *testing.T) {
	runLevelCases(t, []levelCase{
		// level comes before lvl, and lvl before severity, wherever they
		// stand in the line.
		{"lvl=err level=info", "info"},
		{"severity=critical lvl=dbg", "debug"},
		{"severity=critical lvl=dbg level=warn", "warn"},
		// The first key that is present decides, even when its value is
		// unknown or empty.
		{"level=bogus lvl=error", "unknown"},
		{"lvl=verbose severity=critical", "unknown"},
		{"level= lvl=error", "unknown"},
		{"level lvl=error", "unknown"},
		// Keys are case-sensitive: LEVEL and Level are other keys.
		{"LEVEL=error", "unknown"},
		{"Level=error", "unknown"},
		{"LEVEL=error lvl=warn", "warn"},
	})
}

func TestLevelUnknown(t *testing.T) {
	runLevelCases(t, []levelCase{
		{"ts=2026-09-01T10:00:16Z feeder=feeder-1203 msg=boot", "unknown"},
		{"", "unknown"},
		{"   ", "unknown"},
		{"level=", "unknown"},
		{"level", "unknown"},
		{"level=errors", "unknown"},
		{"level=3", "unknown"},
		{"level=verbose", "unknown"},
		{"level=unknown", "unknown"},
		{"level=warn-ish", "unknown"},
		{"=error", "unknown"},
		{"loglevel=error", "unknown"},
	})
}
