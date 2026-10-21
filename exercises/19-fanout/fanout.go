// Package fanout runs independent checks, such as the health probes of a
// status page, all at the same time and collects their results.
package fanout

import "errors"

// Check is 1 health check, for example the probe of 1 service.
type Check struct {
	// Name identifies the check in its Result. Names need not be unique.
	Name string
	// Run performs the check and returns nil when it passes. A nil Run
	// counts as a check that panics.
	Run func() error
}

// Result is the outcome of 1 check.
type Result struct {
	// Name is the Name of the check.
	Name string
	// Err is the error that Run returned, unchanged, or an error that wraps
	// ErrPanic if Run panicked.
	Err error
}

// ErrPanic is wrapped by the error of every check whose Run panicked.
var ErrPanic = errors.New("fanout: check panicked")

// RunAll calls the Run function of every check exactly once, each in its own
// goroutine, all at the same time, and waits until every Run has returned.
// It returns 1 Result per check, in the order of checks: result i belongs to
// checks[i], whatever order the checks finish in. For no checks it returns a
// slice of length 0.
//
// A panic in Run does not stop the program. It becomes the Err of that
// check: an error that wraps ErrPanic, with the message
// "fanout: check panicked: " followed by the panic value formatted with %v.
//
// When RunAll returns, every goroutine it started has ended or is about to
// end.
func RunAll(checks []Check) []Result {
	panic("TODO: implement")
}

// Stream calls the Run function of every check exactly once, each in its own
// goroutine, all at the same time, and returns at once. It sends the Result
// of each check on the returned channel as soon as that check's Run returns,
// so results arrive in the order in which the checks finish. Panics become
// errors as in RunAll.
//
// The channel has room for every result, so no check waits for a reader.
// Stream closes the channel after the last result; for no checks it closes
// it at once. When the channel is closed, every goroutine Stream started has
// ended or is about to end. Stream never returns a nil channel.
func Stream(checks []Check) <-chan Result {
	panic("TODO: implement")
}
