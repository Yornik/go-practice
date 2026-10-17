package promtext_test

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"testing/iotest"

	promtext "github.com/yornik/go-practice/exercises/17-promtext"
)

// Step 2a: one sample line at a time. Every input here has no HELP or TYPE
// line, so the result must be exactly one untyped family holding the sample.
func TestParseSampleLines(t *testing.T) {
	withTS := sample("up", 1)
	withTS.TimestampMs, withTS.HasTimestamp = 1729080000000, true

	tests := []struct {
		name  string
		input string
		want  promtext.Sample
	}{
		{"no labels", `up 1`, sample("up", 1)},
		{"one label", `up{job="api"} 1`, sample("up", 1, "job", "api")},
		{
			"labels are sorted by name",
			`http_requests_total{path="/api",code="200",method="GET"} 1027`,
			sample("http_requests_total", 1027, "code", "200", "method", "GET", "path", "/api"),
		},
		{"empty label set", `up{} 1`, sample("up", 1)},
		{"trailing comma", `up{job="api",instance="a:9100",} 1`, sample("up", 1, "instance", "a:9100", "job", "api")},
		{"empty label value is kept", `up{job=""} 0`, sample("up", 0, "job", "")},
		{
			"escapes in label values",
			`log_lines_total{msg="say \"hi\"",path="C:\\temp",text="one\ntwo"} 3`,
			sample("log_lines_total", 3, "msg", `say "hi"`, "path", `C:\temp`, "text", "one\ntwo"),
		},
		{
			// The value ends in an escaped backslash. The quote after \\
			// closes it, although a backslash comes right before it.
			"backslash at the end of a label value",
			`dir_size_bytes{path="C:\\"} 4096`,
			sample("dir_size_bytes", 4096, "path", `C:\`),
		},
		{
			// A split on "," or "}" breaks here.
			"commas, braces and = inside a label value",
			`queries_total{q="a=b, c={d}"} 1`,
			sample("queries_total", 1, "q", "a=b, c={d}"),
		},
		{"UTF-8 in a label value", `orders_total{city="São Paulo"} 2`, sample("orders_total", 2, "city", "São Paulo")},
		{
			"colons in a metric name (recording rules)",
			`job:http_requests:rate5m{job="api"} 0.25`,
			sample("job:http_requests:rate5m", 0.25, "job", "api"),
		},
		{"underscores and digits in names", `_x_1{_l2="v"} 1`, sample("_x_1", 1, "_l2", "v")},

		// Values: whatever strconv.ParseFloat accepts.
		{"float value", `temperature_celsius 21.5`, sample("temperature_celsius", 21.5)},
		{"negative value", `temperature_celsius -3.25`, sample("temperature_celsius", -3.25)},
		{"exponent", `process_resident_memory_bytes 2.5165824e+07`, sample("process_resident_memory_bytes", 2.5165824e+07)},
		{"NaN", `ratio NaN`, sample("ratio", math.NaN())},
		{"+Inf", `limit +Inf`, sample("limit", math.Inf(1))},
		{"-Inf", `floor -Inf`, sample("floor", math.Inf(-1))},

		{"timestamp in milliseconds", `up 1 1729080000000`, withTS},

		// Whitespace: spaces and tabs, any amount, between any two tokens.
		{"leading and trailing whitespace", " \t up 1 \t ", sample("up", 1)},
		{"tabs and repeated spaces", "up\t\t1   1729080000000", withTS},
		{"spaces inside the label set", `up{ job = "api" , instance = "a:9100" , } 1`, sample("up", 1, "instance", "a:9100", "job", "api")},
		{"space before the label set", `up {job="api"} 1`, sample("up", 1, "job", "api")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parse(t, tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v, want nil", tc.input, err)
			}
			if len(got) != 1 || got[0].Name != tc.want.Name || got[0].Type != "untyped" ||
				len(got[0].Samples) != 1 || !sameSample(got[0].Samples[0], tc.want) {
				t.Errorf("Parse(%q)\n got:\n%s want 1 untyped family %q with 1 sample:\n        %s",
					tc.input, dump(got), tc.want.Name, dumpSample(tc.want))
			}
		})
	}
}

// Step 2b: HELP and TYPE lines, and which family each sample belongs to.
func TestParseFamilies(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []promtext.Family
	}{
		{
			name: "HELP and TYPE",
			input: `# HELP http_requests_total Total HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1027
http_requests_total{code="500"} 3
`,
			want: []promtext.Family{{
				Name: "http_requests_total", Help: "Total HTTP requests.", Type: "counter",
				Samples: []promtext.Sample{
					sample("http_requests_total", 1027, "code", "200"),
					sample("http_requests_total", 3, "code", "500"),
				},
			}},
		},
		{
			name:  "TYPE before HELP",
			input: "# TYPE up gauge\n# HELP up Is the target up?\nup 1\n",
			want:  []promtext.Family{{Name: "up", Help: "Is the target up?", Type: "gauge", Samples: []promtext.Sample{sample("up", 1)}}},
		},
		{
			name:  "HELP escapes",
			input: `# HELP build_info First line.\nSecond line: C:\\temp` + "\n",
			want:  []promtext.Family{{Name: "build_info", Help: "First line.\nSecond line: C:\\temp", Type: "untyped"}},
		},
		{
			// The text starts after the whitespace that follows the name.
			// Inner whitespace is kept; trailing whitespace of a line is ignored.
			name:  "HELP text keeps inner spaces",
			input: "# HELP up   Is the   target\tup?  \n",
			want:  []promtext.Family{{Name: "up", Help: "Is the   target\tup?", Type: "untyped"}},
		},
		{
			name:  "HELP without text",
			input: "# HELP up\n# TYPE up gauge\nup 1\n",
			want:  []promtext.Family{{Name: "up", Type: "gauge", Samples: []promtext.Sample{sample("up", 1)}}},
		},
		{
			name:  "no TYPE line means untyped",
			input: `build_info{version="1.2.3"} 1`,
			want:  []promtext.Family{{Name: "build_info", Type: "untyped", Samples: []promtext.Sample{sample("build_info", 1, "version", "1.2.3")}}},
		},
		{
			name:  "family with HELP and TYPE but no samples",
			input: "# HELP jobs_failed_total Failed jobs.\n# TYPE jobs_failed_total counter\n# TYPE up gauge\nup 1\n",
			want: []promtext.Family{
				{Name: "jobs_failed_total", Help: "Failed jobs.", Type: "counter"},
				{Name: "up", Type: "gauge", Samples: []promtext.Sample{sample("up", 1)}},
			},
		},
		{
			// Order of first appearance, whether that is a HELP, a TYPE or a sample.
			name:  "families in input order",
			input: "zeta 1\n# HELP alpha First.\n# TYPE mid gauge\nmid 3\nalpha 2\n",
			want: []promtext.Family{
				{Name: "zeta", Type: "untyped", Samples: []promtext.Sample{sample("zeta", 1)}},
				{Name: "alpha", Help: "First.", Type: "untyped", Samples: []promtext.Sample{sample("alpha", 2)}},
				{Name: "mid", Type: "gauge", Samples: []promtext.Sample{sample("mid", 3)}},
			},
		},
		{
			name: "histogram: _bucket, _sum and _count belong to the family",
			input: `# HELP http_request_duration_seconds Latency.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 2900
http_request_duration_seconds_bucket{le="+Inf"} 3200
http_request_duration_seconds_sum 412.5
http_request_duration_seconds_count 3200
`,
			want: []promtext.Family{{
				Name: "http_request_duration_seconds", Help: "Latency.", Type: "histogram",
				Samples: []promtext.Sample{
					sample("http_request_duration_seconds_bucket", 2900, "le", "0.1"),
					sample("http_request_duration_seconds_bucket", 3200, "le", "+Inf"),
					sample("http_request_duration_seconds_sum", 412.5),
					sample("http_request_duration_seconds_count", 3200),
				},
			}},
		},
		{
			name: "summary: quantiles, _sum and _count belong to the family",
			input: `# TYPE rpc_duration_seconds summary
rpc_duration_seconds{quantile="0.5"} 0.21
rpc_duration_seconds{quantile="0.99"} 1.3
rpc_duration_seconds_sum 301.25
rpc_duration_seconds_count 1270
`,
			want: []promtext.Family{{
				Name: "rpc_duration_seconds", Type: "summary",
				Samples: []promtext.Sample{
					sample("rpc_duration_seconds", 0.21, "quantile", "0.5"),
					sample("rpc_duration_seconds", 1.3, "quantile", "0.99"),
					sample("rpc_duration_seconds_sum", 301.25),
					sample("rpc_duration_seconds_count", 1270),
				},
			}},
		},
		{
			// What you get from an exporter that forgot its TYPE line.
			name:  "without a TYPE line, _sum and _count are families of their own",
			input: "rpc_duration_seconds_sum 301.25\nrpc_duration_seconds_count 1270\n",
			want: []promtext.Family{
				{Name: "rpc_duration_seconds_sum", Type: "untyped", Samples: []promtext.Sample{sample("rpc_duration_seconds_sum", 301.25)}},
				{Name: "rpc_duration_seconds_count", Type: "untyped", Samples: []promtext.Sample{sample("rpc_duration_seconds_count", 1270)}},
			},
		},
		{
			name:  "a gauge does not own _count samples",
			input: "# TYPE queue_length gauge\nqueue_length 3\nqueue_length_count 1\n",
			want: []promtext.Family{
				{Name: "queue_length", Type: "gauge", Samples: []promtext.Sample{sample("queue_length", 3)}},
				{Name: "queue_length_count", Type: "untyped", Samples: []promtext.Sample{sample("queue_length_count", 1)}},
			},
		},
		{
			name:  "a summary does not own _bucket samples",
			input: "# TYPE rpc_seconds summary\nrpc_seconds_sum 2\nrpc_seconds_bucket{le=\"1\"} 1\n",
			want: []promtext.Family{
				{Name: "rpc_seconds", Type: "summary", Samples: []promtext.Sample{sample("rpc_seconds_sum", 2)}},
				{Name: "rpc_seconds_bucket", Type: "untyped", Samples: []promtext.Sample{sample("rpc_seconds_bucket", 1, "le", "1")}},
			},
		},
		{
			// The format wants the lines of a family together. A hand-written
			// exporter that loops over pods and prints every metric per pod
			// breaks that rule; this parser merges them anyway.
			name:  "samples of one family split by another family are merged",
			input: "pod_cpu_seconds_total{pod=\"a\"} 1\npod_memory_bytes{pod=\"a\"} 100\npod_cpu_seconds_total{pod=\"b\"} 2\npod_memory_bytes{pod=\"b\"} 200\n",
			want: []promtext.Family{
				{Name: "pod_cpu_seconds_total", Type: "untyped", Samples: []promtext.Sample{
					sample("pod_cpu_seconds_total", 1, "pod", "a"),
					sample("pod_cpu_seconds_total", 2, "pod", "b"),
				}},
				{Name: "pod_memory_bytes", Type: "untyped", Samples: []promtext.Sample{
					sample("pod_memory_bytes", 100, "pod", "a"),
					sample("pod_memory_bytes", 200, "pod", "b"),
				}},
			},
		},
		{
			name:  "comments and blank lines are ignored",
			input: "# Exported by checkout v1.4.2\n\n   \n  # an indented comment\n#\nup 1\n\n",
			want:  []promtext.Family{{Name: "up", Type: "untyped", Samples: []promtext.Sample{sample("up", 1)}}},
		},
		{"empty input", "", nil},
		{"only comments and blank lines", "# nothing here yet\n\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parse(t, tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v, want nil", tc.input, err)
			}
			if !sameFamilies(got, tc.want) {
				t.Errorf("Parse(%q)\n got:\n%s want:\n%s", tc.input, dump(got), dump(tc.want))
			}
		})
	}
}

