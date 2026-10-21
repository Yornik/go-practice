package linefilter_test

import (
	"fmt"
	"slices"
	"testing"

	linefilter "github.com/yornik/go-practice/exercises/12-linefilter"
)

// always and never are matchers with a fixed answer.
func always(string) bool { return true }
func never(string) bool  { return false }

// lineCase is 1 line and whether the matcher must select it.
type lineCase struct {
	line string
	want bool
}

// check calls m on the lines in order and reports every wrong answer. desc
// names the matcher in the message, for example `Contains("error")`.
func check(t *testing.T, desc string, m linefilter.Matcher, cases []lineCase) {
	t.Helper()
	for _, c := range cases {
		if got := m(c.line); got != c.want {
			t.Errorf("%s(%q) = %v, want %v", desc, c.line, got, c.want)
		}
	}
}

// counted wraps m and adds 1 to *calls on every call.
func counted(m linefilter.Matcher, calls *int) linefilter.Matcher {
	return func(line string) bool {
		*calls++
		return m(line)
	}
}

// callLog records the calls of spy matchers, in order, as "name(line)".
type callLog struct{ calls []string }

// spy returns a Matcher that records its call and returns result.
func (l *callLog) spy(name string, result bool) linefilter.Matcher {
	return func(line string) bool {
		l.calls = append(l.calls, name+"("+line+")")
		return result
	}
}

// Step 1: Contains and Field.

func TestBasicContains(t *testing.T) {
	cases := []struct {
		sub, line string
		want      bool
	}{
		{"timeout", "level=error msg=timeout", true},
		{"timeout", "level=error msg=disk", false},
		{"timeout", "msg=gateway_timeout_retry", true},
		// The letter case must be the same.
		{"timeout", "msg=Timeout", false},
		{"Timeout", "msg=Timeout", true},
		{"chkpt-store-3", "store=chkpt-store-3 ms=12", true},
		{"timeout", "", false},
		// The empty string is in every string.
		{"", "level=info", true},
		{"", "", true},
	}
	for _, c := range cases {
		if got := linefilter.Contains(c.sub)(c.line); got != c.want {
			t.Errorf("Contains(%q)(%q) = %v, want %v", c.sub, c.line, got, c.want)
		}
	}
}

func TestBasicField(t *testing.T) {
	cases := []struct {
		name       string
		key, value string
		line       string
		want       bool
	}{
		{"word at the start", "level", "error", "level=error svc=gateway", true},
		{"word in the middle", "level", "error", "ts=1 level=error svc=gateway", true},
		{"word at the end", "level", "error", "svc=gateway level=error", true},
		{"the only word", "level", "error", "level=error", true},
		{"spaces around the line", "level", "error", "   level=error   ", true},
		{"tabs separate words", "level", "error", "ts=1\tlevel=error\tsvc=gateway", true},
		// strings.Fields treats \r as white space, so a Windows line ending
		// does not stick to the last word.
		{"CRLF line ending", "level", "error", "svc=gateway level=error\r\n", true},
		{"longer value", "level", "error", "svc=gateway level=errors", false},
		{"longer key", "level", "error", "svc=gateway loglevel=error", false},
		{"other letter case", "level", "error", "svc=gateway level=ERROR", false},
		{"key=value inside a longer word", "level", "error", "msg=level=error", false},
		{"other value", "level", "error", "svc=gateway level=warn", false},
		{"key and value apart", "level", "error", "level = error", false},
		{"quotes are part of the word", "level", "error", `level="error"`, false},
		{"longer host name", "host", "canary-h3-01", "host=canary-h3-012", false},
		{"exact host name", "host", "canary-h3-01", "svc=gateway host=canary-h3-01 ms=95", true},
		{"value with =", "q", "a=b", "q=a=b", true},
		{"empty line", "level", "error", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := linefilter.Field(c.key, c.value)(c.line); got != c.want {
				t.Errorf("Field(%q, %q)(%q) = %v, want %v", c.key, c.value, c.line, got, c.want)
			}
		})
	}
}

// Step 2: Not, All and Any.

func TestCombineNot(t *testing.T) {
	check(t, `Not(Contains("healthz"))`, linefilter.Not(linefilter.Contains("healthz")), []lineCase{
		{"path=/healthz ms=1", false},
		{"path=/v1/infer ms=180", true},
		{"", true},
	})

	calls := 0
	m := linefilter.Not(counted(always, &calls))
	for _, line := range []string{"a", "b", "c"} {
		m(line)
	}
	if calls != 3 {
		t.Errorf("Not(m) called m %d times for 3 lines, want 3", calls)
	}
}

