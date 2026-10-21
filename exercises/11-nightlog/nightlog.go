// Package nightlog summarises a night of log lines: how many lines there
// were, how many of each level, and where the log went silent.
package nightlog

import (
	"io"
	"time"
)

// Gap is a silence in the log: 2 consecutive valid lines whose times are
// further apart than the minimum gap.
type Gap struct {
	From time.Time // time of the valid line before the silence
	To   time.Time // time of the valid line after the silence
}

// Summary describes the lines that Summarize read.
type Summary struct {
	Lines     int            // every line read, blank lines too
	Malformed int            // lines that are not blank and not valid
	Levels    map[string]int // valid lines per level, in upper case; never nil
	First     time.Time      // time of the first valid line; zero if there is none
	Last      time.Time      // time of the last valid line; zero if there is none
	Gaps      []Gap          // silences longer than minGap, in input order
}

// Summarize reads r to the end and summarises its lines. A valid line has
// at least 2 fields separated by whitespace: an RFC 3339 time and a level,
// as in "2026-10-16T03:12:09Z INFO upload started". Blank lines count only
// in Lines. Every other line is malformed.
//
// A gap is reported between 2 consecutive valid lines whose times differ by
// more than minGap. A backwards jump is never a gap, and minGap <= 0 reports
// no gaps.
//
// A read error or a line longer than 64 KiB stops Summarize. It then returns
// the summary of the lines before that point and an error that wraps the
// cause, so errors.Is works for the reader's error and for bufio.ErrTooLong.
func Summarize(r io.Reader, minGap time.Duration) (Summary, error) {
	panic("TODO: implement")
}

// Write writes s to w as a text report of 5 lines plus 1 line per gap:
//
//	lines:     9
//	malformed: 2
//	first:     2026-10-16T03:10:01Z
//	last:      2026-10-16T04:00:02Z
//	levels:    ERROR=1 INFO=4 WARN=1
//	gap:       2026-10-16T03:12:09Z to 2026-10-16T03:59:41Z (47m32s)
//
// Times use the time.RFC3339 layout in their own zone. Levels are sorted A
// to Z. first, last and levels show "-" when there is nothing to show.
//
// Write returns nil on success. After the first write error it writes
// nothing more and returns an error that wraps it.
func Write(w io.Writer, s Summary) error {
	panic("TODO: implement")
}
