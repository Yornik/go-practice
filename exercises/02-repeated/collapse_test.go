package repeated_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	repeated "github.com/yornik/go-practice/exercises/02-repeated"
)

const motion = "motiond: motion detected"

type collapseCase struct {
	name  string
	lines []string
	want  []string
}

// runCollapseCases compares the whole result. A nil and an empty slice count
// as equal.
func runCollapseCases(t *testing.T, cases []collapseCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := repeated.Collapse(tc.lines)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Collapse(%s)\n got  %s\n want %s", formatLines(tc.lines), formatLines(got), formatLines(tc.want))
			}
		})
	}
}

// formatLines prints 1 quoted line per row, so that spaces and empty lines
// are visible.
func formatLines(lines []string) string {
	if lines == nil {
		return "nil"
	}
	var b strings.Builder
	b.WriteString("[")
	for _, l := range lines {
		fmt.Fprintf(&b, "\n\t%q", l)
	}
	b.WriteString("\n]")
	return b.String()
}

func TestCollapseRuns(t *testing.T) {
	runCollapseCases(t, []collapseCase{
		{
			"no repeats",
			[]string{"00:01 a", "00:02 b", "00:03 c"},
			[]string{"00:01 a", "00:02 b", "00:03 c"},
		},
		{
			"1 line",
			[]string{"00:01 a"},
			[]string{"00:01 a"},
		},
		{
			"run of 2 says 1 time",
			[]string{"00:01 a", "00:02 a"},
			[]string{"00:01 a", "00:02 last message repeated 1 time"},
		},
		{
			"run of 3 says 2 times",
			[]string{"00:01 a", "00:02 a", "00:03 a", "00:04 b"},
			[]string{"00:01 a", "00:03 last message repeated 2 times", "00:04 b"},
		},
		{
			"run at the start",
			[]string{"00:01 a", "00:02 a", "00:03 a", "00:04 b", "00:05 c"},
			[]string{"00:01 a", "00:03 last message repeated 2 times", "00:04 b", "00:05 c"},
		},
		{
			"run in the middle",
			[]string{"00:01 b", "00:02 a", "00:03 a", "00:04 c"},
			[]string{"00:01 b", "00:02 a", "00:03 last message repeated 1 time", "00:04 c"},
		},
		{
			"run at the end",
			[]string{"00:01 b", "00:02 a", "00:03 a", "00:04 a"},
			[]string{"00:01 b", "00:02 a", "00:04 last message repeated 2 times"},
		},
		{
			"whole input is 1 run",
			[]string{"00:01 a", "00:02 a", "00:03 a", "00:04 a", "00:05 a"},
			[]string{"00:01 a", "00:05 last message repeated 4 times"},
		},
		{
			"2 runs separated by 1 line",
			[]string{"00:01 a", "00:02 a", "00:03 b", "00:04 a", "00:05 a", "00:06 a"},
			[]string{"00:01 a", "00:02 last message repeated 1 time", "00:03 b", "00:04 a", "00:06 last message repeated 2 times"},
		},
		{
			"2 different runs next to each other",
			[]string{"00:01 a", "00:02 a", "00:03 b", "00:04 b"},
			[]string{"00:01 a", "00:02 last message repeated 1 time", "00:03 b", "00:04 last message repeated 1 time"},
		},
		{
			"same message with the same timestamp",
			[]string{"00:01 a", "00:01 a", "00:01 a"},
			[]string{"00:01 a", "00:01 last message repeated 2 times"},
		},
		{
			"same timestamp, different messages",
			[]string{"00:01 a", "00:01 b", "00:01 c"},
			[]string{"00:01 a", "00:01 b", "00:01 c"},
		},
		{
			"a run of 10",
			[]string{"00:01 a", "00:02 a", "00:03 a", "00:04 a", "00:05 a", "00:06 a", "00:07 a", "00:08 a", "00:09 a", "00:10 a"},
			[]string{"00:01 a", "00:10 last message repeated 9 times"},
		},
	})
}

func TestCollapseEmpty(t *testing.T) {
	for _, lines := range [][]string{nil, {}} {
		if got := repeated.Collapse(lines); len(got) != 0 {
			t.Errorf("Collapse(%s) = %s, want an empty result", formatLines(lines), formatLines(got))
		}
	}
}

