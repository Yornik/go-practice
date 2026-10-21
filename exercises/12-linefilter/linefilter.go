// Package linefilter selects log lines with small matchers that you combine,
// and builds 1 matcher from a filter expression such as
//
//	gateway !healthz !host=canary-h3-01 level=error|level=warn
package linefilter

// Matcher reports whether a line is selected.
//
// The matchers that Limit and Sample return keep state between calls, and so
// does any Matcher built from them. A Matcher is not safe for concurrent use.
type Matcher func(line string) bool

// Contains returns a Matcher that selects every line that contains sub, with
// the same letter case. Contains("") selects every line.
func Contains(sub string) Matcher {
	panic("TODO: implement")
}

// Field returns a Matcher that selects a line when 1 of its words is exactly
// key=value. White space separates the words, as strings.Fields splits
// them, so a tab and the \r of a CRLF line ending separate words too.
//
// Field does not understand quotes: level="error" is not the word
// level=error, and msg="disk full" is the 2 words msg="disk and full".
func Field(key, value string) Matcher {
	panic("TODO: implement")
}

// Not returns a Matcher that selects exactly the lines that m does not
// select. It calls m once per line.
func Not(m Matcher) Matcher {
	panic("TODO: implement")
}

// All returns a Matcher that selects a line when every matcher in ms selects
// it. It calls the matchers in order, with the same line, and stops at the
// first one that returns false. With no matchers it selects every line.
//
// All keeps its own copy of the list: a later change to the caller's slice
// does not change the Matcher.
func All(ms ...Matcher) Matcher {
	panic("TODO: implement")
}

// Any returns a Matcher that selects a line when at least 1 matcher in ms
// selects it. It calls the matchers in order, with the same line, and stops
// at the first one that returns true. With no matchers it selects no line.
//
// Any keeps its own copy of the list: a later change to the caller's slice
// does not change the Matcher.
func Any(ms ...Matcher) Matcher {
	panic("TODO: implement")
}

// Limit returns a Matcher that selects the first n lines that m selects, and
// no line after those. After it has selected n lines, it no longer calls m.
// With n <= 0 it selects no line and never calls m.
//
// Every Matcher that Limit returns has its own count.
func Limit(n int, m Matcher) Matcher {
	panic("TODO: implement")
}

// Sample returns a Matcher that selects a line when rnd() < rate. rnd returns
// a number from 0 up to but not including 1, as math/rand/v2.Float64 does.
//
// The Matcher calls rnd exactly once per line, with 2 exceptions that never
// call rnd: with rate <= 0 it selects no line, and with rate >= 1 it selects
// every line.
func Sample(rate float64, rnd func() float64) Matcher {
	panic("TODO: implement")
}

// Parse builds a Matcher from a filter expression. The README lists the
// syntax. In short: terms separated by white space must all match, a term
// that starts with ! must not match, a|b matches either alternative, and an
// alternative with = in it is a Field, any other is a Contains. An empty
// expression selects every line.
//
// On a bad term Parse returns a nil Matcher and an error that contains the
// term quoted with %q. It reports the first bad term from the left.
func Parse(expr string) (Matcher, error) {
	panic("TODO: implement")
}

// Filter returns the lines that m selects, in their input order. It calls m
// exactly once per line, from the first line to the last, and never changes
// lines. It returns nil when no line is selected.
func Filter(lines []string, m Matcher) []string {
	panic("TODO: implement")
}
