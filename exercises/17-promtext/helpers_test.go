package promtext_test

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	promtext "github.com/yornik/go-practice/exercises/17-promtext"
)

// labels builds a []promtext.Label from alternating names and values:
// labels("code", "200", "method", "GET") is [{code 200} {method GET}].
// It keeps the order you give.
func labels(nv ...string) []promtext.Label {
	if len(nv)%2 != 0 {
		panic("labels: odd number of arguments")
	}
	var out []promtext.Label
	for i := 0; i < len(nv); i += 2 {
		out = append(out, promtext.Label{Name: nv[i], Value: nv[i+1]})
	}
	return out
}

// sample builds a sample without a timestamp. The labels are alternating
// names and values, as for labels().
func sample(name string, value float64, nv ...string) promtext.Sample {
	return promtext.Sample{Name: name, Labels: labels(nv...), Value: value}
}

// parse runs promtext.Parse on text. A panic in Parse becomes a test failure
// with the stack, and a Parse that does not return within 5 s fails the test
// (a lexer whose position stops moving loops forever).
func parse(t *testing.T, text string) ([]promtext.Family, error) {
	t.Helper()
	return parseFrom(t, fmt.Sprintf("%q", text), strings.NewReader(text))
}

func parseFrom(t *testing.T, what string, r io.Reader) ([]promtext.Family, error) {
	t.Helper()
	type result struct {
		fams     []promtext.Family
		err      error
		panicked any
		stack    []byte
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- result{panicked: p, stack: debug.Stack()}
			}
		}()
		fams, err := promtext.Parse(r)
		done <- result{fams: fams, err: err}
	}()
	select {
	case res := <-done:
		if res.panicked != nil {
			t.Fatalf("Parse(%s) panicked: %v%s", what, res.panicked, panicStack(res.stack))
		}
		return res.fams, res.err
	case <-time.After(5 * time.Second):
		t.Fatalf("Parse(%s) did not return within 5s: does your lexer always move forward?", what)
		return nil, nil
	}
}

// stackShown makes panicStack print a stack only once per test run: one
// stack shows where your code panicked, 30 copies of it hide everything else.
var stackShown atomic.Bool

// panicStack returns the stack from the panic call on, the part that shows
// your code, or a short note if a stack was already printed.
func panicStack(stack []byte) string {
	if stackShown.Swap(true) {
		return " (stack printed in an earlier failure)"
	}
	s := string(stack)
	if i := strings.Index(s, "\npanic("); i >= 0 {
		s = s[i+1:]
	}
	return "\n" + s
}

// mustParse parses text and stops the test if Parse fails. The Write and
// Rates tests use it to build their input, so make the Parse tests pass first.
func mustParse(t *testing.T, text string) []promtext.Family {
	t.Helper()
	fams, err := parse(t, text)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v; the Parse tests must pass before this one", text, err)
	}
	return fams
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// sameValue reports whether two sample values are equal. NaN != NaN in Go
// (IEEE 754), so a plain == would make every NaN sample "different".
func sameValue(a, b float64) bool {
	return a == b || (math.IsNaN(a) && math.IsNaN(b))
}

// sameLabels treats a nil and an empty slice as equal.
func sameLabels(a, b []promtext.Label) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameSample(a, b promtext.Sample) bool {
	return a.Name == b.Name && sameLabels(a.Labels, b.Labels) && sameValue(a.Value, b.Value) &&
		a.TimestampMs == b.TimestampMs && a.HasTimestamp == b.HasTimestamp
}

func sameFamilies(a, b []promtext.Family) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		fa, fb := a[i], b[i]
		if fa.Name != fb.Name || fa.Help != fb.Help || fa.Type != fb.Type || len(fa.Samples) != len(fb.Samples) {
			return false
		}
		for j := range fa.Samples {
			if !sameSample(fa.Samples[j], fb.Samples[j]) {
				return false
			}
		}
	}
	return true
}

// dump formats families for failure messages, one line per family and per
// sample. Labels are printed in the order they are stored, so a sorting bug
// is visible.
func dump(fams []promtext.Family) string {
	if len(fams) == 0 {
		return "    (no families)\n"
	}
	var b strings.Builder
	for _, f := range fams {
		fmt.Fprintf(&b, "    family %q type=%q help=%q\n", f.Name, f.Type, f.Help)
		for _, s := range f.Samples {
			fmt.Fprintf(&b, "        %s\n", dumpSample(s))
		}
	}
	return b.String()
}

func dumpSample(s promtext.Sample) string {
	var b strings.Builder
	b.WriteString(s.Name)
	if len(s.Labels) > 0 {
		b.WriteByte('{')
		for i, l := range s.Labels {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "%s=%q", l.Name, l.Value)
		}
		b.WriteByte('}')
	}
	fmt.Fprintf(&b, " value=%v", s.Value)
	if s.HasTimestamp || s.TimestampMs != 0 {
		fmt.Fprintf(&b, " timestampMs=%d hasTimestamp=%v", s.TimestampMs, s.HasTimestamp)
	}
	return b.String()
}
