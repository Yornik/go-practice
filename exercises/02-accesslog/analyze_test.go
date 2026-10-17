package accesslog_test

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	accesslog "github.com/yornik/go-practice/exercises/02-accesslog"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// logLine prefixes a fixed timestamp:
// logLine("GET /users/1 200 12") is "2026-10-16T12:00:00Z GET /users/1 200 12".
func logLine(rest string) string {
	return "2026-10-16T12:00:00Z " + rest
}

// logLines applies logLine to every argument.
func logLines(rests ...string) []string {
	out := make([]string, len(rests))
	for i, r := range rests {
		out[i] = logLine(r)
	}
	return out
}

// repeat returns n copies of logLine(rest).
func repeat(n int, rest string) []string {
	return slices.Repeat([]string{logLine(rest)}, n)
}

// dump formats lines with their 1-based numbers for failure messages.
func dump(lines []string) string {
	if len(lines) > 20 {
		return fmt.Sprintf("  (%d lines)\n", len(lines))
	}
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "  %2d  %q\n", i+1, l)
	}
	return b.String()
}

// malformedLines returns the line numbers in r.Malformed. It fails the test
// if an entry has a nil Err.
func malformedLines(t *testing.T, r accesslog.Report) []int {
	t.Helper()
	var nums []int
	for _, le := range r.Malformed {
		if le.Err == nil {
			t.Errorf("Malformed entry for line %d has a nil Err, want an error that says what is wrong", le.Line)
		}
		nums = append(nums, le.Line)
	}
	return nums
}

// checkLines checks Total and the malformed line numbers of a report.
func checkLines(t *testing.T, lines []string, r accesslog.Report, wantTotal int, wantMalformed []int) {
	t.Helper()
	if r.Total != wantTotal {
		t.Errorf("Analyze(lines).Total = %d, want %d\nlines:\n%s", r.Total, wantTotal, dump(lines))
	}
	if got := malformedLines(t, r); !slices.Equal(got, wantMalformed) {
		t.Errorf("Analyze(lines).Malformed has lines %v, want %v\nlines:\n%s", got, wantMalformed, dump(lines))
	}
}

var classes = []string{"1xx", "2xx", "3xx", "4xx", "5xx"}

// checkByClass compares the 5 classes. A class with 0 lines may be absent.
func checkByClass(t *testing.T, got, want map[string]int) {
	t.Helper()
	for k := range got {
		if !slices.Contains(classes, k) {
			t.Errorf("ByClass has the key %q, want only %v", k, classes)
		}
	}
	for _, c := range classes {
		if got[c] != want[c] {
			t.Errorf("ByClass[%q] = %d, want %d (got %v, want %v)", c, got[c], want[c], got, want)
		}
	}
}

func sameStats(a, b accesslog.RouteStats) bool {
	return a.Method == b.Method && a.Route == b.Route &&
		a.Requests == b.Requests && a.Client4xx == b.Client4xx && a.Server5xx == b.Server5xx &&
		a.MaxDurationMs == b.MaxDurationMs &&
		math.Abs(a.ErrorRate-b.ErrorRate) < 1e-9
}

func formatRoutes(rs []accesslog.RouteStats) string {
	if len(rs) == 0 {
		return "  (none)\n"
	}
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "  %-7s %-32s requests=%d 4xx=%d 5xx=%d rate=%.4f max=%dms\n",
			r.Method, r.Route, r.Requests, r.Client4xx, r.Server5xx, r.ErrorRate, r.MaxDurationMs)
	}
	return b.String()
}

// checkRoutes compares got and want entry by entry, in order. ErrorRate is
// compared with a tolerance of 1e-9.
func checkRoutes(t *testing.T, lines []string, got, want []accesslog.RouteStats) {
	t.Helper()
	if len(got) == len(want) {
		same := true
		for i := range got {
			same = same && sameStats(got[i], want[i])
		}
		if same {
			return
		}
	}
	t.Errorf("Analyze(lines).Routes:\n got:\n%s want:\n%slines:\n%s", formatRoutes(got), formatRoutes(want), dump(lines))
}

// byKey returns a copy of rs sorted by Method, then Route. The counting tests
// use it, so they pass before you implement the ranking.
func byKey(rs []accesslog.RouteStats) []accesslog.RouteStats {
	out := slices.Clone(rs)
	slices.SortFunc(out, func(a, b accesslog.RouteStats) int {
		return cmp.Or(cmp.Compare(a.Method, b.Method), cmp.Compare(a.Route, b.Route))
	})
	return out
}

