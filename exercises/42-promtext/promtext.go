// Package promtext parses the Prometheus text exposition format (what an
// exporter serves on /metrics), writes it back in a canonical form, and
// computes per-second rates from two scrapes.
package promtext

import (
	"fmt"
	"io"
)

// Label is one label of a series, for example {Name: "code", Value: "200"}.
type Label struct {
	Name  string
	Value string
}

// Sample is one sample line: a series (name and labels), its value and an
// optional timestamp.
type Sample struct {
	// Name is the name on the sample line, suffix included, for example
	// "http_request_duration_seconds_bucket".
	Name string
	// Labels are sorted by Name. A sample without labels has a nil or empty
	// slice.
	Labels []Label
	Value  float64
	// TimestampMs is the timestamp in milliseconds since the Unix epoch. It
	// is 0 when HasTimestamp is false.
	TimestampMs  int64
	HasTimestamp bool
}

// Family is a metric family: a metric name with its HELP text, its TYPE and
// all its samples.
type Family struct {
	Name string
	// Help is the unescaped HELP text, "" when there is no HELP line.
	Help string
	// Type is "counter", "gauge", "histogram", "summary" or "untyped".
	// A family without a TYPE line is "untyped".
	Type string
	// Samples are in input order.
	Samples []Sample
}

// ParseError reports malformed input.
type ParseError struct {
	Line int    // 1-based number of the offending line
	Msg  string // short description of the problem
}

// Error returns "promtext: line <Line>: <Msg>".
func (e *ParseError) Error() string {
	return fmt.Sprintf("promtext: line %d: %s", e.Line, e.Msg)
}

// Parse reads the text exposition format from r and returns the metric
// families in the order they first appear. The README lists the rules.
//
// On malformed input Parse returns nil and a *ParseError with the line
// number. If reading r fails, it returns nil and an error that wraps the
// reader's error.
func Parse(r io.Reader) ([]Family, error) {
	panic("TODO: implement")
}