// Step 2c: malformed input. Parse must return nil families and a
// *ParseError with the 1-based number of the first bad line.
func TestParseErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		line  int
	}{
		// Sample lines.
		{"metric name starts with a digit", "up 1\n1up 1\n", 2},
		{"dash in a metric name", "http-requests_total 5", 1},
		{"missing value", "up", 1},
		{"missing value after labels", `up{job="api"}`, 1},
		{"value is not a number", "up one", 1},
		{"timestamp is not an integer", "up 1 1729080000.5", 1},
		{"text after the timestamp", "up 1 1729080000000 extra", 1},

		// Label sets.
		{"label set not closed", `up{job="api" 1`, 1},
		{"label value not closed", `up{job="api} 1`, 1},
		{"line cut off after a backslash in a label value", `up{path="C:\`, 1},
		{"label value without quotes", `up{job=api} 1`, 1},
		{"label name starts with a digit", `up{1job="api"} 1`, 1},
		{"missing = after a label name", `up{job "api"} 1`, 1},
		{"missing comma between labels", `up{a="1" b="2"} 1`, 1},
		{"unknown escape in a label value", `up{path="C:\temp"} 1`, 1},
		{"same label name twice", `up{a="1",a="2"} 1`, 1},

		// Duplicate series: same name and same labels, in any label order.
		{"duplicate series", "up{job=\"a\"} 1\nup{job=\"b\"} 1\nup{job=\"a\"} 0\n", 3},
		{"duplicate series with labels in another order", "x{a=\"1\",b=\"2\"} 1\nx{b=\"2\",a=\"1\"} 1\n", 2},
		{"duplicate series without labels", "# TYPE up gauge\nup 1\nup 1\n", 3},

		// HELP and TYPE lines.
		{"TYPE with an unknown type", "# TYPE up countr", 1},
		{"TYPE without a type", "# TYPE up", 1},
		{"TYPE without a name", "# TYPE", 1},
		{"HELP without a name", "# HELP", 1},
		{"invalid metric name in TYPE", "# TYPE 1up gauge", 1},
		{"unknown escape in HELP", `# HELP up Path C:\temp`, 1},
		{"HELP text ends with a backslash", `# HELP up Path C:\`, 1},
		{"second HELP for a family", "# HELP up One.\n# TYPE up gauge\n# HELP up Two.\nup 1\n", 3},
		{"second TYPE for a family", "# TYPE up gauge\n# TYPE up gauge\n", 2},
		// "# TYPE up untyped" is a TYPE line too (Write writes such lines),
		// although Type is "untyped" with or without it.
		{"second TYPE after an untyped TYPE line", "# TYPE up untyped\n# TYPE up gauge\n", 2},
		{"TYPE after the family's samples", "up 1\n# TYPE up gauge\n", 2},
		{"HELP after the family's samples", "# TYPE up gauge\nup 1\n# HELP up Late.\n", 3},

		// Line numbers count every line: blank lines and comments too.
		{"line numbers count blank and comment lines", "# HELP up Up.\n\n# a comment\n\nup{ 1\n", 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parse(t, tc.input)
			if err == nil {
				t.Fatalf("Parse(%q) error = nil, want a *ParseError for line %d; got families:\n%s", tc.input, tc.line, dump(got))
			}
			var pe *promtext.ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("Parse(%q) error = %v (type %T), want a *promtext.ParseError", tc.input, err, err)
			}
			if pe.Line != tc.line {
				t.Errorf("Parse(%q) error line = %d, want %d (error: %v)", tc.input, pe.Line, tc.line, err)
			}
			if got != nil {
				t.Errorf("Parse(%q) returned families with the error, want nil:\n%s", tc.input, dump(got))
			}
		})
	}
}

// A scrape over the network can fail halfway. bufio.Scanner hides read errors
// until you ask for them.
func TestParseReadError(t *testing.T) {
	errReset := errors.New("connection reset by peer")
	r := io.MultiReader(strings.NewReader("# TYPE up gauge\nup 1\n"), iotest.ErrReader(errReset))

	got, err := parseFrom(t, "a reader that fails after 2 lines", r)
	if !errors.Is(err, errReset) {
		t.Fatalf("Parse(failing reader) error = %v, want an error that wraps %q (errors.Is)", err, errReset)
	}
	if got != nil {
		t.Errorf("Parse(failing reader) returned families with the error, want nil:\n%s", dump(got))
	}
}

// A whole scrape as a client library writes it.
func TestParseRealScrape(t *testing.T) {
	got, err := parse(t, readTestdata(t, "scrape-1.prom"))
	if err != nil {
		t.Fatalf("Parse(testdata/scrape-1.prom) error = %v, want nil", err)
	}
	// name, type, number of samples
	want := []string{
		"checkout_orders_total counter 3",
		"checkout_inflight_requests gauge 1",
		"http_request_duration_seconds histogram 5",
		"payment_gateway_latency_seconds summary 4",
		"process_start_time_seconds gauge 1",
		"legacy_jobs_processed untyped 1",
	}
	var summary []string
	for _, f := range got {
		summary = append(summary, fmt.Sprintf("%s %s %d", f.Name, f.Type, len(f.Samples)))
	}
	if strings.Join(summary, "\n") != strings.Join(want, "\n") {
		t.Errorf("Parse(testdata/scrape-1.prom) families (name type samples):\n got:\n    %s\n want:\n    %s",
			strings.Join(summary, "\n    "), strings.Join(want, "\n    "))
	}
	if len(got) > 0 && got[0].Help != "Orders placed, by payment method and result." {
		t.Errorf("Parse(testdata/scrape-1.prom) first family Help = %q, want %q", got[0].Help, "Orders placed, by payment method and result.")
	}
}
