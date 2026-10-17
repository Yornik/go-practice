package promtext_test

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	promtext "github.com/yornik/go-practice/exercises/17-promtext"
)

// checkRates compares two rate maps: the same keys, values equal up to a
// relative error of 1e-9 (float64 math is not exact: 0.3 - 0.1 is 0.19999999999999998).
func checkRates(t *testing.T, call string, got, want map[string]float64) {
	t.Helper()
	ok := len(got) == len(want)
	for k, w := range want {
		g, found := got[k]
		if !found || math.Abs(g-w) > 1e-9*math.Max(1, math.Abs(w)) {
			ok = false
		}
	}
	if !ok {
		t.Errorf("%s\n got:\n%s want:\n%s", call, formatRates(got), formatRates(want))
	}
}

func formatRates(m map[string]float64) string {
	if len(m) == 0 {
		return "    (empty)\n"
	}
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(m)) {
		b.WriteString("    " + k + " = " + strconv.FormatFloat(m[k], 'g', -1, 64) + "\n")
	}
	return b.String()
}

// Step 4: Rates, one rule per case. Each case is two scrapes, taken
// elapsed apart.
func TestRatesRules(t *testing.T) {
	tests := []struct {
		name       string
		prev, curr string
		elapsed    time.Duration
		want       map[string]float64
	}{
		{
			name:    "counter: increase per second",
			prev:    "# TYPE jobs_total counter\njobs_total 100\n",
			curr:    "# TYPE jobs_total counter\njobs_total 160\n",
			elapsed: time.Minute,
			want:    map[string]float64{"jobs_total": 1},
		},
		{
			name:    "the key is the SeriesKey",
			prev:    "# TYPE http_requests_total counter\nhttp_requests_total{method=\"GET\",code=\"200\"} 10\nhttp_requests_total{code=\"500\",method=\"GET\"} 4\n",
			curr:    "# TYPE http_requests_total counter\nhttp_requests_total{code=\"200\",method=\"GET\"} 40\nhttp_requests_total{code=\"500\",method=\"GET\"} 7\n",
			elapsed: 15 * time.Second,
			want: map[string]float64{
				`http_requests_total{code="200",method="GET"}`: 2,
				`http_requests_total{code="500",method="GET"}`: 0.2,
			},
		},
		{
			// An idle series is there with rate 0: "nothing happened" is
			// different from "no data".
			name:    "no increase is a rate of 0",
			prev:    "# TYPE jobs_total counter\njobs_total 100\n",
			curr:    "# TYPE jobs_total counter\njobs_total 100\n",
			elapsed: 10 * time.Second,
			want:    map[string]float64{"jobs_total": 0},
		},
		{
			// The pod restarted between the scrapes and the counter began
			// again at 0. curr - prev would be -480; the increase is 20.
			name:    "counter reset",
			prev:    "# TYPE jobs_total counter\njobs_total 500\n",
			curr:    "# TYPE jobs_total counter\njobs_total 20\n",
			elapsed: 10 * time.Second,
			want:    map[string]float64{"jobs_total": 2},
		},
		{
			name:    "a series new in curr is skipped",
			prev:    "# TYPE jobs_total counter\njobs_total{queue=\"a\"} 1\n",
			curr:    "# TYPE jobs_total counter\njobs_total{queue=\"a\"} 11\njobs_total{queue=\"b\"} 50\n",
			elapsed: 10 * time.Second,
			want:    map[string]float64{`jobs_total{queue="a"}`: 1},
		},
		{
			name:    "a series gone from curr is not in the result",
			prev:    "# TYPE jobs_total counter\njobs_total{queue=\"a\"} 1\njobs_total{queue=\"b\"} 50\n",
			curr:    "# TYPE jobs_total counter\njobs_total{queue=\"a\"} 11\n",
			elapsed: 10 * time.Second,
			want:    map[string]float64{`jobs_total{queue="a"}`: 1},
		},
		{
			name:    "gauges and untyped series are skipped",
			prev:    "# TYPE queue_length gauge\nqueue_length 3\nlegacy_jobs_processed 88\n",
			curr:    "# TYPE queue_length gauge\nqueue_length 9\nlegacy_jobs_processed 95\n",
			elapsed: 10 * time.Second,
			want:    map[string]float64{},
		},
		{
			name: "histogram: _bucket, _sum and _count",
			prev: `# TYPE req_seconds histogram
req_seconds_bucket{le="0.1"} 100
req_seconds_bucket{le="+Inf"} 120
req_seconds_sum 30
req_seconds_count 120
`,
			curr: `# TYPE req_seconds histogram
req_seconds_bucket{le="0.1"} 130
req_seconds_bucket{le="+Inf"} 160
req_seconds_sum 40
req_seconds_count 160
`,
			elapsed: 10 * time.Second,
			want: map[string]float64{
				`req_seconds_bucket{le="0.1"}`:  3,
				`req_seconds_bucket{le="+Inf"}`: 4,
				`req_seconds_sum`:               1,
				`req_seconds_count`:             4,
			},
		},
		{
			// A quantile is a gauge: the p99 going from 1.3 to 1.1 is not a
			// counter reset and has no rate.
			name: "summary: _sum and _count, not the quantiles",
			prev: `# TYPE rpc_seconds summary
rpc_seconds{quantile="0.99"} 1.3
rpc_seconds_sum 300
rpc_seconds_count 1270
`,
			curr: `# TYPE rpc_seconds summary
rpc_seconds{quantile="0.99"} 1.1
rpc_seconds_sum 303
rpc_seconds_count 1285
`,
			elapsed: 15 * time.Second,
			want:    map[string]float64{"rpc_seconds_sum": 0.2, "rpc_seconds_count": 1},
		},
		{
			// elapsed is the only clock; timestamps on the lines do not matter.
			name:    "timestamps are ignored",
			prev:    "# TYPE jobs_total counter\njobs_total 100 1729080000000\n",
			curr:    "# TYPE jobs_total counter\njobs_total 130 1729080001000\n",
			elapsed: 30 * time.Second,
			want:    map[string]float64{"jobs_total": 1},
		},
		{
			name:    "sub-second elapsed",
			prev:    "# TYPE jobs_total counter\njobs_total 100\n",
			curr:    "# TYPE jobs_total counter\njobs_total 101\n",
			elapsed: 250 * time.Millisecond,
			want:    map[string]float64{"jobs_total": 4},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev, curr := mustParse(t, tc.prev), mustParse(t, tc.curr)
			got, err := promtext.Rates(prev, curr, tc.elapsed)
			if err != nil {
				t.Fatalf("Rates(prev, curr, %v) error = %v, want nil", tc.elapsed, err)
			}
			checkRates(t, "Rates(prev, curr, "+tc.elapsed.String()+")\n prev:\n"+tc.prev+" curr:\n"+tc.curr, got, tc.want)
		})
	}
}