func TestCombineEmpty(t *testing.T) {
	// The neutral element: All of nothing is true, Any of nothing is false,
	// as with an empty AND and an empty OR.
	check(t, "All()", linefilter.All(), []lineCase{{"level=info", true}, {"", true}})
	check(t, "Any()", linefilter.Any(), []lineCase{{"level=info", false}, {"", false}})
	var none []linefilter.Matcher
	check(t, "All(nil...)", linefilter.All(none...), []lineCase{{"level=info", true}})
	check(t, "Any(nil...)", linefilter.Any(none...), []lineCase{{"level=info", false}})
}

func TestCombineAllAny(t *testing.T) {
	gateway, errLevel := linefilter.Contains("svc=gateway"), linefilter.Field("level", "error")
	check(t, "All(gateway, error)", linefilter.All(gateway, errLevel), []lineCase{
		{"level=error svc=gateway", true},
		{"level=info svc=gateway", false},
		{"level=error svc=notify", false},
		{"level=info svc=notify", false},
	})
	check(t, "Any(gateway, error)", linefilter.Any(gateway, errLevel), []lineCase{
		{"level=error svc=gateway", true},
		{"level=info svc=gateway", true},
		{"level=error svc=notify", true},
		{"level=info svc=notify", false},
	})
	check(t, "All(gateway)", linefilter.All(gateway), []lineCase{
		{"svc=gateway", true},
		{"svc=notify", false},
	})
	check(t, "Any(gateway)", linefilter.Any(gateway), []lineCase{
		{"svc=gateway", true},
		{"svc=notify", false},
	})
}

func TestCombineShortCircuit(t *testing.T) {
	cases := []struct {
		name      string
		all       bool   // All if true, Any if false
		results   []bool // the answer of spy 1, 2, 3, ...
		want      bool
		wantCalls []string
	}{
		{"All stops at the first false", true, []bool{true, false, true}, false, []string{"m1(x)", "m2(x)"}},
		{"All calls every matcher when all are true", true, []bool{true, true, true}, true, []string{"m1(x)", "m2(x)", "m3(x)"}},
		{"All stops at once on a false first", true, []bool{false, true}, false, []string{"m1(x)"}},
		{"Any stops at the first true", false, []bool{false, true, false}, true, []string{"m1(x)", "m2(x)"}},
		{"Any calls every matcher when all are false", false, []bool{false, false, false}, false, []string{"m1(x)", "m2(x)", "m3(x)"}},
		{"Any stops at once on a true first", false, []bool{true, false}, true, []string{"m1(x)"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var log callLog
			var ms []linefilter.Matcher
			for i, r := range c.results {
				ms = append(ms, log.spy(fmt.Sprintf("m%d", i+1), r))
			}
			desc, m := "Any", linefilter.Any(ms...)
			if c.all {
				desc, m = "All", linefilter.All(ms...)
			}
			if got := m("x"); got != c.want {
				t.Errorf("%s of matchers that return %v: got %v, want %v", desc, c.results, got, c.want)
			}
			if !slices.Equal(log.calls, c.wantCalls) {
				t.Errorf("%s of matchers that return %v made the calls\n got  %q\n want %q", desc, c.results, log.calls, c.wantCalls)
			}
		})
	}
}

func TestCombineCopiesList(t *testing.T) {
	ms := []linefilter.Matcher{always, always}
	all := linefilter.All(ms...)
	ms[1] = never // must not reach the Matcher that All returned
	if !all("x") {
		t.Errorf("All(ms...) changed when the caller changed ms[1] afterwards: got false, want true")
	}

	ms = []linefilter.Matcher{never, never}
	anyOf := linefilter.Any(ms...)
	ms[0] = always
	if anyOf("x") {
		t.Errorf("Any(ms...) changed when the caller changed ms[0] afterwards: got true, want false")
	}
}

// Step 3: Limit and Sample.

