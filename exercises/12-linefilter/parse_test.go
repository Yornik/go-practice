package linefilter_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	linefilter "github.com/yornik/go-practice/exercises/12-linefilter"
)

// mustParse parses expr and stops the test on an error.
func mustParse(t *testing.T, expr string) linefilter.Matcher {
	t.Helper()
	m, err := linefilter.Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v, want nil", expr, err)
	}
	if m == nil {
		t.Fatalf("Parse(%q) returned a nil Matcher and no error", expr)
	}
	return m
}

func TestParseExpressions(t *testing.T) {
	cases := []struct {
		name  string
		expr  string
		lines []lineCase
	}{
		{"empty expression selects every line", "", []lineCase{
			{"level=info", true},
			{"", true},
		}},
		{"white space only selects every line", " \t ", []lineCase{
			{"level=info", true},
		}},
		{"1 word is a Contains", "gateway", []lineCase{
			{"svc=gateway ms=180", true},
			{"svc=notify ms=4", false},
			{"svc=Gateway", false},
		}},
		{"every term must match", "gateway timeout", []lineCase{
			{"svc=gateway msg=timeout", true},
			{"svc=gateway msg=stored", false},
			{"svc=notify msg=timeout", false},
		}},
		{"a tab separates terms", "gateway\ttimeout", []lineCase{
			{"svc=gateway msg=timeout", true},
			{"svc=gateway msg=stored", false},
		}},
		{"! negates a term", "!healthz", []lineCase{
			{"path=/healthz ms=1", false},
			{"path=/v1/infer ms=180", true},
		}},
		{"a term with = is a Field", "level=error", []lineCase{
			{"svc=gateway level=error", true},
			{"svc=gateway level=errors", false},
			{"msg=level=error", false},
		}},
		{"split at the first =", "q=a=b", []lineCase{
			{"q=a=b", true},
			{"q=a", false},
		}},
		{"negated Field", "!host=canary-h3-01", []lineCase{
			{"host=canary-h3-01 ms=95", false},
			{"host=canary-h3-012 ms=95", true},
			{"msg=canary-h3-01", true},
		}},
		{"! that is not the first byte is an ordinary byte", "wow!", []lineCase{
			{"msg=wow!", true},
			{"msg=wow", false},
		}},
		{"alternatives", "level=error|level=warn", []lineCase{
			{"level=error", true},
			{"level=warn", true},
			{"level=info", false},
		}},
		{"Contains and Field alternatives", "timeout|level=error", []lineCase{
			{"level=info msg=timeout", true},
			{"level=error msg=disk", true},
			{"level=info msg=disk", false},
		}},
		{"! applies to every alternative", "!healthz|readyz", []lineCase{
			{"path=/healthz", false},
			{"path=/readyz", false},
			{"path=/v1/infer", true},
		}},
		{"the runbook expression", "gateway !healthz !host=canary-h3-01 level=error|level=warn", []lineCase{
			{"level=error svc=gateway host=vl-gpu-0412 msg=timeout", true},
			{"level=warn svc=gateway host=vl-gpu-0448 msg=slow", true},
			{"level=info svc=gateway msg=stats errors=0", false},
			{"level=error svc=gateway host=canary-h3-01", false},
			{"level=error svc=gateway path=/healthz", false},
			{"level=error svc=hushd", false},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := mustParse(t, c.expr)
			check(t, fmt.Sprintf("Parse(%q)", c.expr), m, c.lines)
		})
	}
}

// readLines returns the lines of a file in testdata/.
func readLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// selected calls m on every line, in order, and returns the 1-based numbers
// of the lines it selects.
func selected(lines []string, m linefilter.Matcher) []int {
	var nums []int
	for i, line := range lines {
		if m(line) {
			nums = append(nums, i+1)
		}
	}
	return nums
}

func TestParseRunbook(t *testing.T) {
	lines := readLines(t, "gateway.log")
	cases := []struct {
		name string
		expr string
		want []int // line numbers in testdata/gateway.log
	}{
		{"errors", "gateway !healthz !host=canary-h3-01 level=error", []int{3, 8, 11, 14, 17, 20, 22, 25}},
		{"errors and warnings", "gateway !healthz !host=canary-h3-01 level=error|level=warn", []int{3, 7, 8, 11, 14, 16, 17, 20, 22, 25}},
		{"stats lines", "svc=gateway msg=stats", []int{2, 9, 18, 24}},
		{"canary-h3-01 only", "host=canary-h3-01", []int{4, 12, 21, 23}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := selected(lines, mustParse(t, c.expr))
			if !slices.Equal(got, c.want) {
				t.Errorf("Parse(%q) on testdata/gateway.log selects lines\n got  %v\n want %v", c.expr, got, c.want)
			}
		})
	}

	// head -5 of the runbook: a Limit around the parsed expression.
	m := linefilter.Limit(5, mustParse(t, "gateway !healthz !host=canary-h3-01 level=error"))
	if got, want := selected(lines, m), []int{3, 8, 11, 14, 17}; !slices.Equal(got, want) {
		t.Errorf("Limit(5, Parse(...)) on testdata/gateway.log selects lines\n got  %v\n want %v", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		expr string
		bad  string // the term the error must quote
	}{
		{"!", "!"},
		{"gateway !", "!"},
		{"!!x", "!!x"},
		{"a|!b", "a|!b"},
		{"a||b", "a||b"},
		{"|a", "|a"},
		{"a|", "a|"},
		{"|", "|"},
		{"!|a", "!|a"},
		{"=v", "=v"},
		{"k=", "k="},
		{"=", "="},
		{"!k=", "!k="},
		{"gateway level=error|=warn", "level=error|=warn"},
		{"gateway !healthz a||b", "a||b"},
	}
	for _, c := range cases {
		m, err := linefilter.Parse(c.expr)
		if err == nil {
			t.Errorf("Parse(%q) error = nil, want an error about the term %q", c.expr, c.bad)
			continue
		}
		if m != nil {
			t.Errorf("Parse(%q) returned a Matcher with the error, want nil", c.expr)
		}
		if quoted := fmt.Sprintf("%q", c.bad); !strings.Contains(err.Error(), quoted) {
			t.Errorf("Parse(%q) error = %q, want it to contain the term quoted with %%q: %s", c.expr, err, quoted)
		}
	}
}

func TestParseErrorsFirstBadTerm(t *testing.T) {
	expr := "gateway a||b =v"
	_, err := linefilter.Parse(expr)
	if err == nil {
		t.Fatalf("Parse(%q) error = nil, want an error about the term %q", expr, "a||b")
	}
	if !strings.Contains(err.Error(), `"a||b"`) || strings.Contains(err.Error(), `"=v"`) {
		t.Errorf("Parse(%q) error = %q, want it to name only the first bad term, %q", expr, err, "a||b")
	}
}
