package grep_test

import (
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	grep "github.com/yornik/go-practice/exercises/29-grep"
)

// ---- Step 3: context lines and separators ----

func TestContext(t *testing.T) {
	requireFilter(t)
	// In every case Match is contains("ERROR").
	cases := []struct {
		name          string
		before, after int
		input         []string
		want          []string
	}{
		{
			"no context: only the matches, no separator", 0, 0,
			[]string{"a", "ERROR 1", "b", "c", "ERROR 2", "d"},
			[]string{"ERROR 1", "ERROR 2"},
		},
		{
			"after 1 (grep -A 1)", 0, 1,
			[]string{"a", "ERROR 1", "b", "c", "d", "ERROR 2", "e"},
			[]string{"ERROR 1", "b", "--", "ERROR 2", "e"},
		},
		{
			"before 1 (grep -B 1)", 1, 0,
			[]string{"a", "ERROR 1", "b", "c", "d", "ERROR 2", "e"},
			[]string{"a", "ERROR 1", "--", "d", "ERROR 2"},
		},
		{
			"before 1 and after 1 (grep -C 1)", 1, 1,
			[]string{"a", "ERROR 1", "b", "c", "d", "ERROR 2", "e"},
			[]string{"a", "ERROR 1", "b", "--", "d", "ERROR 2", "e"},
		},
		{
			// b and c are adjacent: no line is skipped, so no separator.
			"windows that touch merge into one group", 1, 1,
			[]string{"a", "ERROR 1", "b", "c", "ERROR 2", "d"},
			[]string{"a", "ERROR 1", "b", "c", "ERROR 2", "d"},
		},
		{
			"overlapping windows write each line once", 2, 2,
			[]string{"a", "b", "ERROR 1", "c", "ERROR 2", "d", "e", "f"},
			[]string{"a", "b", "ERROR 1", "c", "ERROR 2", "d", "e"},
		},
		{
			// ERROR 2 is inside the After window of ERROR 1. It is a match
			// itself, so 2 more lines follow it.
			"a match inside the after window restarts the countdown", 0, 2,
			[]string{"ERROR 1", "a", "ERROR 2", "b", "c", "d"},
			[]string{"ERROR 1", "a", "ERROR 2", "b", "c"},
		},
		{
			"one skipped line is enough for a separator", 0, 1,
			[]string{"ERROR 1", "a", "b", "ERROR 2"},
			[]string{"ERROR 1", "a", "--", "ERROR 2"},
		},
		{
			"before stops at the start of the input", 3, 0,
			[]string{"a", "ERROR 1", "b"},
			[]string{"a", "ERROR 1"},
		},
		{
			"after stops at the end of the input, no separator at the end", 0, 3,
			[]string{"a", "b", "ERROR 1", "c"},
			[]string{"ERROR 1", "c"},
		},
		{
			"no separator before the first group", 1, 0,
			[]string{"a", "b", "c", "ERROR 1"},
			[]string{"c", "ERROR 1"},
		},
		{
			"after-context lines are not written again as before-context", 2, 1,
			[]string{"ERROR 1", "a", "ERROR 2"},
			[]string{"ERROR 1", "a", "ERROR 2"},
		},
		{
			"before only reaches back to the last written line", 3, 0,
			[]string{"ERROR 1", "a", "ERROR 2"},
			[]string{"ERROR 1", "a", "ERROR 2"},
		},
		{
			// Only the last 2 lines before each match are kept.
			"many lines between the matches", 2, 0,
			numbered(20, 5, 13, 19),
			[]string{
				"line 3", "line 4", "line 5 ERROR", "--",
				"line 11", "line 12", "line 13 ERROR", "--",
				"line 17", "line 18", "line 19 ERROR",
			},
		},
		{
			"context larger than the input", 10, 10,
			[]string{"a", "ERROR 1", "b"},
			[]string{"a", "ERROR 1", "b"},
		},
		{
			"every line matches", 2, 2,
			[]string{"ERROR 1", "ERROR 2", "ERROR 3"},
			[]string{"ERROR 1", "ERROR 2", "ERROR 3"},
		},
		{
			"no match, no output", 2, 2,
			[]string{"a", "b", "c"},
			nil,
		},
		{
			"empty lines are context too", 1, 1,
			[]string{"", "ERROR 1", ""},
			[]string{"", "ERROR 1", ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt := grep.Options{Match: contains("ERROR"), Before: tc.before, After: tc.after}
			input := text(tc.input...)
			out, st, err := filter(t, strings.NewReader(input), opt)
			if err != nil {
				t.Fatalf("Filter error = %v, want nil", err)
			}
			if want := text(tc.want...); out != want {
				t.Errorf("Filter(Before %d, After %d) on %q\n got  %q\n want %q",
					tc.before, tc.after, tc.input, outputLines(out), tc.want)
			}
			wantStats := grep.Stats{
				Lines:     len(tc.input),
				Matches:   countMatches(tc.input),
				BytesRead: int64(len(input)),
			}
			if st != wantStats {
				t.Errorf("stats = %+v, want %+v", st, wantStats)
			}
		})
	}
}

