package grep_test

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	grep "github.com/yornik/go-practice/exercises/04-grep"
)

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// text joins lines with "\n" and ends the result with "\n":
// text("a", "b") is "a\nb\n". text() is "".
func text(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// contains returns a Match func that reports whether a line contains sub.
func contains(sub string) func(string) bool {
	return func(line string) bool { return strings.Contains(line, sub) }
}

// all is a Match func that matches every line.
func all(string) bool { return true }

// readers wrap the input so that it arrives in different ways. Filter must
// give the same result for every one of them.
var readers = []struct {
	name string
	wrap func(io.Reader) io.Reader
}{
	{"plain", func(r io.Reader) io.Reader { return r }},
	{"OneByteReader", iotest.OneByteReader}, // 1 byte per Read
	{"HalfReader", iotest.HalfReader},       // half of the bytes asked for
	{"DataErrReader", iotest.DataErrReader}, // io.EOF together with the last data
}

// filter runs grep.Filter on r and returns what it wrote to w.
func filter(t *testing.T, r io.Reader, opt grep.Options) (string, grep.Stats, error) {
	t.Helper()
	var out strings.Builder
	st, err := filterTo(t, r, &out, opt)
	return out.String(), st, err
}

// filterTo runs grep.Filter in a goroutine. It fails the test if Filter
// panics (with the stack, so you can find the line) or does not return
// within the timeout.
func filterTo(t *testing.T, r io.Reader, w io.Writer, opt grep.Options) (grep.Stats, error) {
	t.Helper()
	return filterWithin(t, timeout, r, w, opt)
}

// filterWithin is filterTo with a time limit of its own, for the tests that
// read a lot of input.
func filterWithin(t *testing.T, limit time.Duration, r io.Reader, w io.Writer, opt grep.Options) (grep.Stats, error) {
	t.Helper()
	type result struct {
		st    grep.Stats
		err   error
		panic string
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- result{panic: fmt.Sprintf("%v\n%s", v, debug.Stack())}
			}
		}()
		st, err := grep.Filter(r, w, opt)
		done <- result{st: st, err: err}
	}()
	select {
	case res := <-done:
		if res.panic != "" {
			t.Fatalf("Filter panicked: %s", res.panic)
		}
		return res.st, res.err
	case <-time.After(limit):
		t.Fatalf("Filter did not return within %v", limit)
		return grep.Stats{}, nil
	}
}

// checkLineTooLong fails the test unless err wraps grep.ErrLineTooLong and
// its message contains "line <n>". call describes the Filter call.
func checkLineTooLong(t *testing.T, call string, err error, n int) {
	t.Helper()
	if !errors.Is(err, grep.ErrLineTooLong) {
		t.Fatalf("%s error = %v, want an error that wraps ErrLineTooLong (line %d is too long)", call, err, n)
	}
	re := regexp.MustCompile(fmt.Sprintf(`\bline %d\b`, n))
	if !re.MatchString(err.Error()) {
		t.Errorf("%s error = %q, want it to contain \"line %d\" (the number of the long line)", call, err, n)
	}
}

// checkNoGoroutineLeft waits until no more than n goroutines run, and fails
// the test if that takes longer than the timeout. Call it with the
// runtime.NumGoroutine() from before Filter ran.
func checkNoGoroutineLeft(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for runtime.NumGoroutine() > n {
		if time.Now().After(deadline) {
			t.Errorf("%d goroutine(s) still running %v after Filter returned, want 0: "+
				"a goroutine that is blocked on a channel after Filter stopped early never ends",
				runtime.NumGoroutine()-n, timeout)
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// errAtEnd returns data and then err. The last bytes of data come in the same
// Read call as err (n > 0 and err != nil), which the io.Reader contract
// allows: the caller must use those bytes before it looks at the error.
type errAtEnd struct {
	data string
	err  error
}

func (r *errAtEnd) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

// failWriter is a disk that fills up: it accepts room bytes, then fails every
// Write with err. failed counts the failed calls.
type failWriter struct {
	err    error
	room   int
	used   int
	failed int
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.failed > 0 || w.used+len(p) > w.room {
		w.failed++
		return 0, w.err
	}
	w.used += len(p)
	return len(p), nil
}

// hugeLine is a reader that sends prefix, then a line of size 'x' bytes,
// then "\n", without holding that line in memory. served counts the bytes it
// handed out.
type hugeLine struct {
	prefix string
	size   int64
	sent   int64 // bytes of the long line sent so far
	done   bool  // the final "\n" was sent
	served int64
}

var xs = []byte(strings.Repeat("x", 32<<10))

func (h *hugeLine) Read(p []byte) (int, error) {
	n := 0
	switch {
	case h.prefix != "":
		n = copy(p, h.prefix)
		h.prefix = h.prefix[n:]
	case h.sent < h.size:
		n = copy(p[:min(int64(len(p)), h.size-h.sent)], xs)
		h.sent += int64(n)
	case !h.done && len(p) > 0:
		p[0] = '\n'
		h.done = true
		n = 1
	default:
		return 0, io.EOF
	}
	h.served += int64(n)
	return n, nil
}