// order returns "METHOD route" for every entry, in order.
func order(rs []accesslog.RouteStats) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Method + " " + r.Route
	}
	return out
}

// ---------------------------------------------------------------------------
// Step 2: reading lines (Total and Malformed)
// ---------------------------------------------------------------------------

func TestAnalyzeLinesValid(t *testing.T) {
	cases := []struct{ name, line string }{
		{"typical line", logLine("GET /users/1 200 12")},
		{"fractional seconds and an offset", "2026-10-16T14:00:00.250+02:00 GET /users/1 200 12"},
		{"lowest status 100", logLine("GET /upload 100 1")},
		{"highest status 599", logLine("GET /x 599 1")},
		{"zero duration", logLine("GET /x 200 0")},
		{"any method is fine", logLine("PURGE /cache/home 200 3")},
		{"spaces and tabs between fields", "  2026-10-16T12:00:00Z \t GET   /x\t200  12  "},
		{"Windows line ending", logLine("GET /x 200 12") + "\r"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{tc.line}
			checkLines(t, lines, accesslog.Analyze(lines, 0), 1, nil)
		})
	}
}

func TestAnalyzeLinesMalformed(t *testing.T) {
	cases := []struct{ name, line string }{
		{"4 fields: no duration", logLine("GET /x 200")},
		{"4 fields: no time", "GET /x 200 12"},
		{"6 fields: an extra field", logLine("GET /x 200 12 curl/8.4.0")},
		{"time is a word", "yesterday GET /x 200 12"},
		{"time without a zone", "2026-10-16T12:00:00 GET /x 200 12"},
		{"date without a time", "2026-10-16 GET /x 200 12"},
		{"path and status swapped", logLine("GET 200 /x 12")},
		{"status is a word", logLine("GET /x OK 12")},
		{"status with a letter O", logLine("GET /x 2OO 12")},
		// Envoy logs status 0 when no response was sent at all.
		{"status 0", logLine("GET /x 0 12")},
		{"status 99", logLine("GET /x 99 12")},
		{"status 600", logLine("GET /x 600 12")},
		{"duration with a unit", logLine("GET /x 200 12ms")},
		{"duration with decimals", logLine("GET /x 200 12.5")},
		{"negative duration", logLine("GET /x 200 -5")},
		// nginx writes "-" for a value it does not know.
		{"duration is a dash", logLine("GET /x 200 -")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The bad line sits between 2 good ones: it is recorded, and
			// the analysis goes on.
			lines := []string{logLine("GET /a 200 1"), tc.line, logLine("GET /b 200 1")}
			checkLines(t, lines, accesslog.Analyze(lines, 0), 2, []int{2})
		})
	}
}

func TestAnalyzeLinesNumbering(t *testing.T) {
	// Line numbers are positions in the input, so empty lines count too:
	// the numbers must match what `sed -n 4p access.log` shows.
	lines := []string{
		"",                            // 1: empty, skipped
		logLine("GET /a 200 1"),       // 2
		"   \t ",                      // 3: whitespace only, skipped
		logLine("GET /a 200 fast"),    // 4: malformed
		logLine("GET /a 500 1"),       // 5
		"2026-10-16T12:00:00Z GET /a", // 6: malformed
		"",                            // 7: what a trailing newline leaves after a split
	}
	checkLines(t, lines, accesslog.Analyze(lines, 0), 2, []int{4, 6})
}

func TestAnalyzeLinesNothingValid(t *testing.T) {
	cases := []struct {
		name          string
		lines         []string
		wantMalformed []int
	}{
		{"nil", nil, nil},
		{"empty slice", []string{}, nil},
		{"blank lines", []string{"", " ", "\t", "\r"}, nil},
		{"only malformed lines", []string{"x", "y z", logLine("GET /x 600 1")}, []int{1, 2, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := accesslog.Analyze(tc.lines, 0)
			checkLines(t, tc.lines, r, 0, tc.wantMalformed)
			if len(r.Routes) != 0 {
				t.Errorf("Analyze(lines).Routes =\n%s want none", formatRoutes(r.Routes))
			}
			checkByClass(t, r.ByClass, nil)
		})
	}
}

// ---------------------------------------------------------------------------
// Step 3: counting (Routes, ByClass). These tests ignore the order of Routes.
// ---------------------------------------------------------------------------