// Rates matches series by SeriesKey, so the order of the labels in
// hand-built families must not matter.
func TestRatesLabelOrder(t *testing.T) {
	prev := []promtext.Family{{Name: "jobs_total", Type: "counter", Samples: []promtext.Sample{
		sample("jobs_total", 10, "queue", "emails", "priority", "high"),
	}}}
	curr := []promtext.Family{{Name: "jobs_total", Type: "counter", Samples: []promtext.Sample{
		sample("jobs_total", 30, "priority", "high", "queue", "emails"),
	}}}
	got, err := promtext.Rates(prev, curr, 10*time.Second)
	if err != nil {
		t.Fatalf("Rates error = %v, want nil", err)
	}
	checkRates(t, "Rates with labels in a different order in prev and curr", got,
		map[string]float64{`jobs_total{priority="high",queue="emails"}`: 2})
}

func TestRatesBadElapsed(t *testing.T) {
	fams := []promtext.Family{{Name: "jobs_total", Type: "counter", Samples: []promtext.Sample{sample("jobs_total", 1)}}}
	for _, elapsed := range []time.Duration{0, -15 * time.Second} {
		got, err := promtext.Rates(fams, fams, elapsed)
		if err == nil {
			t.Errorf("Rates(prev, curr, %v) = %v, nil; want an error", elapsed, got)
		}
	}
}

// Two real scrapes, 15 s apart (testdata/scrape-1.prom and scrape-2.prom).
func TestRatesTwoScrapes(t *testing.T) {
	prev := mustParse(t, readTestdata(t, "scrape-1.prom"))
	curr := mustParse(t, readTestdata(t, "scrape-2.prom"))
	got, err := promtext.Rates(prev, curr, 15*time.Second)
	if err != nil {
		t.Fatalf("Rates error = %v, want nil", err)
	}
	want := map[string]float64{
		`checkout_orders_total{method="card",result="ok"}`:       2, // 1027 -> 1057
		`checkout_orders_total{method="card",result="declined"}`: 0,
		`checkout_orders_total{method="paypal",result="ok"}`:     1, // 212 -> 227
		// applepay is new in scrape 2: skipped.
		// checkout_inflight_requests and process_start_time_seconds are gauges.
		`http_request_duration_seconds_bucket{handler="/checkout",le="0.1"}`:  3,
		`http_request_duration_seconds_bucket{handler="/checkout",le="0.5"}`:  4,
		`http_request_duration_seconds_bucket{handler="/checkout",le="+Inf"}`: 5,
		`http_request_duration_seconds_sum{handler="/checkout"}`:              2.5,
		`http_request_duration_seconds_count{handler="/checkout"}`:            5,
		// The summary's quantiles are gauges; _sum and _count are counters.
		`payment_gateway_latency_seconds_sum`:   0.2,
		`payment_gateway_latency_seconds_count`: 1,
		// legacy_jobs_processed has no TYPE line, so it is untyped: skipped.
	}
	checkRates(t, "Rates(scrape-1, scrape-2, 15s)", got, want)
}
