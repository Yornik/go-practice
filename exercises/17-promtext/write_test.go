package promtext_test

import (
	"bytes"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	promtext "github.com/yornik/go-practice/exercises/17-promtext"
)

// Step 1: the canonical key of a series.
func TestSeriesKey(t *testing.T) {
	tests := []struct {
		name   string
		metric string
		labels []promtext.Label
		want   string
	}{
		{"nil labels", "up", nil, `up`},
		{"empty labels", "up", []promtext.Label{}, `up`},
		{"one label", "up", labels("job", "api"), `up{job="api"}`},
		{
			"sorted labels",
			"http_requests_total", labels("code", "200", "method", "GET"),
			`http_requests_total{code="200",method="GET"}`,
		},
		{
			"unsorted labels come out sorted",
			"http_requests_total", labels("method", "GET", "path", "/api", "code", "200"),
			`http_requests_total{code="200",method="GET",path="/api"}`,
		},
		{"empty value", "up", labels("job", ""), `up{job=""}`},
		{
			// Only \ " and newline are escaped. A tab and UTF-8 stay as they
			// are; strconv.Quote would write the tab as \t.
			"escapes",
			"log_lines_total", labels("msg", "say \"hi\"\n\tC:\\temp", "city", "São Paulo"),
			"log_lines_total{city=\"São Paulo\",msg=\"say \\\"hi\\\"\\n\tC:\\\\temp\"}",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := promtext.SeriesKey(tc.metric, tc.labels); got != tc.want {
				t.Errorf("SeriesKey(%q, %q)\n got  %s\n want %s", tc.metric, tc.labels, got, tc.want)
			}
		})
	}
}

// The caller's slice is theirs. Sorting it in place is a side effect that
// shows up as a confusing bug far away.
func TestSeriesKeyDoesNotModifyLabels(t *testing.T) {
	in := labels("method", "GET", "code", "200")
	before := slices.Clone(in)
	promtext.SeriesKey("http_requests_total", in)
	if !slices.Equal(in, before) {
		t.Errorf("SeriesKey changed its labels argument from %v to %v", before, in)
	}
}

