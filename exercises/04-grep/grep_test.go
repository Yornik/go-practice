package grep_test

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	grep "github.com/yornik/go-practice/exercises/04-grep"
)

// ---- Step 2: lines and stats, no context ----

func TestLinesSplitAndStats(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string // what Filter writes
		lines   int
		matches int
	}{
		{"empty input", "", "", 0, 0},
		{"one line without newline", "ERROR boom", "ERROR boom\n", 1, 1},
		{"last line without newline still counts", "ok\nERROR boom", "ERROR boom\n", 2, 1},
		{"a single newline is one empty line", "\n", "", 1, 0},
		{"empty lines count", "\n\nERROR boom\n\n", "ERROR boom\n", 4, 1},
		{"no match", text("a", "b", "c"), "", 3, 0},
		{"every line matches", text("ERROR 1", "ERROR 2"), text("ERROR 1", "ERROR 2"), 2, 2},
		// Without context there are no "--" separators, even with lines
		// skipped between the matches.
		{
			"only the matching lines",
			text("ok", "ERROR 1", "ok", "ok", "ERROR 2", "ok"),
			text("ERROR 1", "ERROR 2"), 6, 2,
		},
		{
			"lines are written unchanged",
			text("  ERROR\tcafé ☕  ", "ok"),
			text("  ERROR\tcafé ☕  "), 2, 1,
		},
	}
	for _, rd := range readers {
		for _, tc := range cases {
			t.Run(rd.name+"/"+tc.name, func(t *testing.T) {
				out, st, err := filter(t, rd.wrap(strings.NewReader(tc.input)), grep.Options{Match: contains("ERROR")})
				if err != nil {
					t.Fatalf("Filter(%q) error = %v, want nil", tc.input, err)
				}
				if out != tc.want {
					t.Errorf("Filter(%q) wrote %q, want %q", tc.input, out, tc.want)
				}
				want := grep.Stats{Lines: tc.lines, Matches: tc.matches, BytesRead: int64(len(tc.input))}
				if st != want {
					t.Errorf("Filter(%q) stats = %+v, want %+v", tc.input, st, want)
				}
			})
		}
	}
}

func TestLinesMatchCalledOncePerLine(t *testing.T) {
	input := "first\n\n  spaced  \nERROR x\nlast"
	want := []string{"first", "", "  spaced  ", "ERROR x", "last"}
	for _, rd := range readers {
		t.Run(rd.name, func(t *testing.T) {
			var got []string
			opt := grep.Options{
				Match: func(line string) bool {
					got = append(got, line)
					return strings.Contains(line, "ERROR")
				},
				// Lines written as context are passed to Match too.
				Before: 2,
				After:  2,
			}
			if _, _, err := filter(t, rd.wrap(strings.NewReader(input)), opt); err != nil {
				t.Fatalf("Filter error = %v, want nil", err)
			}
			if !slices.Equal(got, want) {
				t.Errorf("Match was called with %q\nwant %q (once per line, in order, without the \\n)", got, want)
			}
		})
	}
}

// ---- Step 4: long lines ----

func TestLongLineOver64KiB(t *testing.T) {
	// bufio.Scanner gives up on lines over 64 KiB unless you tell it otherwise.
	long := "ERROR body=" + strings.Repeat("x", 100<<10)
	input := text("before", long, "after")
	for _, rd := range readers {
		t.Run(rd.name, func(t *testing.T) {
			var gotLen int
			opt := grep.Options{
				Match: func(line string) bool {
					if strings.HasPrefix(line, "ERROR") {
						gotLen = len(line)
					}
					return strings.HasPrefix(line, "ERROR")
				},
				Before: 1,
				After:  1,
			}
			out, st, err := filter(t, rd.wrap(strings.NewReader(input)), opt)
			if err != nil {
				t.Fatalf("Filter with a %d-byte line: error = %v, want nil", len(long), err)
			}
			if gotLen != len(long) {
				t.Errorf("Match got the long line with %d bytes, want %d", gotLen, len(long))
			}
			if out != input {
				t.Errorf("Filter wrote %d bytes, want the whole input back (%d bytes)", len(out), len(input))
			}
			want := grep.Stats{Lines: 3, Matches: 1, BytesRead: int64(len(input))}
			if st != want {
				t.Errorf("stats = %+v, want %+v", st, want)
			}
		})
	}
}

