// Package grep filters a stream of log lines the way grep -B, -A and -C do:
// it writes the matching lines with a few lines of context around them, reads
// the input once, and keeps only a bounded number of lines in memory.
package grep

import (
	"errors"
	"io"
)

// DefaultMaxLineBytes is the line length limit Filter uses when
// Options.MaxLineBytes is 0: 1 MiB.
const DefaultMaxLineBytes = 1 << 20

// Options configures Filter.
type Options struct {
	// Match reports whether a line matches. It gets the line without its
	// "\n". Filter calls it exactly once for every line, in input order.
	// Match must not be nil.
	Match func(line string) bool

	// Before and After are the number of context lines to write before and
	// after each matching line, like grep -B and -A. 0 means none. They must
	// not be negative.
	Before, After int

	// MaxLineBytes is the longest line Filter accepts, in bytes, not counting
	// the "\n". 0 means DefaultMaxLineBytes. It must not be negative.
	MaxLineBytes int
}

// Stats describes what Filter did. When Filter returns an error, Stats
// describes the work done up to that point.
type Stats struct {
	Lines     int   // lines read and passed to Match
	Matches   int   // lines for which Match returned true
	BytesRead int64 // bytes read from the input
}

// ErrLineTooLong is wrapped by the error Filter returns when a line is longer
// than the limit. Check for it with errors.Is.
var ErrLineTooLong = errors.New("line too long")

// Filter reads lines from r, calls opt.Match for each one, and writes every
// matching line, with up to opt.Before lines before it and opt.After lines
// after it, to w. Each line is written at most once and ends with "\n". When
// context is on, a "--" line separates groups that are not adjacent.
//
// Filter streams: it reads r once, writes output as soon as it is decided,
// and holds at most opt.Before lines plus the current one in memory. When it
// returns, it is done with r and w: nothing it started keeps running.
//
// It returns an error, together with the Stats so far, for invalid options
// (before reading anything), a line longer than the limit (wraps
// ErrLineTooLong and names the line number), a read error from r, or a write
// error from w. The read and write errors are wrapped, so errors.Is works.
func Filter(r io.Reader, w io.Writer, opt Options) (Stats, error) {
	panic("TODO: implement")
}

// CountingReader wraps an io.Reader and counts the bytes read through it.
// It is not safe for concurrent use.
type CountingReader struct {
	// TODO: add fields.
}

// NewCountingReader returns a CountingReader that reads from r.
func NewCountingReader(r io.Reader) *CountingReader {
	panic("TODO: implement")
}

// Read calls Read on the wrapped reader once and returns its n and err
// unchanged. It adds n to the count, also when err is not nil.
func (c *CountingReader) Read(p []byte) (int, error) {
	panic("TODO: implement")
}

// N returns the number of bytes read so far.
func (c *CountingReader) N() int64 {
	panic("TODO: implement")
}
