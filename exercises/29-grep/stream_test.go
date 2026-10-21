package grep_test

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	grep "github.com/yornik/go-practice/exercises/29-grep"
)

// ---- Step 6: streaming and memory ----

// TestStreamOutputBeforeInputEnds feeds the input through a pipe, a few lines
// at a time, like kubectl logs -f | yourtool. After each chunk it waits until
// the output for that chunk has arrived, while the input is still open.
func TestStreamOutputBeforeInputEnds(t *testing.T) {
	pr, pw := io.Pipe()
	// Ends the pipe if the test fails early, so Filter can return.
	t.Cleanup(func() { pw.CloseWithError(errors.New("test ended")) })

	out := &syncBuffer{}
	opt := grep.Options{Match: contains("ERROR"), Before: 2, After: 1}
	type result struct {
		st  grep.Stats
		err error
	}
	done := make(chan result, 1)
	go func() {
		st, err := grep.Filter(pr, out, opt)
		pr.CloseWithError(errors.New("filter returned")) // unblock pending writes to the pipe
		done <- result{st, err}
	}()

	steps := []struct {
		send string
		want string // all output so far
	}{
		// The match and its 2 Before lines can be written at once.
		{text("a", "b", "c", "ERROR 1"), text("b", "c", "ERROR 1")},
		// d is the After line. e, f, g wait: they may become Before lines.
		{text("d", "e", "f", "g"), text("b", "c", "ERROR 1", "d")},
		// Now the separator and the Before lines f and g are known.
		{text("ERROR 2"), text("b", "c", "ERROR 1", "d", "--", "f", "g", "ERROR 2")},
	}
	for i, s := range steps {
		send(t, pw, s.send)
		deadline := time.Now().Add(timeout)
		for out.String() != s.want {
			if time.Now().After(deadline) {
				t.Fatalf("after chunk %d, with the input still open, Filter wrote %q, want %q\n"+
					"(Filter must write as soon as the output is known: do not read all input first, do not wait for a full read buffer, do not buffer the output)",
					i+1, out.String(), s.want)
			}
			time.Sleep(time.Millisecond)
		}
		select {
		case res := <-done:
			t.Fatalf("Filter returned (%+v, %v) while the input was still open", res.st, res.err)
		default:
		}
	}

	sent := 0
	for _, s := range steps {
		sent += len(s.send)
	}
	if err := pw.Close(); err != nil { // end of input
		t.Fatalf("closing the pipe: %v", err)
	}
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("Filter error = %v, want nil", res.err)
		}
		want := grep.Stats{Lines: 9, Matches: 2, BytesRead: int64(sent)}
		if res.st != want {
			t.Errorf("stats = %+v, want %+v", res.st, want)
		}
	case <-time.After(timeout):
		t.Fatalf("Filter did not return within %v after the input was closed", timeout)
	}
}

// TestStreamConstantMemory filters 64 MiB and checks that the heap does not
// grow with the input. Filter may hold the Before lines, the current line and
// its buffers, but not the whole input.
func TestStreamConstantMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("reads 64 MiB")
	}
	const (
		total   = 64 << 20
		lineLen = 256
	)
	src := &repeatLine{line: strings.Repeat("x", lineLen-1) + "\n", left: total}

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapAlloc
	var peak uint64
	calls := 0
	opt := grep.Options{
		Match: func(string) bool {
			calls++
			if calls%4096 == 0 {
				runtime.ReadMemStats(&ms)
				peak = max(peak, ms.HeapAlloc)
			}
			return calls%1000 == 0
		},
		Before: 3,
		After:  3,
	}
	// 64 MiB takes well under a second, but a correct loop that reads one
	// byte at a time can take several seconds with -race on a busy machine.
	st, err := filterWithin(t, 60*time.Second, src, io.Discard, opt)
	if err != nil {
		t.Fatalf("Filter error = %v, want nil", err)
	}
	want := grep.Stats{Lines: total / lineLen, Matches: total / lineLen / 1000, BytesRead: total}
	if st != want {
		t.Errorf("stats = %+v, want %+v", st, want)
	}
	if peak > base && peak-base > 32<<20 {
		t.Errorf("the heap grew by %d MiB while Filter read 64 MiB, want less than 32 MiB: keep only the Before lines, not the input",
			(peak-base)>>20)
	}
}

// syncBuffer is a strings.Builder that is safe for concurrent use: Filter
// writes to it while the test reads it.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// send writes s to the pipe. A pipe write blocks until Filter has read all
// of it, so send fails the test if that takes longer than the timeout.
func send(t *testing.T, pw *io.PipeWriter, s string) {
	t.Helper()
	errc := make(chan error, 1)
	go func() {
		_, err := io.WriteString(pw, s)
		errc <- err
	}()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("writing %q to Filter's input: %v", s, err)
		}
	case <-time.After(timeout):
		t.Fatalf("Filter did not read %q from its input within %v", s, timeout)
	}
}

// repeatLine is a reader that sends line over and over until left bytes have
// been sent. It holds only the one line in memory.
type repeatLine struct {
	line string
	off  int // position in line
	left int
}

func (r *repeatLine) Read(p []byte) (int, error) {
	if r.left == 0 {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && r.left > 0 {
		c := copy(p[n:min(len(p), n+r.left)], r.line[r.off:])
		n += c
		r.left -= c
		r.off = (r.off + c) % len(r.line)
	}
	return n, nil
}