func TestLongLineLimit(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string // what Filter writes; every line matches
		lines   int
		errLine int // the line Filter must reject; 0 means no error
	}{
		{"exactly the limit", "0123456789\n", "0123456789\n", 1, 0},
		{"exactly the limit, no newline", "0123456789", "0123456789\n", 1, 0},
		{"one byte over", "0123456789X\n", "", 0, 1},
		// With DataErrReader this line arrives in the same Read as io.EOF.
		{"one byte over, no newline", "0123456789X", "", 0, 1},
		{"empty lines are fine", "\n\n", "\n\n", 2, 0},
		// The lines before the long one are matched and written as usual.
		{"lines before the long one are written", text("a", "b", "0123456789X", "c"), text("a", "b"), 2, 3},
	}
	for _, rd := range readers {
		for _, tc := range cases {
			t.Run(rd.name+"/"+tc.name, func(t *testing.T) {
				opt := grep.Options{Match: all, MaxLineBytes: 10}
				out, st, err := filter(t, rd.wrap(strings.NewReader(tc.input)), opt)
				call := fmt.Sprintf("Filter(%q, MaxLineBytes 10)", tc.input)
				if tc.errLine == 0 && err != nil {
					t.Fatalf("%s error = %v, want nil", call, err)
				}
				if tc.errLine > 0 {
					checkLineTooLong(t, call, err, tc.errLine)
				}
				if out != tc.want {
					t.Errorf("%s wrote %q, want %q", call, out, tc.want)
				}
				if st.Lines != tc.lines || st.Matches != tc.lines {
					t.Errorf("%s stats = %+v, want Lines and Matches %d", call, st, tc.lines)
				}
			})
		}
	}
}

func TestLongLineDefaultLimit(t *testing.T) {
	limit := strings.Repeat("x", grep.DefaultMaxLineBytes)

	t.Run("exactly 1 MiB is fine", func(t *testing.T) {
		input := text("first", limit, "last")
		out, st, err := filter(t, strings.NewReader(input), grep.Options{Match: all})
		if err != nil {
			t.Fatalf("Filter with a line of exactly DefaultMaxLineBytes: error = %v, want nil", err)
		}
		if out != input {
			t.Errorf("Filter wrote %d bytes, want the whole input back (%d bytes)", len(out), len(input))
		}
		if st.Lines != 3 {
			t.Errorf("stats = %+v, want Lines 3", st)
		}
	})

	t.Run("1 MiB plus 1 byte is too long", func(t *testing.T) {
		input := text("first", limit+"x", "last")
		out, st, err := filter(t, strings.NewReader(input), grep.Options{Match: all})
		checkLineTooLong(t, "Filter(a line of DefaultMaxLineBytes+1 bytes)", err, 2)
		if out != "first\n" {
			t.Errorf("Filter wrote %d bytes, want only %q", len(out), "first\n")
		}
		if st.Lines != 1 {
			t.Errorf("stats = %+v, want Lines 1", st)
		}
	})
}

func TestLongLineStopsEarly(t *testing.T) {
	// A 64 MiB line with no newline in sight: a minified JSON dump, or a
	// binary file piped in by mistake. With a limit of 1000 bytes, Filter
	// must give up long before it has read the whole line.
	src := &hugeLine{prefix: "ok\n", size: 64 << 20}
	out, st, err := filter(t, src, grep.Options{Match: all, MaxLineBytes: 1000})
	checkLineTooLong(t, "Filter(a 64 MiB line, MaxLineBytes 1000)", err, 2)
	if out != "ok\n" {
		t.Errorf("Filter wrote %q, want %q", out, "ok\n")
	}
	if st.Lines != 1 {
		t.Errorf("stats = %+v, want Lines 1", st)
	}
	if src.served > 1<<20 {
		t.Errorf("Filter read %d bytes before it gave up, want at most 1 MiB (1048576): check the length while you read, not after", src.served)
	}
}

// ---- Step 5: errors ----

func TestErrInvalidOptions(t *testing.T) {
	cases := []struct {
		name string
		opt  grep.Options
	}{
		{"nil Match", grep.Options{Before: 1}},
		{"negative Before", grep.Options{Match: all, Before: -1}},
		{"negative After", grep.Options{Match: all, After: -1}},
		{"negative MaxLineBytes", grep.Options{Match: all, MaxLineBytes: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &spyReader{r: strings.NewReader(text("ERROR 1"))}
			out, st, err := filter(t, r, tc.opt)
			if err == nil {
				t.Errorf("Filter(%+v) error = nil, want an error", tc.opt)
			}
			if r.reads != 0 {
				t.Errorf("Filter(%+v) called Read %d times, want 0: check the options first", tc.opt, r.reads)
			}
			if out != "" || st != (grep.Stats{}) {
				t.Errorf("Filter(%+v) wrote %q with stats %+v, want nothing and zero stats", tc.opt, out, st)
			}
		})
	}
}