func TestAnalyzeCountsGrouping(t *testing.T) {
	lines := logLines(
		"GET /users/1 200 10",
		"GET /users/2?expand=orders 200 20",
		"GET /users//3/ 200 30",
		"DELETE /users/4 204 40",
		"GET /users/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f 200 50",
		"GET /users/me 200 60",
		"GET /users/1 200 70",
	)
	want := []accesslog.RouteStats{
		{Method: "GET", Route: "/users/:id", Requests: 4, MaxDurationMs: 70},
		{Method: "DELETE", Route: "/users/:id", Requests: 1, MaxDurationMs: 40},
		{Method: "GET", Route: "/users/:uuid", Requests: 1, MaxDurationMs: 50},
		{Method: "GET", Route: "/users/me", Requests: 1, MaxDurationMs: 60},
	}
	r := accesslog.Analyze(lines, 0)
	checkRoutes(t, lines, byKey(r.Routes), byKey(want))
}

func TestAnalyzeCountsPerRoute(t *testing.T) {
	lines := logLines(
		"GET /users/1 200 12",
		"GET /users/2 500 340",
		"GET /users/3 404 8",
		"GET /users/5 429 15",
		"POST /orders 201 95",
		"POST /orders 502 1200",
		"POST /orders 504 30000",
		"GET /search 400 3", // only 4xx: the ErrorRate stays 0
		"GET /search 404 4",
		"GET /healthz 200 1",
	)
	want := []accesslog.RouteStats{
		{Method: "GET", Route: "/users/:id", Requests: 4, Client4xx: 2, Server5xx: 1, ErrorRate: 0.25, MaxDurationMs: 340},
		{Method: "POST", Route: "/orders", Requests: 3, Client4xx: 0, Server5xx: 2, ErrorRate: 2.0 / 3.0, MaxDurationMs: 30000},
		{Method: "GET", Route: "/search", Requests: 2, Client4xx: 2, Server5xx: 0, ErrorRate: 0, MaxDurationMs: 4},
		{Method: "GET", Route: "/healthz", Requests: 1, Client4xx: 0, Server5xx: 0, ErrorRate: 0, MaxDurationMs: 1},
	}
	r := accesslog.Analyze(lines, 0)
	checkRoutes(t, lines, byKey(r.Routes), byKey(want))
}

func TestAnalyzeCountsStatusBoundaries(t *testing.T) {
	lines := logLines(
		"GET /a 399 1", // 3xx: neither 4xx nor 5xx
		"GET /a 400 1",
		"GET /a 499 1",
		"GET /a 500 1",
		"GET /a 599 1",
	)
	want := []accesslog.RouteStats{
		{Method: "GET", Route: "/a", Requests: 5, Client4xx: 2, Server5xx: 2, ErrorRate: 0.4, MaxDurationMs: 1},
	}
	r := accesslog.Analyze(lines, 0)
	checkRoutes(t, lines, byKey(r.Routes), want)
}

func TestAnalyzeCountsByClass(t *testing.T) {
	lines := logLines(
		"GET /ws 101 5",
		"GET /a 200 1",
		"POST /a 201 1",
		"OPTIONS /a 204 1",
		"GET /old 301 1",
		"GET /static/app.js 304 1",
		"GET /a 404 1",
		"GET /a 429 1",
		"GET /a 499 1",
		"GET /a 500 1",
		"GET /a 503 1",
		"GET /a 599 1",
	)
	r := accesslog.Analyze(lines, 0)
	checkLines(t, lines, r, 12, nil)
	checkByClass(t, r.ByClass, map[string]int{"1xx": 1, "2xx": 3, "3xx": 2, "4xx": 3, "5xx": 3})
}

func TestAnalyzeCountsSkipMalformed(t *testing.T) {
	// Malformed lines must not leak into any count, not even partly.
	lines := logLines(
		"GET /users/1 200 12",
		"GET /users/2 600 99999", // status out of range
		"GET /users/3 500 12ms",  // bad duration
		"GET /broken 503 oops",   // bad duration, would be a new route
	)
	r := accesslog.Analyze(lines, 0)
	checkLines(t, lines, r, 1, []int{2, 3, 4})
	checkByClass(t, r.ByClass, map[string]int{"2xx": 1})
	want := []accesslog.RouteStats{
		{Method: "GET", Route: "/users/:id", Requests: 1, MaxDurationMs: 12},
	}
	checkRoutes(t, lines, byKey(r.Routes), want)
}

// ---------------------------------------------------------------------------
// Step 4: ranking (order of Routes and top)
// ---------------------------------------------------------------------------