func TestCollapseMessages(t *testing.T) {
	runCollapseCases(t, []collapseCase{
		{
			"spaces inside the message",
			[]string{"00:01 " + motion, "00:02 " + motion, "00:03 " + motion},
			[]string{"00:01 " + motion, "00:03 last message repeated 2 times"},
		},
		{
			"letter case makes a different message",
			[]string{"00:01 Motion detected", "00:02 motion detected"},
			[]string{"00:01 Motion detected", "00:02 motion detected"},
		},
		{
			"a trailing space makes a different message",
			[]string{"00:01 a", "00:02 a "},
			[]string{"00:01 a", "00:02 a "},
		},
		{
			"lines are copied unchanged",
			[]string{"00:01 a  ", "00:02 a  ", "00:03 \tb"},
			[]string{"00:01 a  ", "00:02 last message repeated 1 time", "00:03 \tb"},
		},
		{
			"empty lines collapse",
			[]string{"00:01 a", "", "", "", "00:05 b"},
			[]string{"00:01 a", "", "last message repeated 2 times", "00:05 b"},
		},
		{
			"lines without a timestamp: marker without a timestamp",
			[]string{"booting", "booting", "booting"},
			[]string{"booting", "last message repeated 2 times"},
		},
		{
			"last line starts with a space: marker without a timestamp",
			[]string{" a", " a"},
			[]string{" a", "last message repeated 1 time"},
		},
		{
			"a marker in the input does not extend the run before it",
			[]string{"00:01 a", "00:02 a", "00:03 last message repeated 1 time"},
			[]string{"00:01 a", "00:02 last message repeated 1 time", "00:03 last message repeated 1 time"},
		},
		{
			"equal markers in the input form a run",
			[]string{"00:01 x", "00:02 last message repeated 1 time", "00:03 last message repeated 1 time", "00:04 last message repeated 1 time"},
			[]string{"00:01 x", "00:02 last message repeated 1 time", "00:04 last message repeated 2 times"},
		},
	})
}

func TestCollapseKeepsInput(t *testing.T) {
	lines := []string{"00:01 a", "00:02 a", "00:03 a", "00:04 b", "00:05 a", "00:06 a"}
	before := slices.Clone(lines)
	got := repeated.Collapse(lines)
	if !slices.Equal(lines, before) {
		t.Errorf("Collapse changed its input:\n before %s\n after  %s", formatLines(before), formatLines(lines))
	}
	want := []string{"00:01 a", "00:03 last message repeated 2 times", "00:04 b", "00:05 a", "00:06 last message repeated 1 time"}
	if !slices.Equal(got, want) {
		t.Errorf("Collapse(%s)\n got  %s\n want %s", formatLines(before), formatLines(got), formatLines(want))
	}
}

// readLines reads a file as lines. The newline at the end of the file ends
// the last line; it does not start an empty one.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// TestCollapseNight runs Collapse on the 9000 lines that hq-window-01 wrote
// in 1 night and compares the result with testdata/hq-window-01.golden.
func TestCollapseNight(t *testing.T) {
	lines := readLines(t, "testdata/hq-window-01.log")
	want := readLines(t, "testdata/hq-window-01.golden")

	got := repeated.Collapse(lines)
	for i := range min(len(got), len(want)) {
		if got[i] != want[i] {
			t.Fatalf("Collapse(testdata/hq-window-01.log): first difference at output line %d\n got  %q\n want %q", i+1, got[i], want[i])
		}
	}
	if len(got) != len(want) {
		var extra string
		if len(got) > len(want) {
			extra = fmt.Sprintf("first extra line: %q", got[len(want)])
		} else {
			extra = fmt.Sprintf("first missing line: %q", want[len(got)])
		}
		t.Fatalf("Collapse(testdata/hq-window-01.log) returned %d lines, want %d; %s", len(got), len(want), extra)
	}
}

func ExampleCollapse() {
	lines := []string{
		"2026-09-09T03:01:23Z motiond: motion detected",
		"2026-09-09T03:01:24Z motiond: motion detected",
		"2026-09-09T03:01:25Z motiond: motion detected",
		"2026-09-09T03:01:26Z wifid: connection lost",
		"2026-09-09T03:01:28Z motiond: motion detected",
		"2026-09-09T03:01:31Z wifid: reconnected after 41 s",
		"2026-09-09T03:01:34Z motiond: motion detected",
		"2026-09-09T03:01:36Z motiond: motion detected",
	}
	for _, line := range repeated.Collapse(lines) {
		fmt.Println(line)
	}
	// Output:
	// 2026-09-09T03:01:23Z motiond: motion detected
	// 2026-09-09T03:01:25Z last message repeated 2 times
	// 2026-09-09T03:01:26Z wifid: connection lost
	// 2026-09-09T03:01:28Z motiond: motion detected
	// 2026-09-09T03:01:31Z wifid: reconnected after 41 s
	// 2026-09-09T03:01:34Z motiond: motion detected
	// 2026-09-09T03:01:36Z last message repeated 1 time
}
