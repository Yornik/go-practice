package linefilter_test

import (
	"fmt"
	"slices"
	"testing"

	linefilter "github.com/yornik/go-practice/exercises/12-linefilter"
)

var nightLines = []string{
	"level=info svc=gateway msg=stored",
	"level=error svc=gateway msg=timeout",
	"level=info path=/healthz",
	"level=error svc=hushd msg=model_timeout",
	"level=warn svc=gateway msg=slow",
	"level=error svc=gateway msg=disk_full",
}

func TestFilterKeepsOrder(t *testing.T) {
	cases := []struct {
		desc string
		m    linefilter.Matcher
		want []string
	}{
		{`Contains("level=error")`, linefilter.Contains("level=error"), []string{nightLines[1], nightLines[3], nightLines[5]}},
		{`Not(Contains("level=error"))`, linefilter.Not(linefilter.Contains("level=error")), []string{nightLines[0], nightLines[2], nightLines[4]}},
		{"always", always, nightLines},
	}
	for _, c := range cases {
		lines := slices.Clone(nightLines)
		got := linefilter.Filter(lines, c.m)
		if !slices.Equal(got, c.want) {
			t.Errorf("Filter(lines, %s)\n got  %q\n want %q", c.desc, got, c.want)
		}
	}
}

func TestFilterInputUnchanged(t *testing.T) {
	// The kept lines are not a prefix of the input, so a filter that writes
	// into the input slice would move them.
	lines := slices.Clone(nightLines)
	linefilter.Filter(lines, linefilter.Not(linefilter.Contains("level=info")))
	if !slices.Equal(lines, nightLines) {
		t.Errorf("Filter changed its input\n got  %q\n want %q", lines, nightLines)
	}
}

func TestFilterNil(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		m     linefilter.Matcher
	}{
		{"no line matches", nightLines, never},
		{"empty input", []string{}, always},
		{"nil input", nil, always},
	}
	for _, c := range cases {
		if got := linefilter.Filter(c.lines, c.m); got != nil {
			t.Errorf("%s: Filter = %#v, want nil", c.name, got)
		}
	}
}

func TestFilterCallsOncePerLine(t *testing.T) {
	var log callLog
	linefilter.Filter([]string{"a", "b", "a"}, log.spy("m", true))
	if want := []string{"m(a)", "m(b)", "m(a)"}; !slices.Equal(log.calls, want) {
		t.Errorf("Filter([a b a], m) made the calls\n got  %q\n want %q", log.calls, want)
	}
}

func TestFilterWithLimit(t *testing.T) {
	got := linefilter.Filter(nightLines, linefilter.Limit(2, linefilter.Contains("level=error")))
	want := []string{nightLines[1], nightLines[3]}
	if !slices.Equal(got, want) {
		t.Errorf("Filter(lines, Limit(2, Contains(\"level=error\")))\n got  %q\n want %q", got, want)
	}
}

func ExampleParse() {
	m, err := linefilter.Parse("gateway !healthz !host=canary-h3-01 level=error|level=warn")
	if err != nil {
		fmt.Println(err)
		return
	}
	lines := []string{
		"03:10:02 level=info svc=gateway msg=stats errors=0",
		"03:10:03 level=error svc=gateway host=vl-gpu-0412 msg=timeout",
		"03:10:05 level=error svc=gateway host=canary-h3-01 msg=ecc_corrected",
		"03:10:05 level=error svc=gateway path=/healthz",
		"03:10:08 level=warn svc=gateway host=vl-gpu-0448 msg=slow",
		"03:10:10 level=error svc=gateway host=vl-gpu-0469 msg=disk_full",
	}
	// Limit(2, m) is the head -2 of the pipeline.
	for _, line := range linefilter.Filter(lines, linefilter.Limit(2, m)) {
		fmt.Println(line)
	}
	// Output:
	// 03:10:03 level=error svc=gateway host=vl-gpu-0412 msg=timeout
	// 03:10:08 level=warn svc=gateway host=vl-gpu-0448 msg=slow
}