func TestAnalyzeRankByServerErrors(t *testing.T) {
	// The route with the most 5xx comes first, not the one with the highest
	// ErrorRate: 3 failures hurt more users than 2. 4xx do not count.
	var lines []string
	lines = append(lines, repeat(97, "GET /api/search 200 20")...)
	lines = append(lines, repeat(3, "GET /api/search 500 20")...)
	lines = append(lines, repeat(2, "POST /api/coupons/7/redeem 500 20")...)
	lines = append(lines, repeat(40, "GET /api/users/1 404 20")...)
	lines = append(lines, repeat(10, "GET /api/users/1 200 20")...)
	lines = append(lines, repeat(1, "GET /api/legacy 503 20")...)

	r := accesslog.Analyze(lines, 0)
	want := []string{
		"GET /api/search",              // 3 x 5xx, ErrorRate 0.03
		"POST /api/coupons/:id/redeem", // 2 x 5xx, ErrorRate 1.0
		"GET /api/legacy",              // 1 x 5xx, ErrorRate 1.0
		"GET /api/users/:id",           // 0 x 5xx, 40 x 4xx
	}
	if got := order(r.Routes); !slices.Equal(got, want) {
		t.Errorf("Analyze(lines).Routes order:\n got  %q\n want %q\nroutes:\n%s", got, want, formatRoutes(r.Routes))
	}
}

func TestAnalyzeRankTieBreaks(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  []string
	}{
		{
			name: "same 5xx: more requests first",
			lines: slices.Concat(
				logLines("GET /a 500 1"), repeat(9, "GET /a 200 1"), // 10 requests
				logLines("GET /b 500 1"), repeat(19, "GET /b 200 1"), // 20 requests
			),
			want: []string{"GET /b", "GET /a"},
		},
		{
			name:  "same 5xx and requests: route A to Z",
			lines: logLines("GET /zeta 503 1", "GET /mid 503 1", "GET /alpha 503 1"),
			want:  []string{"GET /alpha", "GET /mid", "GET /zeta"},
		},
		{
			name:  "same 5xx, requests and route: method A to Z",
			lines: logLines("PUT /items 500 1", "POST /items 500 1", "GET /items 500 1", "DELETE /items 500 1"),
			want:  []string{"DELETE /items", "GET /items", "POST /items", "PUT /items"},
		},
		{
			// Every rule at once. The input order is the reverse of the
			// wanted order inside each tie, so a missing tie-break shows.
			name: "all rules together",
			lines: logLines(
				"PUT /zeta 200 1", "GET /zeta 200 1",
				"POST /alpha 200 1", "GET /alpha 200 1",
				"GET /mid 200 1", "DELETE /mid 200 1",
				"PATCH /omega 500 1",
				"GET /beta 500 1", "GET /beta 200 1",
			),
			want: []string{
				"GET /beta", "PATCH /omega",
				"GET /alpha", "POST /alpha", "DELETE /mid", "GET /mid", "GET /zeta", "PUT /zeta",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Map iteration order changes from run to run. Several runs make
			// an incomplete sort fail here instead of now and then.
			for range 10 {
				r := accesslog.Analyze(tc.lines, 0)
				if got := order(r.Routes); !slices.Equal(got, tc.want) {
					t.Fatalf("Analyze(lines).Routes order:\n got  %q\n want %q\nlines:\n%s", got, tc.want, dump(tc.lines))
				}
			}
		})
	}
}

func TestAnalyzeRankTop(t *testing.T) {
	// 5 routes with 1, 2, 3, 4 and 5 server errors, plus 1 malformed line.
	lines := slices.Concat(
		repeat(1, "GET /e 500 1"),
		repeat(2, "GET /d 500 1"),
		repeat(3, "GET /c 500 1"),
		repeat(4, "GET /b 500 1"),
		repeat(5, "GET /a 500 1"),
		[]string{"garbage"},
	)
	all := []string{"GET /a", "GET /b", "GET /c", "GET /d", "GET /e"}
	cases := []struct {
		top  int
		want []string
	}{
		{1, all[:1]},
		{2, all[:2]},
		{4, all[:4]},
		{5, all},
		{6, all},  // more than there are: all of them
		{0, all},  // 0 means all
		{-1, all}, // so does any negative number
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("top=%d", tc.top), func(t *testing.T) {
			for range 5 {
				r := accesslog.Analyze(lines, tc.top)
				if got := order(r.Routes); !slices.Equal(got, tc.want) {
					t.Fatalf("Analyze(lines, %d).Routes order:\n got  %q\n want %q", tc.top, got, tc.want)
				}
				// top cuts Routes only. Total, ByClass and Malformed
				// still describe the whole input.
				checkLines(t, lines, r, 15, []int{16})
				checkByClass(t, r.ByClass, map[string]int{"5xx": 15})
				if t.Failed() {
					return // report each problem once, not once per run
				}
			}
		})
	}
}
