package logmerge

import (
	"fmt"
	"io"
	"iter"
)

// ParseError reports a line that ParseKubectl could not parse.
type ParseError struct {
	Source string // the source name passed to ParseKubectl
	Line   int    // 1-based line number in the input
	Err    error  // the reason, for example the error from time.Parse
}

// Error returns "<Source>:<Line>: <Err>", for example
// "api-7d9f:12: parsing time ...".
func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d: %v", e.Source, e.Line, e.Err)
}

// Unwrap returns Err, so errors.Is and errors.As see the reason.
func (e *ParseError) Unwrap() error { return e.Err }

// ParseKubectl parses the output of kubectl logs --timestamps, read from r.
//
// Each line is an RFC 3339 timestamp, one space and the message:
//
//	2026-10-16T12:00:00.123456789Z GET /healthz 200
//
// For every line it yields an Entry with Source set to source, and a nil
// error. An empty line is skipped. A line whose timestamp does not parse
// yields a *ParseError, and parsing goes on with the next line. If reading r
// fails, the sequence yields one error that wraps the read error and ends.
// When the error is not nil, ignore the Entry.
//
// ParseKubectl is lazy: calling it does not read r, and ranging reads r only
// as far as it needs for the next entry, so it works on endless input such as
// kubectl logs -f. It does not close r. The README lists the exact rules.
func ParseKubectl(source string, r io.Reader) iter.Seq2[Entry, error] {
	panic("TODO: implement")
}