func TestErrRead(t *testing.T) {
	errDisk := errors.New("read /var/log/app.log: input/output error")
	input := text("a", "ERROR 1", "b", "c")
	sources := []struct {
		name string
		r    func() io.Reader
	}{
		{"error after the data", func() io.Reader {
			return io.MultiReader(strings.NewReader(input), iotest.ErrReader(errDisk))
		}},
		{"error together with the last data", func() io.Reader {
			return &errAtEnd{data: input, err: errDisk}
		}},
	}
	for _, src := range sources {
		for _, rd := range readers {
			if rd.name == "DataErrReader" && src.name == "error together with the last data" {
				// iotest.DataErrReader itself drops data that comes
				// together with an error other than io.EOF.
				continue
			}
			t.Run(src.name+"/"+rd.name, func(t *testing.T) {
				opt := grep.Options{Match: contains("ERROR"), Before: 1, After: 1}
				out, st, err := filter(t, rd.wrap(src.r()), opt)
				if !errors.Is(err, errDisk) {
					t.Fatalf("Filter error = %v, want an error that wraps %q", err, errDisk)
				}
				// The lines read before the error are processed as usual.
				if want := text("a", "ERROR 1", "b"); out != want {
					t.Errorf("Filter wrote %q, want %q", out, want)
				}
				want := grep.Stats{Lines: 4, Matches: 1, BytesRead: int64(len(input))}
				if st != want {
					t.Errorf("stats = %+v, want %+v", st, want)
				}
			})
		}
	}
}

func TestErrReadIsNotRetried(t *testing.T) {
	// TimeoutReader fails the 2nd Read with iotest.ErrTimeout, and the
	// Reads after that work again. Filter must stop at the error and
	// return it: retrying or skipping is the caller's decision.
	r := iotest.TimeoutReader(strings.NewReader(text("ERROR 1", "ERROR 2", "ERROR 3")))
	_, _, err := filter(t, r, grep.Options{Match: contains("ERROR")})
	if !errors.Is(err, iotest.ErrTimeout) {
		t.Errorf("Filter error = %v, want an error that wraps iotest.ErrTimeout", err)
	}
}

func TestErrWrite(t *testing.T) {
	// The disk fills up, or the reader at the other end of the pipe went
	// away. The writer accepts room bytes, then every Write fails. Each case
	// makes a different kind of output line hit the error.
	errFull := errors.New("write /tmp/errors.log: no space left on device")
	input := text("ok", "ERROR 1", "a", "b", "ERROR 2") + strings.Repeat("ERROR again\n", 100)
	cases := []struct {
		name           string
		before, after  int
		room           int // bytes written before the disk is full
		lines, matches int // Stats when Filter stops
	}{
		{"a matching line fails", 0, 0, 0, 2, 1},               // "ERROR 1"
		{"a Before line fails", 1, 0, 0, 2, 1},                 // "ok", for ERROR 1
		{"an After line fails", 0, 1, len("ERROR 1\n"), 3, 1},  // "a"
		{"a separator fails", 0, 1, len("ERROR 1\na\n"), 5, 2}, // "--", for ERROR 2
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goroutines := runtime.NumGoroutine()
			w := &failWriter{err: errFull, room: tc.room}
			opt := grep.Options{Match: contains("ERROR"), Before: tc.before, After: tc.after}
			st, err := filterTo(t, strings.NewReader(input), w, opt)
			if !errors.Is(err, errFull) {
				t.Fatalf("Filter error = %v, want an error that wraps %q", err, errFull)
			}
			if w.failed > 1 {
				t.Errorf("Filter called Write %d more time(s) after a Write failed, want 0: stop at the first write error", w.failed-1)
			}
			if st.Lines != tc.lines || st.Matches != tc.matches {
				t.Errorf("stats = %+v, want Lines %d and Matches %d: pass no more lines to Match after the write error",
					st, tc.lines, tc.matches)
			}
			checkNoGoroutineLeft(t, goroutines)
		})
	}
}

// spyReader counts the calls to Read.
type spyReader struct {
	r     io.Reader
	reads int
}

func (s *spyReader) Read(p []byte) (int, error) {
	s.reads++
	return s.r.Read(p)
}
