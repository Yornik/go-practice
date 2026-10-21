// Package syslog parses classic BSD syslog lines (RFC 3164) such as
//
//	<11>Dec 31 23:58:02 feeder-0412 camd[311]: lens heater failed
//
// into typed messages, and reports the exact column of a malformed line.
package syslog

import (
	"errors"
	"time"
)

// Severity is the syslog severity: the priority value modulo 8. A lower
// value is more severe, so "msg.Severity <= Error" selects errors and worse.
type Severity int

// The 8 severities of RFC 3164, from most to least severe.
const (
	Emergency Severity = iota
	Alert
	Critical
	Error
	Warning
	Notice
	Info
	Debug
)

// String returns the keyword that syslog.conf uses for s: "emerg", "alert",
// "crit", "err", "warning", "notice", "info" or "debug". For any other value
// it returns "Severity(N)", for example "Severity(8)".
func (s Severity) String() string {
	panic("TODO: implement")
}

// Message is 1 parsed syslog line.
type Message struct {
	Facility int       // priority / 8: 0 to 23
	Severity Severity  // priority % 8
	Time     time.Time // the timestamp with the year Parse chose, in now's location
	Host     string    // the HOSTNAME field; never empty
	App      string    // the tag without the [pid] and the colon; never empty
	PID      int       // the number between [ and ]; 0 when the tag has no [pid]
	Text     string    // the rest of the line after "TAG: "; may be empty
}

// The sentinel errors. The Err of every *ParseError from Parse wraps exactly
// 1 of them, the one that belongs to its Field. Check for them with errors.Is.
var (
	ErrPriority  = errors.New("bad priority")
	ErrTimestamp = errors.New("bad timestamp")
	ErrHost      = errors.New("missing host")
	ErrTag       = errors.New("bad tag")
)

// ParseError reports a line that Parse rejects, and where.
type ParseError struct {
	// Field is the field where the problem is: "priority", "timestamp",
	// "host" or "tag".
	Field string
	// Col is the 1-based byte column of the problem: the byte at
	// line[Col-1], or len(line)+1 when the problem is the end of the line.
	Col int
	// Err is the cause. It is or wraps the sentinel of Field: ErrPriority,
	// ErrTimestamp, ErrHost or ErrTag. For a timestamp that time.Parse
	// rejects, it wraps the *time.ParseError too.
	Err error
}

// Error returns "syslog: column <Col>: <Field>: <Err>", for example
// "syslog: column 21: host: missing host".
func (e *ParseError) Error() string {
	panic("TODO: implement")
}

// Unwrap returns Err, so that errors.Is and errors.As look inside it.
func (e *ParseError) Unwrap() error {
	panic("TODO: implement")
}

// Parse reads 1 syslog line of the form
//
//	<PRI>Mmm dd hh:mm:ss HOST TAG: TEXT
//
// The timestamp has no year and no zone. Parse reads it as a wall clock time
// in now's location, in now's year, or in the year before when that puts the
// time more than 7 days after now. The README holds the exact format.
//
// On a malformed line Parse returns the zero Message and a *ParseError that
// points at the first problem. Parse never guesses a missing field, and it
// never panics.
func Parse(line string, now time.Time) (Message, error) {
	panic("TODO: implement")
}