func TestStatefulLimit(t *testing.T) {
	calls := 0
	m := linefilter.Limit(2, counted(linefilter.Contains("error"), &calls))
	check(t, `Limit(2, Contains("error"))`, m, []lineCase{
		{"level=info a", false},
		{"level=error a", true},
		{"level=error b", true},
		{"level=error c", false}, // the limit is reached
		{"level=info b", false},
		{"level=error d", false},
	})
	// m was needed for the first 3 lines only: after 2 matches the answer
	// is false whatever m says.
	if calls != 3 {
		t.Errorf("Limit(2, m) called m %d times for 6 lines, want 3 (m is not called after the 2nd match)", calls)
	}
}

func TestStatefulLimitZero(t *testing.T) {
	for _, n := range []int{0, -1} {
		calls := 0
		m := linefilter.Limit(n, counted(always, &calls))
		for _, line := range []string{"a", "b", "c"} {
			if m(line) {
				t.Errorf("Limit(%d, always)(%q) = true, want false", n, line)
			}
		}
		if calls != 0 {
			t.Errorf("Limit(%d, m) called m %d times, want 0", n, calls)
		}
	}
}

func TestStatefulLimitIndependent(t *testing.T) {
	inner := linefilter.Contains("error")
	a := linefilter.Limit(1, inner)
	b := linefilter.Limit(1, inner)
	check(t, "a", a, []lineCase{{"error 1", true}})
	check(t, "b", b, []lineCase{{"error 1", true}})
	check(t, "a", a, []lineCase{{"error 2", false}})
	check(t, "b", b, []lineCase{{"error 2", false}})
}

func TestStatefulLimitInsideAll(t *testing.T) {
	lines := []lineCase{
		{"svc=hushd level=error", false},
		{"svc=gateway level=error", true},
		{"svc=gateway level=error", false},
	}
	// The Limit sees only lines that passed the first matcher, so the
	// hushd line does not use up its count.
	gateway := linefilter.Contains("svc=gateway")
	check(t, `All(gateway, Limit(1, error))`, linefilter.All(gateway, linefilter.Limit(1, linefilter.Contains("level=error"))), lines)

	// Here the Limit comes first: the hushd error uses up the count, and
	// no line is selected.
	check(t, `All(Limit(1, error), gateway)`, linefilter.All(linefilter.Limit(1, linefilter.Contains("level=error")), gateway), []lineCase{
		{"svc=hushd level=error", false},
		{"svc=gateway level=error", false},
		{"svc=gateway level=error", false},
	})
}

// script returns an rnd function that returns vals in order, and a pointer
// to its number of calls. A call after the last value fails the test.
func script(t *testing.T, vals ...float64) (func() float64, *int) {
	t.Helper()
	calls := 0
	return func() float64 {
		calls++
		if calls > len(vals) {
			t.Errorf("rnd called %d times, but the test scripted only %d values", calls, len(vals))
			return 0
		}
		return vals[calls-1]
	}, &calls
}

func TestStatefulSample(t *testing.T) {
	cases := []struct {
		rate float64
		rnd  []float64
		want []bool
	}{
		{0.1, []float64{0.05, 0.1, 0.5, 0.95}, []bool{true, false, false, false}},
		{0.5, []float64{0.49, 0.5, 0, 0.99}, []bool{true, false, true, false}},
		{0.99, []float64{0.98, 0.999}, []bool{true, false}},
	}
	for _, c := range cases {
		rnd, calls := script(t, c.rnd...)
		m := linefilter.Sample(c.rate, rnd)
		for i, want := range c.want {
			line := fmt.Sprintf("line %d", i+1)
			if got := m(line); got != want {
				t.Errorf("Sample(%v) with rnd() = %v: got %v, want %v", c.rate, c.rnd[i], got, want)
			}
			if *calls != i+1 {
				t.Errorf("Sample(%v): after %d lines rnd was called %d times, want %d", c.rate, i+1, *calls, i+1)
			}
		}
	}
}

func TestStatefulSampleEdgeRates(t *testing.T) {
	cases := []struct {
		rate float64
		want bool
	}{
		{0, false},
		{-0.5, false},
		{1, true},
		{1.5, true},
	}
	for _, c := range cases {
		rnd, calls := script(t) // no values: any call fails the test
		m := linefilter.Sample(c.rate, rnd)
		for _, line := range []string{"a", "b", "c"} {
			if got := m(line); got != c.want {
				t.Errorf("Sample(%v)(%q) = %v, want %v", c.rate, line, got, c.want)
			}
		}
		if *calls != 0 {
			t.Errorf("Sample(%v) called rnd %d times, want 0", c.rate, *calls)
		}
	}
}
