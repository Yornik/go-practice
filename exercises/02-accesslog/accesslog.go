// Package accesslog summarises an HTTP access log per route, so that during
// an incident you can see which endpoints return server errors.
//
// Each log line has 5 fields:
//
//	2026-10-16T12:00:01Z GET /api/v1/users/7?expand=orders 500 340
//	<RFC 3339 time>      <method> <path>                   <status> <duration_ms>
package accesslog

// NormalizePath turns a raw request path into a route template with low
// cardinality, so that /users/1 and /users/2 count as the same route.
//
// It drops the query string, removes empty segments (from repeated or
// trailing slashes), replaces a segment of only ASCII digits with ":id" and a
// canonical UUID segment (8-4-4-4-12 hex digits) with ":uuid", and keeps every
// other segment unchanged. The result always starts with "/"; a path without
// segments gives "/".
//
//	NormalizePath("/users/123/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f?x=1")
//	// "/users/:id/orders/:uuid"
func NormalizePath(p string) string {
	panic("TODO: implement")
}

// RouteStats holds the numbers for one route: one method plus one normalized
// path.
type RouteStats struct {
	Method        string  // the method as written in the log, for example "GET"
	Route         string  // NormalizePath of the request path
	Requests      int     // valid lines with this method and route
	Client4xx     int     // requests with a status from 400 to 499
	Server5xx     int     // requests with a status from 500 to 599
	ErrorRate     float64 // Server5xx / Requests, from 0 to 1
	MaxDurationMs int     // the largest duration_ms of these requests
}

// LineError records one malformed line.
type LineError struct {
	Line int   // 1-based line number: lines[Line-1] is the malformed line
	Err  error // what is wrong with the line; never nil
}

// Report is the result of Analyze.
type Report struct {
	// Total is the number of valid lines.
	Total int
	// ByClass counts the valid lines per status class. The keys are "1xx",
	// "2xx", "3xx", "4xx" and "5xx". A class without lines may be absent or 0.
	ByClass map[string]int
	// Routes holds one entry per method and route, the worst first, cut to
	// the top entries.
	Routes []RouteStats
	// Malformed lists the malformed lines in input order.
	Malformed []LineError
}

// Analyze reads access log lines and reports which routes fail.
//
// Empty and whitespace-only lines are skipped. A malformed line is recorded
// in Report.Malformed and otherwise ignored; it does not stop the analysis.
// Valid lines are grouped by method and NormalizePath(path).
//
// Routes are sorted by Server5xx (most first), then Requests (most first),
// then Route (A to Z), then Method (A to Z). If top > 0, only the first top
// routes are returned; top <= 0 returns all of them. Total and ByClass always
// count every valid line, also those of routes cut by top.
func Analyze(lines []string, top int) Report {
	panic("TODO: implement")
}
