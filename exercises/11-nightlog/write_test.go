package nightlog_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	nightlog "github.com/yornik/go-practice/exercises/11-nightlog"
)

// ---- Step 5: the report ----

// fullSummary is a night of 9213 lines with 1 gap. Its levels line is the
// one in the README rules for Write.
var fullSummary = nightlog.Summary{
	Lines:     9213,
	Malformed: 2,
	Levels:    map[string]int{"WARN": 207, "INFO": 9001, "ERROR": 3},
	First:     at(0, 0, 1),
	Last:      at(6, 59, 58),
	Gaps:      []nightlog.Gap{{From: at(3, 12, 9), To: at(3, 59, 41)}},
}

const fullReport = `lines:     9213
malformed: 2
first:     2026-10-16T00:00:01Z
last:      2026-10-16T06:59:58Z
levels:    ERROR=3 INFO=9001 WARN=207
gap:       2026-10-16T03:12:09Z to 2026-10-16T03:59:41Z (47m32s)
`

func TestWriteReport(t *testing.T) {
	cases := []struct {
		name string
		s    nightlog.Summary
		want string
	}{
		{"full report", fullSummary, fullReport},
		{
			"zero summary",
			nightlog.Summary{},
			"lines:     0\nmalformed: 0\nfirst:     -\nlast:      -\nlevels:    -\n",
		},
		{
			"empty levels map",
			nightlog.Summary{Lines: 2, Malformed: 1, Levels: map[string]int{}},
			"lines:     2\nmalformed: 1\nfirst:     -\nlast:      -\nlevels:    -\n",
		},
		{
			"levels A to Z, no gaps",
			nightlog.Summary{
				Lines:  21,
				Levels: map[string]int{"WARN": 2, "NOTICE": 1, "INFO": 10, "ERROR": 4, "DEBUG": 1, "GET": 3},
				First:  at(0, 0, 1),
				Last:   at(0, 0, 1),
			},
			"lines:     21\nmalformed: 0\nfirst:     2026-10-16T00:00:01Z\nlast:      2026-10-16T00:00:01Z\n" +
				"levels:    DEBUG=1 ERROR=4 GET=3 INFO=10 NOTICE=1 WARN=2\n",
		},
		{
			"several gaps in slice order",
			nightlog.Summary{
				Lines:  4,
				Levels: map[string]int{"INFO": 4},
				First:  at(0, 0, 0),
				Last:   at(2, 0, 5),
				Gaps: []nightlog.Gap{
					{From: at(0, 0, 0), To: at(0, 10, 1)},
					{From: at(0, 10, 1), To: at(2, 0, 5)},
				},
			},
			"lines:     4\nmalformed: 0\nfirst:     2026-10-16T00:00:00Z\nlast:      2026-10-16T02:00:05Z\n" +
				"levels:    INFO=4\n" +
				"gap:       2026-10-16T00:00:00Z to 2026-10-16T00:10:01Z (10m1s)\n" +
				"gap:       2026-10-16T00:10:01Z to 2026-10-16T02:00:05Z (1h50m4s)\n",
		},
		{
			"times keep their own zone",
			nightlog.Summary{
				Lines:  2,
				Levels: map[string]int{"INFO": 2},
				First:  at(3, 12, 9),
				Last:   time.Date(2026, 10, 16, 5, 59, 41, 0, plus2),
				Gaps:   []nightlog.Gap{{From: at(3, 12, 9), To: time.Date(2026, 10, 16, 5, 59, 41, 0, plus2)}},
			},
			"lines:     2\nmalformed: 0\nfirst:     2026-10-16T03:12:09Z\nlast:      2026-10-16T05:59:41+02:00\n" +
				"levels:    INFO=2\n" +
				"gap:       2026-10-16T03:12:09Z to 2026-10-16T05:59:41+02:00 (47m32s)\n",
		},
		{
			"fractional seconds are not shown",
			nightlog.Summary{
				Lines:  1,
				Levels: map[string]int{"INFO": 1},
				First:  at(0, 0, 1).Add(250 * time.Millisecond),
				Last:   at(0, 0, 1).Add(250 * time.Millisecond),
			},
			"lines:     1\nmalformed: 0\nfirst:     2026-10-16T00:00:01Z\nlast:      2026-10-16T00:00:01Z\n" +
				"levels:    INFO=1\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			if err := write(t, &out, tc.s); err != nil {
				t.Fatalf("Write(%s) error = %v, want nil", describe(tc.s), err)
			}
			if got := out.String(); got != tc.want {
				t.Errorf("Write of\n%s\nwrote\n%s\nwant\n%s\n(as Go strings: %q\nwant %q)", describe(tc.s), got, tc.want, got, tc.want)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	errFull := errors.New("write /var/log/report.txt: no space left on device")
	// Every room smaller than the report makes some write fail, however
	// Write splits the report into calls.
	for room := range len(fullReport) {
		w := &failWriter{err: errFull, room: room}
		err := write(t, w, fullSummary)
		if !errors.Is(err, errFull) {
			t.Fatalf("Write to a writer that accepts %d bytes: error = %v, want an error that wraps %q (errors.Is)", room, err, errFull)
		}
		if w.failed != 1 {
			t.Fatalf("Write to a writer that accepts %d bytes called Write %d times after the first error, want 0", room, w.failed-1)
		}
		if got := w.buf.String(); !strings.HasPrefix(fullReport, got) {
			t.Fatalf("Write to a writer that accepts %d bytes wrote %q, want the start of the report %q", room, got, fullReport)
		}
	}
}
