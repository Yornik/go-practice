// Package logfmt parses logfmt log lines such as
//
//	ts=2026-10-16T12:00:00Z level=error msg="db timeout" latency_ms=532
//
// into ordered key/value pairs (Parse) and into typed records (ParseRecord).
package logfmt

import (
	"errors"
	"time"
)

// Pair is one key/value pair of a logfmt line. Value is already unquoted and
// unescaped: for msg="say \"hi\"" the Value is `say "hi"`.
type Pair struct {
	Key   string
	Value string
}

// SyntaxError reports a malformed logfmt line.
type SyntaxError struct {
	// Col is the 1-based byte column of the offending byte: the byte at
	// line[Col-1]. It counts bytes, not runes.
	Col int
	// Msg is a short, human-readable description of the problem, for example
	// "unterminated quoted value". The wording is up to you.
	Msg string
}

// Error returns "logfmt: column <Col>: <Msg>", for example
// "logfmt: column 5: unterminated quoted value".
func (e *SyntaxError) Error() string {
	panic("TODO: implement")
}

// Parse splits one logfmt line into its key/value pairs.
//
// The pairs come back in the order they appear in the line. Duplicate keys
// are kept. An empty or whitespace-only line gives no pairs and no error.
//
// On malformed input Parse returns a nil slice and a *SyntaxError that points
// at the first problem in the line. Parse never panics. The README lists the
// exact syntax and the column each error must report.
func Parse(line string) ([]Pair, error) {
	panic("TODO: implement")
}

// Level is the severity of a log record. Higher is more severe, so
// "rec.Level >= Warn" selects warnings and errors.
type Level int

// The known levels. The zero value is Debug.
const (
	Debug Level = iota
	Info
	Warn
	Error
)

// String returns "debug", "info", "warn" or "error". For any other value it
// returns "Level(N)", for example "Level(7)".
func (l Level) String() string {
	panic("TODO: implement")
}

// ErrUnknownLevel is wrapped by the error ParseLevel returns for a word it
// does not know. Check for it with errors.Is.
var ErrUnknownLevel = errors.New("unknown level")

// ParseLevel converts a level word to a Level. It accepts "debug", "info",
// "warn", "warning" and "error" in any letter case, and nothing else.
//
// For any other input it returns an error that wraps ErrUnknownLevel and
// contains the input quoted with %q, for example: unknown level "fatal".
func ParseLevel(s string) (Level, error) {
	panic("TODO: implement")
}

// Record is a parsed log line with the well-known keys pulled out.
type Record struct {
	Time   time.Time         // from ts (RFC 3339), required
	Level  Level             // from level, required
	Msg    string            // from msg, optional ("" when absent)
	Fields map[string]string // every other key; never nil
}

// ErrMissingField is wrapped by the error ParseRecord returns when a required
// key (ts or level) is absent from the line. Check for it with errors.Is.
var ErrMissingField = errors.New("missing required field")

// ParseRecord parses a line with Parse and builds a Record from the pairs.
//
// If a key appears more than once, its last occurrence wins. A syntax error
// from Parse is returned so that errors.As finds the *SyntaxError. Errors about
// ts or level start with the key and a colon ("ts: ...", "level: ...") and wrap
// the cause: ErrMissingField, the *time.ParseError from time.Parse, or the
// ParseLevel error. ts is checked before level.
func ParseRecord(line string) (Record, error) {
	panic("TODO: implement")
}
