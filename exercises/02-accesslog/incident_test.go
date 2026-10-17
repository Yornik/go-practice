package accesslog_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	accesslog "github.com/yornik/go-practice/exercises/02-accesslog"
)

// TestIncidentLog runs Analyze on 43 lines from a checkout service during an
// incident. The last line was cut off when the log was copied.
func TestIncidentLog(t *testing.T) {
	data, err := os.ReadFile("testdata/incident.log")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")

	r := accesslog.Analyze(lines, 3)
	// Line 25 has status 0 (no response was sent), line 43 is cut off.
	checkLines(t, lines, r, 41, []int{25, 43})
	checkByClass(t, r.ByClass, map[string]int{"1xx": 1, "2xx": 25, "3xx": 2, "4xx": 4, "5xx": 9})
	checkRoutes(t, lines, r.Routes, []accesslog.RouteStats{
		{Method: "POST", Route: "/api/v1/orders", Requests: 9, Server5xx: 5, ErrorRate: 5.0 / 9.0, MaxDurationMs: 30000},
		{Method: "GET", Route: "/api/v1/orders/:uuid", Requests: 7, Client4xx: 1, Server5xx: 2, ErrorRate: 2.0 / 7.0, MaxDurationMs: 3011},
		{Method: "GET", Route: "/api/v1/products/:id", Requests: 6, Server5xx: 1, ErrorRate: 1.0 / 6.0, MaxDurationMs: 1210},
	})

	all := accesslog.Analyze(lines, 0)
	wantOrder := []string{
		"POST /api/v1/orders",
		"GET /api/v1/orders/:uuid",
		"GET /api/v1/products/:id",
		"POST /api/v1/coupons/:uuid/redeem",
		"GET /healthz",
		"GET /api/v1/users/:id/cart",
		"GET /static/app.js",
		"OPTIONS /api/v1/orders",
		"GET /ws",
	}
	if got := order(all.Routes); !slices.Equal(got, wantOrder) {
		t.Errorf("Analyze(lines, 0).Routes order:\n got  %q\n want %q\nroutes:\n%s", got, wantOrder, formatRoutes(all.Routes))
	}
}

// The example from the README, printed as a small report.
func ExampleAnalyze() {
	lines := []string{
		"2026-10-16T12:00:00Z GET /api/v1/users/42 200 12",
		"2026-10-16T12:00:01Z GET /api/v1/users/7?expand=orders 500 340",
		"2026-10-16T12:00:01Z POST /api/v1/orders 503 1200",
		"",
		"2026-10-16T12:00:02Z GET /api/v1/users/9/ 404 8",
		"2026-10-16T12:00:02Z POST /api/v1/orders 201 95",
		"2026-10-16T12:00:03Z GET /healthz 200 1",
		"2026-10-16T12:00:03Z GET /api/v1/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f 200 30",
		"2026-10-16T12:00:04Z GET /api/v1/users/42 200 abc",
	}
	r := accesslog.Analyze(lines, 3)

	fmt.Println("valid lines:", r.Total)
	for _, class := range []string{"1xx", "2xx", "3xx", "4xx", "5xx"} {
		if n := r.ByClass[class]; n > 0 {
			fmt.Printf("  %s: %d\n", class, n)
		}
	}
	for _, rs := range r.Routes {
		fmt.Printf("%-4s %-22s requests=%d 4xx=%d 5xx=%d rate=%.2f max=%dms\n",
			rs.Method, rs.Route, rs.Requests, rs.Client4xx, rs.Server5xx, rs.ErrorRate, rs.MaxDurationMs)
	}
	for _, le := range r.Malformed {
		fmt.Println("malformed line", le.Line)
	}
	// Output:
	// valid lines: 7
	//   2xx: 4
	//   4xx: 1
	//   5xx: 2
	// GET  /api/v1/users/:id      requests=3 4xx=1 5xx=1 rate=0.33 max=340ms
	// POST /api/v1/orders         requests=2 4xx=0 5xx=1 rate=0.50 max=1200ms
	// GET  /api/v1/orders/:uuid   requests=1 4xx=0 5xx=0 rate=0.00 max=30ms
	// malformed line 9
}
