package nightlog_test

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	nightlog "github.com/yornik/go-practice/exercises/11-nightlog"
)

type summarizeCase struct {
	name   string
	input  string
	minGap time.Duration
	want   nightlog.Summary
}

// runSummarizeCases checks that Summarize returns exactly the wanted summary
// and a nil error for every input.
func runSummarizeCases(t *testing.T, cases []summarizeCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call := fmt.Sprintf("Summarize(%q, %v)", tc.input, tc.minGap)
			got, err := summarize(t, strings.NewReader(tc.input), tc.minGap)
			if err != nil {
				t.Fatalf("%s error = %v, want nil", call, err)
			}
			checkSummary(t, call, got, tc.want)
		})
	}
}

// ---- Step 1: lines, malformed lines, levels, first and last ----

func TestSummarizeCounts(t *testing.T) {
	runSummarizeCases(t, []summarizeCase{
		{
			name:  "one valid line",
			input: text("2026-10-16T00:00:01Z INFO heartbeat gpu_temp_c=62"),
			want: nightlog.Summary{
				Lines: 1, Levels: map[string]int{"INFO": 1},
				First: at(0, 0, 1), Last: at(0, 0, 1),
			},
		},
		{
			name: "levels are upper-cased",
			input: text(
				"2026-10-16T00:00:01Z warn fan slow",
				"2026-10-16T00:00:02Z Warn fan slow",
				"2026-10-16T00:00:03Z WARN fan slow",
				"2026-10-16T00:00:04Z info heartbeat",
			),
			want: nightlog.Summary{
				Lines: 4, Levels: map[string]int{"WARN": 3, "INFO": 1},
				First: at(0, 0, 1), Last: at(0, 0, 4),
			},
		},
		{
			name: "any second field is a level",
			input: text(
				"2026-10-16T00:00:01Z notice gpud started",
				"2026-10-16T00:00:02Z GET /status 200",
			),
			want: nightlog.Summary{
				Lines: 2, Levels: map[string]int{"NOTICE": 1, "GET": 1},
				First: at(0, 0, 1), Last: at(0, 0, 2),
			},
		},
		{
			name:  "the message is optional",
			input: text("2026-10-16T00:00:01Z INFO"),
			want: nightlog.Summary{
				Lines: 1, Levels: map[string]int{"INFO": 1},
				First: at(0, 0, 1), Last: at(0, 0, 1),
			},
		},
		{
			name: "blank lines count only in Lines",
			input: text(
				"",
				"2026-10-16T00:00:01Z INFO a",
				"   ",
				"\t",
				"2026-10-16T00:00:02Z INFO b",
			),
			want: nightlog.Summary{
				Lines: 5, Levels: map[string]int{"INFO": 2},
				First: at(0, 0, 1), Last: at(0, 0, 2),
			},
		},
		{
			name: "malformed lines",
			input: text(
				"2026-10-16T00:00:01Z",                        // only 1 field
				"yesterday INFO fan controller failed",        // not a time
				"2026-10-16T00:00:02 INFO no zone",            // RFC 3339 needs a zone
				"2026-10-16 00:00:03 INFO space instead of T", // the first field is only the date
				"INFO 2026-10-16T00:00:04Z level first",       // the time is not the first field
				"U-Boot 2024.01 (Sep 30 2026 - 11:20:07 +0000)",
				"2026-10-16T03:12:",
				"2026-10-16T00:00:05Z INFO ok",
			),
			want: nightlog.Summary{
				Lines: 8, Malformed: 7, Levels: map[string]int{"INFO": 1},
				First: at(0, 0, 5), Last: at(0, 0, 5),
			},
		},
		{
			name:  "only malformed lines",
			input: text("U-Boot 2024.01", "booting"),
			want:  nightlog.Summary{Lines: 2, Malformed: 2, Levels: map[string]int{}},
		},
		{
			name:  "no newline after the last line",
			input: "2026-10-16T00:00:01Z INFO a\n2026-10-16T00:00:02Z ERROR b",
			want: nightlog.Summary{
				Lines: 2, Levels: map[string]int{"INFO": 1, "ERROR": 1},
				First: at(0, 0, 1), Last: at(0, 0, 2),
			},
		},
		{
			name:  "empty input",
			input: "",
			want:  nightlog.Summary{Levels: map[string]int{}},
		},
		{
			name:  "a single newline is 1 blank line",
			input: "\n",
			want:  nightlog.Summary{Lines: 1, Levels: map[string]int{}},
		},
		{
			name: "first and last are in input order",
			input: text(
				"2026-10-16T03:00:00Z INFO a",
				"2026-10-16T01:00:00Z INFO b",
				"2026-10-16T02:00:00Z INFO c",
			),
			want: nightlog.Summary{
				Lines: 3, Levels: map[string]int{"INFO": 3},
				First: at(3, 0, 0), Last: at(2, 0, 0),
			},
		},
		{
			name:  "the offset of a line is kept",
			input: text("2026-10-16T05:12:09+02:00 INFO a"),
			want: nightlog.Summary{
				Lines: 1, Levels: map[string]int{"INFO": 1},
				First: time.Date(2026, 10, 16, 5, 12, 9, 0, plus2),
				Last:  time.Date(2026, 10, 16, 5, 12, 9, 0, plus2),
			},
		},
		{
			name:  "fractional seconds",
			input: text("2026-10-16T00:00:01.250Z INFO a"),
			want: nightlog.Summary{
				Lines: 1, Levels: map[string]int{"INFO": 1},
				First: at(0, 0, 1).Add(250 * time.Millisecond),
				Last:  at(0, 0, 1).Add(250 * time.Millisecond),
			},
		},
		{
			name:  "whitespace around the fields",
			input: text("  2026-10-16T00:00:01Z \t DEBUG   gpud   "),
			want: nightlog.Summary{
				Lines: 1, Levels: map[string]int{"DEBUG": 1},
				First: at(0, 0, 1), Last: at(0, 0, 1),
			},
		},
		{
			name:  "Windows line endings",
			input: "2026-10-16T00:00:01Z INFO a\r\n2026-10-16T00:00:02Z WARN\r\n\r\n",
			want: nightlog.Summary{
				Lines: 3, Levels: map[string]int{"INFO": 1, "WARN": 1},
				First: at(0, 0, 1), Last: at(0, 0, 2),
			},
		},
	})
}

