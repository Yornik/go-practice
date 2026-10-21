package repeated_test

import (
	"fmt"
	"slices"
	"testing"

	repeated "github.com/yornik/go-practice/exercises/02-repeated"
)

func TestCountMessages(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  []repeated.Count
	}{
		{
			"1 line",
			[]string{"00:01 a"},
			[]repeated.Count{{"a", 1}},
		},
		{
			"order of first appearance",
			[]string{"00:01 b", "00:02 a", "00:03 c"},
			[]repeated.Count{{"b", 1}, {"a", 1}, {"c", 1}},
		},
		{
			"consecutive lines are counted",
			[]string{"00:01 a", "00:02 a", "00:03 a"},
			[]repeated.Count{{"a", 3}},
		},
		{
			"lines that are not consecutive are counted too",
			[]string{"00:01 b", "00:02 a", "00:03 b", "00:04 c", "00:05 a", "00:06 b"},
			[]repeated.Count{{"b", 3}, {"a", 2}, {"c", 1}},
		},
		{
			"the timestamp does not matter",
			[]string{"00:01 " + motion, "00:59 " + motion, "07:28 " + motion},
			[]repeated.Count{{motion, 3}},
		},
		{
			"a line without a timestamp counts as a whole",
			[]string{"booting", "00:01 a", "booting"},
			[]repeated.Count{{"booting", 2}, {"a", 1}},
		},
		{
			"empty lines have the empty message",
			[]string{"", "00:01 a", "", "00:02 "},
			[]repeated.Count{{"", 3}, {"a", 1}},
		},
		{
			"letter case makes a different message",
			[]string{"00:01 a", "00:02 A", "00:03 a"},
			[]repeated.Count{{"a", 2}, {"A", 1}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := repeated.CountMessages(tc.lines)
			if !slices.Equal(got, tc.want) {
				t.Errorf("CountMessages(%s)\n got  %+v\n want %+v", formatLines(tc.lines), got, tc.want)
			}
		})
	}
}

func TestCountMessagesEmpty(t *testing.T) {
	for _, lines := range [][]string{nil, {}} {
		if got := repeated.CountMessages(lines); len(got) != 0 {
			t.Errorf("CountMessages(%s) = %+v, want an empty result", formatLines(lines), got)
		}
	}
}

// TestCountMessagesNight counts the messages in the 9000 lines that
// hq-window-01 wrote in 1 night.
func TestCountMessagesNight(t *testing.T) {
	lines := readLines(t, "testdata/hq-window-01.log")
	got := repeated.CountMessages(lines)

	if len(got) != 40 {
		t.Errorf("CountMessages(testdata/hq-window-01.log) returned %d counts, want 40 (1 per distinct message)", len(got))
	}
	total := 0
	for _, c := range got {
		total += c.N
	}
	if total != len(lines) {
		t.Errorf("CountMessages(testdata/hq-window-01.log): the counts add up to %d, want %d (1 per line)", total, len(lines))
	}
	wantFirst := []repeated.Count{
		{"camd: started, firmware 2.9.3", 3},
		{motion, 8600},
		{"uploadd: queue full, photo dropped", 344},
		{"wifid: signal -64 dBm", 4},
		{"seedd: seed level 62%", 1},
	}
	if first := got[:min(len(got), len(wantFirst))]; !slices.Equal(first, wantFirst) {
		t.Errorf("CountMessages(testdata/hq-window-01.log), the first %d counts:\n got  %+v\n want %+v", len(wantFirst), first, wantFirst)
	}
}

func ExampleCountMessages() {
	lines := []string{
		"2026-09-09T03:01:23Z motiond: motion detected",
		"2026-09-09T03:01:24Z motiond: motion detected",
		"2026-09-09T03:01:26Z wifid: connection lost",
		"2026-09-09T03:01:28Z motiond: motion detected",
		"2026-09-09T03:01:31Z wifid: reconnected after 41 s",
	}
	for _, c := range repeated.CountMessages(lines) {
		fmt.Printf("%4d %s\n", c.N, c.Message)
	}
	// Output:
	//    3 motiond: motion detected
	//    1 wifid: connection lost
	//    1 wifid: reconnected after 41 s
}