// TestContextRandom compares Filter with reference on 300 generated inputs,
// read in all the ways of the readers list. The inputs have empty lines,
// lines of a few KiB and a missing final "\n" now and then.
func TestContextRandom(t *testing.T) {
	requireFilter(t)
	rng := rand.New(rand.NewPCG(4, 2)) // fixed seed: the same inputs every run
	for i := range 300 {
		input := randomInput(rng)
		before, after := rng.IntN(5), rng.IntN(5)
		rd := readers[i%len(readers)]
		name := fmt.Sprintf("%d/%s/before=%d,after=%d", i, rd.name, before, after)
		t.Run(name, func(t *testing.T) {
			checkAgainstReference(t, input, before, after, rd.wrap(strings.NewReader(input)))
		})
	}
}

// FuzzContext compares Filter with reference on inputs that the fuzzer
// invents. Plain go test runs only the seeds below; run
// go test -fuzz=FuzzContext to search for more.
func FuzzContext(f *testing.F) {
	requireFilter(f)
	f.Add("a\nERROR 1\nb\nc\nd\nERROR 2\ne", uint8(1), uint8(1))
	f.Add("ERROR\n\n\nERROR\n", uint8(0), uint8(2))
	f.Add("\n", uint8(3), uint8(0))
	f.Fuzz(func(t *testing.T, input string, before, after uint8) {
		// \r is left open by the rules, and the default limit is 1 MiB.
		if strings.Contains(input, "\r") || len(input) > grep.DefaultMaxLineBytes {
			t.Skip()
		}
		checkAgainstReference(t, input, int(before%6), int(after%6), strings.NewReader(input))
	})
}

func checkAgainstReference(t *testing.T, input string, before, after int, r io.Reader) {
	t.Helper()
	opt := grep.Options{Match: contains("ERROR"), Before: before, After: after}
	out, st, err := filter(t, r, opt)
	if err != nil {
		t.Fatalf("Filter error = %v, want nil", err)
	}
	want, matches := reference(input, contains("ERROR"), before, after)
	if out != want {
		got, exp := outputLines(out), outputLines(want)
		i := 0
		for i < len(got) && i < len(exp) && got[i] == exp[i] {
			i++
		}
		msg := fmt.Sprintf("Filter(Before %d, After %d): output differs from reference at output line %d", before, after, i+1)
		msg += fmt.Sprintf("\n got  %s\n want %s", lineAt(got, i), lineAt(exp, i))
		if len(input) < 2000 {
			msg += fmt.Sprintf("\ninput lines: %q", outputLines(input))
		}
		t.Fatal(msg)
	}
	wantStats := grep.Stats{Lines: len(outputLines(input)), Matches: matches, BytesRead: int64(len(input))}
	if st != wantStats {
		t.Errorf("stats = %+v, want %+v", st, wantStats)
	}
}

// reference computes what Filter must write, the slow and obvious way: it
// holds every line in memory, marks each line within Before/After of a match,
// then writes the marked lines with "--" between runs that are not adjacent.
// Filter must do the same while it streams.
func reference(input string, match func(string) bool, before, after int) (out string, matches int) {
	lines := outputLines(input)
	marked := make([]bool, len(lines))
	for i, l := range lines {
		if match(l) {
			matches++
			for j := max(0, i-before); j <= min(len(lines)-1, i+after); j++ {
				marked[j] = true
			}
		}
	}
	var b strings.Builder
	last := -1 // index of the last written line
	for i, m := range marked {
		if !m {
			continue
		}
		if last >= 0 && i > last+1 && (before > 0 || after > 0) {
			b.WriteString("--\n")
		}
		b.WriteString(lines[i] + "\n")
		last = i
	}
	return b.String(), matches
}

// outputLines splits s into lines the way the rules define them: on "\n",
// and a final line without "\n" still counts.
func outputLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func lineAt(lines []string, i int) string {
	if i >= len(lines) {
		return "(no more lines)"
	}
	if l := lines[i]; len(l) > 80 {
		return fmt.Sprintf("%q... (%d bytes)", l[:80], len(l))
	}
	return fmt.Sprintf("%q", lines[i])
}

func countMatches(lines []string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, "ERROR") {
			n++
		}
	}
	return n
}

// numbered returns n lines "line 1" ... "line n". The lines whose numbers are
// in errs get " ERROR" appended.
func numbered(n int, errs ...int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	for _, e := range errs {
		lines[e-1] += " ERROR"
	}
	return lines
}

// randomInput builds up to 60 lines. Each non-empty line starts with its
// line number, so a line written twice or in the wrong place is easy to spot.
func randomInput(rng *rand.Rand) string {
	var b strings.Builder
	n := rng.IntN(61)
	for i := range n {
		kind := rng.IntN(20)
		if kind > 0 { // kind 0 is an empty line
			fmt.Fprintf(&b, "%d", i+1)
			if rng.IntN(5) == 0 {
				b.WriteString(" ERROR")
			}
			if kind == 1 { // a few KiB, so the input outgrows small read buffers
				b.WriteString(" " + strings.Repeat("y", 1000+rng.IntN(5000)))
			} else {
				b.WriteString(" " + strings.Repeat("z", rng.IntN(40)))
			}
		}
		if i < n-1 || rng.IntN(3) > 0 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
