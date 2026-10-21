package tableprint_test

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"testing"
	"time"

	tableprint "github.com/yornik/go-practice/exercises/18-tableprint"
)

// hangLimit is how long guard lets a test run. A correct solution needs well
// under a millisecond per test.
const hangLimit = 5 * time.Second

// guard turns the 2 ways a wrong solution can break the test binary into
// readable failures. Use it as the first line of a test or subtest:
// defer guard(t)().
//
//   - A panic, such as the "TODO: implement" of the stub or a reflect call
//     on the wrong kind of value, becomes a failure of this test with the
//     stack, and the other tests still run.
//   - A loop that never ends would hang until the 10-minute test timeout.
//     After hangLimit, guard prints the stack of every goroutine, so you can
//     see where it is stuck, and stops the test binary.
func guard(t *testing.T) func() {
	t.Helper()
	name := t.Name()
	watchdog := time.AfterFunc(hangLimit, func() {
		var stacks bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
		fmt.Fprintf(os.Stderr, "\n%s is still running after %v: a call does not return "+
			"(does a loop never end?).\nStacks of all goroutines:\n\n%s\n", name, hangLimit, stacks.String())
		os.Exit(2)
	})
	return func() {
		t.Helper()
		watchdog.Stop()
		if v := recover(); v != nil {
			t.Fatalf("panicked: %v\n%s", v, debug.Stack())
		}
	}
}

// wantRender renders rows and fails the test unless Render returns nil and
// writes exactly want.
func wantRender(t *testing.T, rows any, want string) {
	t.Helper()
	var b bytes.Buffer
	if err := tableprint.Render(&b, rows); err != nil {
		t.Fatalf("Render(%#v) error = %v, want nil", rows, err)
	}
	if got := b.String(); got != want {
		t.Errorf("Render(%#v) wrote:\n%s\nwant:\n%s\nas Go strings:\n got  %q\n want %q",
			rows, got, want, got, want)
	}
}

// recordingWriter counts what Render writes to it.
type recordingWriter struct {
	calls int
	bytes.Buffer
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.Buffer.Write(p)
}

// failingWriter accepts the first limit bytes, then fails every write with
// err.
type failingWriter struct {
	limit   int
	written int
	err     error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	n := min(len(p), w.limit-w.written)
	w.written += n
	if n < len(p) {
		return n, w.err
	}
	return n, nil
}

// pod is a row type in the shape of the output of kubectl get pods.
type pod struct {
	Name     string
	Status   string
	Restarts int
}

// Meta is embedded in other row types. It is exported so that the embedded
// field is exported too.
type Meta struct {
	Name string
	Zone string
}

// meta is the unexported twin of Meta: as an embedded field it is
// unexported.
type meta struct {
	Name string
}