// ---- Step 2: gaps ----

func TestSummarizeGaps(t *testing.T) {
	const minGap = 10 * time.Minute
	several := text(
		"2026-10-16T00:00:00Z INFO a",
		"2026-10-16T00:20:00Z INFO b",
		"2026-10-16T00:21:00Z INFO c",
		"2026-10-16T01:00:00Z INFO d",
	)
	severalSummary := func(gaps ...nightlog.Gap) nightlog.Summary {
		return nightlog.Summary{
			Lines: 4, Levels: map[string]int{"INFO": 4},
			First: at(0, 0, 0), Last: at(1, 0, 0), Gaps: gaps,
		}
	}
	runSummarizeCases(t, []summarizeCase{
		{
			name:   "exactly minGap is not a gap",
			input:  text("2026-10-16T00:00:00Z INFO a", "2026-10-16T00:10:00Z INFO b"),
			minGap: minGap,
			want: nightlog.Summary{
				Lines: 2, Levels: map[string]int{"INFO": 2},
				First: at(0, 0, 0), Last: at(0, 10, 0),
			},
		},
		{
			name:   "1 s more than minGap is a gap",
			input:  text("2026-10-16T00:00:00Z INFO a", "2026-10-16T00:10:01Z INFO b"),
			minGap: minGap,
			want: nightlog.Summary{
				Lines: 2, Levels: map[string]int{"INFO": 2},
				First: at(0, 0, 0), Last: at(0, 10, 1),
				Gaps: []nightlog.Gap{{From: at(0, 0, 0), To: at(0, 10, 1)}},
			},
		},
		{
			name:   "a backwards jump is not a gap",
			input:  text("2026-10-16T03:00:00Z INFO a", "2026-10-16T01:00:00Z INFO b"),
			minGap: minGap,
			want: nightlog.Summary{
				Lines: 2, Levels: map[string]int{"INFO": 2},
				First: at(3, 0, 0), Last: at(1, 0, 0),
			},
		},
		{
			name: "after a backwards jump the next line is compared with the line before it",
			input: text(
				"2026-10-16T03:00:00Z INFO a",
				"2026-10-16T01:00:00Z INFO b",
				"2026-10-16T01:30:00Z INFO c",
			),
			minGap: minGap,
			want: nightlog.Summary{
				Lines: 3, Levels: map[string]int{"INFO": 3},
				First: at(3, 0, 0), Last: at(1, 30, 0),
				Gaps: []nightlog.Gap{{From: at(1, 0, 0), To: at(1, 30, 0)}},
			},
		},
		{
			name: "blank and malformed lines do not end a silence",
			input: text(
				"2026-10-16T00:00:00Z INFO a",
				"",
				"U-Boot 2024.01",
				"2026-10-16T00:05:00Z", // malformed: its time does not count
				"2026-10-16T00:30:00Z INFO b",
			),
			minGap: minGap,
			want: nightlog.Summary{
				Lines: 5, Malformed: 2, Levels: map[string]int{"INFO": 2},
				First: at(0, 0, 0), Last: at(0, 30, 0),
				Gaps: []nightlog.Gap{{From: at(0, 0, 0), To: at(0, 30, 0)}},
			},
		},
		{
			name:   "several gaps in input order",
			input:  several,
			minGap: minGap,
			want: severalSummary(
				nightlog.Gap{From: at(0, 0, 0), To: at(0, 20, 0)},
				nightlog.Gap{From: at(0, 21, 0), To: at(1, 0, 0)},
			),
		},
		{
			name:   "minGap 0 reports no gaps",
			input:  several,
			minGap: 0,
			want:   severalSummary(),
		},
		{
			name:   "a negative minGap reports no gaps",
			input:  several,
			minGap: -time.Minute,
			want:   severalSummary(),
		},
		{
			name: "equal times are not a gap",
			input: text(
				"2026-10-16T00:00:00Z INFO a",
				"2026-10-16T00:00:00Z INFO b",
				"2026-10-16T00:00:01Z INFO c",
			),
			minGap: time.Nanosecond,
			want: nightlog.Summary{
				Lines: 3, Levels: map[string]int{"INFO": 3},
				First: at(0, 0, 0), Last: at(0, 0, 1),
				Gaps: []nightlog.Gap{{From: at(0, 0, 0), To: at(0, 0, 1)}},
			},
		},
		{
			name: "instants count, not wall clocks: 47m32s across zones",
			input: text(
				"2026-10-16T03:12:09Z INFO a",
				"2026-10-16T05:59:41+02:00 INFO b",
			),
			minGap: minGap,
			want: nightlog.Summary{
				Lines: 2, Levels: map[string]int{"INFO": 2},
				First: at(3, 12, 9), Last: time.Date(2026, 10, 16, 5, 59, 41, 0, plus2),
				Gaps: []nightlog.Gap{{From: at(3, 12, 9), To: time.Date(2026, 10, 16, 5, 59, 41, 0, plus2)}},
			},
		},
		{
			name: "instants count, not wall clocks: 7m51s across zones",
			input: text(
				"2026-10-16T03:12:09Z INFO a",
				"2026-10-16T05:20:00+02:00 INFO b",
			),
			minGap: minGap,
			want: nightlog.Summary{
				Lines: 2, Levels: map[string]int{"INFO": 2},
				First: at(3, 12, 9), Last: time.Date(2026, 10, 16, 5, 20, 0, 0, plus2),
			},
		},
	})
}