// Step 3: Write. The exact output for hand-built families.
func TestWriteFormat(t *testing.T) {
	ts := sample("http_requests_total", 3, "code", "500", "method", "GET")
	ts.TimestampMs, ts.HasTimestamp = 1729080000000, true
	fams := []promtext.Family{
		{
			Name: "http_requests_total", Type: "counter",
			Help: "Requests by code.\nSecond line with a \\ backslash and \"quotes\".",
			Samples: []promtext.Sample{
				sample("http_requests_total", 1027, "method", "GET", "code", "200"), // unsorted
				ts,
			},
		},
		{
			Name: "room_temperature_celsius", Type: "gauge",
			Samples: []promtext.Sample{
				sample("room_temperature_celsius", -3.5, "room", "say \"hi\"\nC:\\"),
				sample("room_temperature_celsius", math.NaN(), "room", "attic"),
				sample("room_temperature_celsius", math.Inf(1), "room", "oven"),
				sample("room_temperature_celsius", math.Inf(-1), "room", "freezer"),
				sample("room_temperature_celsius", 1234567, "room", "sun"),
				sample("room_temperature_celsius", 0.000025, "room", "lab"),
			},
		},
		{Name: "jobs_failed_total", Help: "Failed jobs.", Type: "counter"},
		{Name: "legacy_jobs_processed", Type: "untyped", Samples: []promtext.Sample{sample("legacy_jobs_processed", 88)}},
	}
	want := `# HELP http_requests_total Requests by code.\nSecond line with a \\ backslash and "quotes".
# TYPE http_requests_total counter
http_requests_total{code="200",method="GET"} 1027
http_requests_total{code="500",method="GET"} 3 1729080000000
# TYPE room_temperature_celsius gauge
room_temperature_celsius{room="say \"hi\"\nC:\\"} -3.5
room_temperature_celsius{room="attic"} NaN
room_temperature_celsius{room="oven"} +Inf
room_temperature_celsius{room="freezer"} -Inf
room_temperature_celsius{room="sun"} 1.234567e+06
room_temperature_celsius{room="lab"} 2.5e-05
# HELP jobs_failed_total Failed jobs.
# TYPE jobs_failed_total counter
# TYPE legacy_jobs_processed untyped
legacy_jobs_processed 88
`
	unsorted := slices.Clone(fams[0].Samples[0].Labels)

	var buf bytes.Buffer
	if err := promtext.Write(&buf, fams); err != nil {
		t.Fatalf("Write error = %v, want nil", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("Write output differs.\n got:\n%s\n want:\n%s", got, want)
		reportFirstDifference(t, got, want)
	}
	if !slices.Equal(fams[0].Samples[0].Labels, unsorted) {
		t.Errorf("Write changed the labels of its input from %v to %v", unsorted, fams[0].Samples[0].Labels)
	}
}

func TestWriteNothing(t *testing.T) {
	var buf bytes.Buffer
	if err := promtext.Write(&buf, nil); err != nil {
		t.Fatalf("Write(nil) error = %v, want nil", err)
	}
	if buf.Len() != 0 {
		t.Errorf("Write(nil) wrote %q, want nothing", buf.String())
	}
}

// Write(Parse(text)) normalizes text: comments go, labels get sorted, every
// family gets a TYPE line, numbers get their shortest form.
func TestWriteCanonicalizes(t *testing.T) {
	messy := `# Exported by a hand-written script.

  # HELP queue_jobs   Jobs waiting, per queue.
queue_jobs{queue="emails" ,  priority="high",} 1.0
queue_jobs{priority="low",queue="emails"}	1e3	1729080000000
worker_busy 0.50
`
	want := `# HELP queue_jobs Jobs waiting, per queue.
# TYPE queue_jobs untyped
queue_jobs{priority="high",queue="emails"} 1
queue_jobs{priority="low",queue="emails"} 1000 1729080000000
# TYPE worker_busy untyped
worker_busy 0.5
`
	var buf bytes.Buffer
	if err := promtext.Write(&buf, mustParse(t, messy)); err != nil {
		t.Fatalf("Write error = %v, want nil", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("Write(Parse(%q)) differs.\n got:\n%s\n want:\n%s", messy, got, want)
		reportFirstDifference(t, got, want)
	}
}

// Text that is already canonical comes back byte for byte, and parsing what
// Write wrote gives the same families. A round trip is a strong oracle: it
// needs no hand-computed expectation, only two functions that agree.
func TestWriteRoundTrip(t *testing.T) {
	canonical := `# HELP http_request_duration_seconds Latency of HTTP requests.\nIn seconds.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{handler="/checkout",le="0.1"} 2900
http_request_duration_seconds_bucket{handler="/checkout",le="+Inf"} 3200
http_request_duration_seconds_sum{handler="/checkout"} 412.5
http_request_duration_seconds_count{handler="/checkout"} 3200
# TYPE odd_values gauge
odd_values{text="tab` + "\t" + `here, quote \" backslash \\ newline \n end"} NaN 1729080000000
odd_values{text=""} -Inf
`
	for _, input := range []string{canonical, readTestdata(t, "scrape-1.prom"), readTestdata(t, "scrape-2.prom")} {
		first := mustParse(t, input)
		var buf bytes.Buffer
		if err := promtext.Write(&buf, first); err != nil {
			t.Fatalf("Write error = %v, want nil", err)
		}
		if input == canonical && buf.String() != canonical {
			t.Errorf("Write(Parse(canonical text)) is not the same text.\n got:\n%s\n want:\n%s", buf.String(), canonical)
			reportFirstDifference(t, buf.String(), canonical)
		}
		second := mustParse(t, buf.String())
		if !sameFamilies(first, second) {
			t.Errorf("Parse(Write(fams)) != fams\n fams:\n%s Write wrote:\n%s parsed back:\n%s", dump(first), buf.String(), dump(second))
		}
	}
}

// A full disk, a closed connection: Write must report it, not drop it.
func TestWriteError(t *testing.T) {
	fams := []promtext.Family{{Name: "up", Type: "gauge", Samples: []promtext.Sample{sample("up", 1)}}}
	w := &failingWriter{err: errors.New("no space left on device")}
	err := promtext.Write(w, fams)
	if !errors.Is(err, w.err) {
		t.Errorf("Write to a failing writer: error = %v, want an error that wraps %q (errors.Is)", err, w.err)
	}
}

type failingWriter struct{ err error }

func (w *failingWriter) Write([]byte) (int, error) { return 0, w.err }

// FuzzWriteRoundTrip checks, for any input that Parse accepts, that Write
// succeeds and that parsing its output gives the same families. Plain
// "go test" runs only the seeds; "go test -fuzz=FuzzWriteRoundTrip" invents
// more inputs.
func FuzzWriteRoundTrip(f *testing.F) {
	f.Add("up 1\n")
	f.Add(`# HELP x Help with \\ and \n.` + "\n# TYPE x counter\n" + `x{a="1",b="say \"hi\""} 3 1729080000000` + "\n")
	f.Add("# TYPE h histogram\nh_bucket{le=\"+Inf\"} 1\nh_sum 0.5\nh_count 1\nh 7\n")
	f.Add("a{z=\"\",y=\"\\n\",} NaN\nb -Inf\na{z=\"2\"} +Inf\n")
	f.Add("# TYPE s summary\ns_sum 1\n# TYPE s_count counter\ns_count 2\ns_bucket 3\n")
	f.Fuzz(func(t *testing.T, input string) {
		if strings.ContainsRune(input, '\r') {
			// The README leaves \r (Windows line endings) unspecified:
			// bufio.Scanner drops a \r before \n, a split on "\n" keeps it.
			t.Skip()
		}
		// Parse is called directly here: the fuzz engine catches panics
		// and hangs itself and reports the input that caused them.
		first, err := promtext.Parse(strings.NewReader(input))
		if err != nil {
			return
		}
		var buf bytes.Buffer
		if err := promtext.Write(&buf, first); err != nil {
			t.Fatalf("Write error = %v, want nil", err)
		}
		second, err := promtext.Parse(strings.NewReader(buf.String()))
		if err != nil {
			t.Fatalf("Parse(%q) accepted, but Parse(Write(...)) failed: %v\nWrite wrote:\n%s", input, err, buf.String())
		}
		if !sameFamilies(first, second) {
			t.Fatalf("Parse(Write(fams)) != fams for input %q\n fams:\n%s Write wrote:\n%s parsed back:\n%s",
				input, dump(first), buf.String(), dump(second))
		}
	})
}

// reportFirstDifference points at the first line where two texts differ, so
// a missing space or newline is easy to spot.
func reportFirstDifference(t *testing.T, got, want string) {
	t.Helper()
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range max(len(g), len(w)) {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			t.Errorf("first difference on line %d:\n got  %q\n want %q", i+1, gl, wl)
			return
		}
	}
}
