// Package repeated makes noisy logs readable. Collapse folds every run of
// repeated messages into 1 line and 1 marker line, the way classic syslogd
// does, and CountMessages counts how often each message appears.
//
// A log line starts with a timestamp, followed by a space and the message:
//
//	2026-09-09T03:01:26Z netd: sfp0 link down
//	<timestamp>          <message>
package repeated

// Message returns the message of a line: the text after the first space.
// Only a space (0x20) separates the timestamp from the message; a tab is an
// ordinary byte. The message is not trimmed, so further spaces are kept.
//
// A line without a space has no timestamp, and Message returns it whole.
// A line that starts with a space has an empty timestamp.
//
//	Message("2026-09-09T03:01:26Z netd: sfp0 link down") // "netd: sfp0 link down"
//	Message("booting")                                   // "booting"
func Message(line string) string {
	panic("TODO: implement")
}

// Collapse keeps the first line of every run of 2 or more consecutive lines
// with the same Message, and replaces the rest of the run with 1 marker line:
//
//	<timestamp of the last line of the run> last message repeated N times
//
// N is the length of the run minus 1. For N = 1 the marker ends in "1 time".
// When the timestamp of the last line is empty, the marker is
// "last message repeated N times", with no leading space.
//
// Every other line is copied unchanged, in input order. Collapse does not
// modify the slice lines. For empty input it returns nil or an empty slice.
func Collapse(lines []string) []string {
	panic("TODO: implement")
}

// Count is the number of lines N that have the message Message.
type Count struct {
	Message string
	N       int
}

// CountMessages returns 1 Count per distinct Message, in order of first
// appearance. N counts every line with that message, consecutive or not.
// For empty input it returns nil or an empty slice.
func CountMessages(lines []string) []Count {
	panic("TODO: implement")
}