// ---- Step 3: any io.Reader ----

func TestSummarizeReaders(t *testing.T) {
	cases := []summarizeCase{
		{
			name: "mixed lines",
			input: text(
				"2026-10-16T00:00:01Z INFO heartbeat",
				"",
				"U-Boot 2024.01",
				"2026-10-16T00:00:02Z warn fan controller slow",
			),
			want: nightlog.Summary{
				Lines: 4, Malformed: 1, Levels: map[string]int{"INFO": 1, "WARN": 1},
				First: at(0, 0, 1), Last: at(0, 0, 2),
			},
		},
		{
			name:  "no newline after the last line",
			input: "2026-10-16T00:00:01Z INFO a\n2026-10-16T00:00:02Z ERROR b",
			want: nightlog.Summary{
				Lines: 2, Levels: map[string]int{"INFO": 1, "ERROR": 1},
				First: at(0, 0, 1), Last: at(0, 0, 2),
			},
		},
		{
			name:  "Windows line endings",
			input: "2026-10-16T00:00:01Z INFO a\r\n\r\n2026-10-16T00:00:02Z WARN b\r\n",
			want: nightlog.Summary{
				Lines: 3, Levels: map[string]int{"INFO": 1, "WARN": 1},
				First: at(0, 0, 1), Last: at(0, 0, 2),
			},
		},
		{
			name: "a gap",
			input: text(
				"2026-10-16T03:12:09Z INFO upload started",
				"2026-10-16T03:12:",
				"2026-10-16T03:59:41Z INFO boot reason=power-on",
			),
			minGap: 10 * time.Minute,
			want: nightlog.Summary{
				Lines: 3, Malformed: 1, Levels: map[string]int{"INFO": 2},
				First: at(3, 12, 9), Last: at(3, 59, 41),
				Gaps: []nightlog.Gap{{From: at(3, 12, 9), To: at(3, 59, 41)}},
			},
		},
		{
			name:  "empty input",
			input: "",
			want:  nightlog.Summary{Levels: map[string]int{}},
		},
	}
	for _, rd := range readers {
		for _, tc := range cases {
			t.Run(rd.name+"/"+tc.name, func(t *testing.T) {
				call := fmt.Sprintf("Summarize(%s(%q), %v)", rd.name, tc.input, tc.minGap)
				got, err := summarize(t, rd.wrap(strings.NewReader(tc.input)), tc.minGap)
				if err != nil {
					t.Fatalf("%s error = %v, want nil", call, err)
				}
				checkSummary(t, call, got, tc.want)
			})
		}
	}
}

