package nightlog_test

import (
	"os"
	"strings"
	"testing"
	"time"

	nightlog "github.com/yornik/go-practice/exercises/11-nightlog"
)

// ---- Step 6: a real night ----

// night summarises testdata/canary-h3-01.log with the given minGap.
func night(t *testing.T, minGap time.Duration) nightlog.Summary {
	t.Helper()
	f, err := os.Open("testdata/canary-h3-01.log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := summarize(t, f, minGap)
	if err != nil {
		t.Fatalf("Summarize(testdata/canary-h3-01.log, %v) error = %v, want nil", minGap, err)
	}
	return s
}

func TestNightReport(t *testing.T) {
	want, err := os.ReadFile("testdata/canary-h3-01.report")
	if err != nil {
		t.Fatal(err)
	}
	s := night(t, 10*time.Minute)
	var out strings.Builder
	if err := write(t, &out, s); err != nil {
		t.Fatalf("Write error = %v, want nil", err)
	}
	if got := out.String(); got != string(want) {
		t.Errorf("the report for testdata/canary-h3-01.log with a minimum gap of 10m is\n%s\nwant (testdata/canary-h3-01.report)\n%s", got, want)
	}
}

func TestNightShorterGap(t *testing.T) {
	// The watchdog reboot at 01:41 is a silence of 8m45s: too short for
	// 10m, long enough for 5m.
	s := night(t, 5*time.Minute)
	want := []nightlog.Gap{
		{From: at(1, 41, 7), To: at(1, 49, 52)},
		{From: at(3, 12, 9), To: at(3, 59, 41)},
	}
	got := nightlog.Summary{Gaps: s.Gaps}
	if g, w := describe(got), describe(nightlog.Summary{Gaps: want}); g != w {
		t.Errorf("Summarize(testdata/canary-h3-01.log, 5m) gaps:\n%s\nwant\n%s", g, w)
	}
}