// ---- Step 4: read errors and long lines ----

func TestSummarizeErrorsRead(t *testing.T) {
	errDisk := errors.New("read /var/log/gpud.log: input/output error")
	input := text(
		"2026-10-16T00:00:00Z INFO boot",
		"2026-10-16T00:20:00Z warn fan slow",
		"not a log line",
	)
	partial := nightlog.Summary{
		Lines: 3, Malformed: 1, Levels: map[string]int{"INFO": 1, "WARN": 1},
		First: at(0, 0, 0), Last: at(0, 20, 0),
		Gaps: []nightlog.Gap{{From: at(0, 0, 0), To: at(0, 20, 0)}},
	}
	cases := []struct {
		name string
		r    *errAfter
		want nightlog.Summary
	}{
		{"error after 3 lines", &errAfter{data: input, err: errDisk}, partial},
		{"error in the same Read as the last data", &errAfter{data: input, err: errDisk, withLast: true}, partial},
		{"error before any data", &errAfter{err: errDisk}, nightlog.Summary{Levels: map[string]int{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call := fmt.Sprintf("Summarize(a reader that returns %q and then an error, 10m)", tc.r.data)
			got, err := summarize(t, tc.r, 10*time.Minute)
			if !errors.Is(err, errDisk) {
				t.Fatalf("%s error = %v, want an error that wraps %q (errors.Is)", call, err, errDisk)
			}
			checkSummary(t, call, got, tc.want)
		})
	}
}

func TestSummarizeErrorsLongLine(t *testing.T) {
	long := "2026-10-16T00:00:01Z INFO body=" + strings.Repeat("x", 70<<10)
	input := text("2026-10-16T00:00:00Z INFO before", long, "2026-10-16T00:00:02Z INFO after")
	call := "Summarize(a log with a 70 KiB line 2, 0)"
	got, err := summarize(t, strings.NewReader(input), 0)
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("%s error = %v, want an error that wraps bufio.ErrTooLong (errors.Is)", call, err)
	}
	checkSummary(t, call, got, nightlog.Summary{
		Lines: 1, Levels: map[string]int{"INFO": 1},
		First: at(0, 0, 0), Last: at(0, 0, 0),
	})
}
