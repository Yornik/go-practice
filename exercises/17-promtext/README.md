# 17 · Parse Prometheus metrics and compute rates

**Tier 4 · about 3.5 h** · Go: a hand-written line lexer, `bufio.Scanner`, `strconv` with NaN and ±Inf, canonical map keys and escaping, `io.Writer` errors, round-trip and fuzz tests · Problem solving: turn a format spec into a grammar; define "the same series" precisely; use a round trip as a test oracle

Since the last deploy, Grafana shows 0 orders per second for the checkout service, but customers
are buying things. Either the app stopped counting, or Prometheus stopped scraping it (a relabel
rule, a wrong port, a NetworkPolicy). You take Prometheus out of the loop: `kubectl port-forward`
to the pod, `curl /metrics` twice, 15 s apart, and compare. By eye that works for 5 lines, not for
400 lines with 12-bucket histograms and counters that drop to 0 when the pod restarts. So you
write the tool: parse the Prometheus text exposition format, turn two scrapes into per-second
rates the way PromQL does, and write metrics back in a canonical form that you can diff.

## Task

Implement the stubs in `promtext.go`, `write.go` and `rates.go`. The types and
`ParseError.Error` are given.

```go
type Label struct{ Name, Value string }
type Sample struct {
	Name         string  // name on the line, suffix included: "x_bucket"
	Labels       []Label // sorted by Name
	Value        float64
	TimestampMs  int64 // 0 when HasTimestamp is false
	HasTimestamp bool
}
type Family struct {
	Name    string
	Help    string   // unescaped; "" without a HELP line
	Type    string   // counter, gauge, histogram, summary or untyped
	Samples []Sample // in input order
}
type ParseError struct{ Line int; Msg string } // Error() is given

func SeriesKey(name string, labels []Label) string  // `name{a="1",b="2"}`, labels sorted
func Parse(r io.Reader) ([]Family, error)            // families in order of first appearance
func Write(w io.Writer, fams []Family) error         // canonical text; Parse(Write(x)) == x
func Rates(prev, curr []Family, elapsed time.Duration) (map[string]float64, error) // SeriesKey → per second
```

Suggested order: `SeriesKey`, `Parse` (sample lines, then families, then errors), `Write`, `Rates`.

## Rules

### SeriesKey

- The name, then the labels sorted by name as `name="value"`, joined by `,` (no spaces), in
  `{}`. No labels (nil or empty): just the name, `up`.
- In a value, `\` becomes `\\`, `"` becomes `\"`, a newline becomes `\n`. Every other byte (a tab,
  UTF-8) is written as is. `strconv.Quote` does more, so it does not fit.
- `SeriesKey` does not modify `labels`, also when they are unsorted.

### Parse: lines

- Lines are numbered from 1, counting every line: blank lines and comments too.
- Whitespace is a space or a tab. Leading and trailing whitespace of a line is ignored. Blank
  lines are ignored.
- A line whose first non-whitespace byte is `#` is a comment. If the first word after `#` is
  `HELP` or `TYPE`, it is a HELP or TYPE line; every other comment is ignored. All other lines
  are sample lines.

### Parse: sample lines

```text
sample = metric_name [ "{" [ label { "," label } [ "," ] ] "}" ] value [ timestamp ]
label  = label_name "=" '"' label_value '"'
```

- Any number of spaces and tabs may appear between tokens: around `{`, `}`, `=`, `,`, and before
  the value and the timestamp.
- A metric name matches `[a-zA-Z_:][a-zA-Z0-9_:]*`, a label name `[a-zA-Z_][a-zA-Z0-9_]*`.
- A label value is in double quotes. `\"` is `"`, `\\` is `\`, `\n` is a newline (so `"C:\\"`
  is `C:\`); any other byte after `\` is an error. All other bytes are kept, including `,` `}`
  `=`, spaces and UTF-8.
- `{}` and a trailing comma (`{a="1",}`) are fine. An empty value (`job=""`) is kept as a label.
  The same label name twice in one sample is an error. `Labels` is sorted by name.
- The value is what `strconv.ParseFloat(s, 64)` accepts: also `NaN`, `+Inf`, `-Inf`, `2.5e+07`.
- The optional timestamp is a base-10 `int64` (`strconv.ParseInt`): milliseconds since the
  epoch. It sets `TimestampMs` and `HasTimestamp`. Nothing may follow it.

### Parse: HELP, TYPE and families

- `# HELP <name> <text>`: the text is the rest of the line after the name and the whitespace
  after it. `\\` is `\` and `\n` is a newline; any other byte after `\`, or a `\` at the end, is an
  error. The text may be empty.
- `# TYPE <name> <type>`: the type is exactly `counter`, `gauge`, `histogram`, `summary` or
  `untyped`.
- The name must be a valid metric name. A HELP or TYPE line creates the family if it does not
  exist yet, so a family can have no samples.
- At most 1 HELP and 1 TYPE line per family, in any order, before the family's first sample.
  `# TYPE x untyped` counts as a TYPE line.

A sample with name `n` belongs to the first family that matches:

| # | Rule | Example |
|---|---|---|
| 1 | a family named exactly `n` exists | `up` → `up`; `rpc_seconds{quantile="0.99"}` → `rpc_seconds` |
| 2 | `n` ends in `_bucket`, `_sum` or `_count`, and the family named `n` without the suffix is a **histogram** | `req_seconds_bucket` → `req_seconds` |
| 3 | `n` ends in `_sum` or `_count`, and the family without the suffix is a **summary** | `rpc_seconds_count` → `rpc_seconds` |
| 4 | otherwise a new family named `n`, type `untyped` | `rpc_seconds_sum` without a TYPE line for `rpc_seconds` |

- Families come in the order they first appear (HELP, TYPE or sample line), samples in input
  order. Without HELP, `Help` is `""`; without TYPE, `Type` is `untyped`.
- The format wants the lines of a family together. This parser is lenient: samples of a family
  that come after lines of another family are still added to the existing family.
- A series (name plus label set, in any label order) may appear only once in the input.
- `Parse` does not check histogram or summary contents (`le` order, the `+Inf` bucket).

### Parse: errors

On malformed input `Parse` returns **nil** families and a `*ParseError` whose `Line` is the first
bad line. `Msg` is yours; the tests do not check it. If reading `r` fails, `Parse` returns nil and
an error that wraps the reader's error (`errors.Is` works).

| Problem | Example (the bad line) |
|---|---|
| Invalid metric name | `1up 1`, `http-requests_total 5`, `# TYPE 1up gauge` |
| Missing or invalid value | `up`, `up{job="api"}`, `up one` |
| Invalid timestamp, or text after it | `up 1 1729080000.5`, `up 1 1729080000000 extra` |
| Label set or label value not closed | `up{job="api" 1`, `up{job="api} 1`, `up{path="C:\` |
| Bad label syntax | `up{job=api} 1`, `up{1job="x"} 1`, `up{job "x"} 1`, `up{a="1" b="2"} 1` |
| Unknown escape, or a `\` at the end of HELP | `up{path="C:\temp"} 1`, `# HELP up Path C:\temp`, `# HELP up Path C:\` |
| Same label name twice | `up{a="1",a="2"} 1` |
| Duplicate series | `x{b="2",a="1"} 1` after `x{a="1",b="2"} 1` |
| HELP or TYPE without a name; missing or unknown type | `# HELP`, `# TYPE up`, `# TYPE up countr` |
| Second HELP or second TYPE for a family | `# TYPE up gauge` after `# TYPE up untyped` |
| HELP or TYPE after the family's samples | `# TYPE up gauge` after `up 1` |

### Write

- For each family, in order: `# HELP <name> <text>` only if `Help` is not empty (`\` written as
  `\\`, a newline as `\n`, a `"` as is); then `# TYPE <name> <type>`, always, also for
  `untyped`; then one line per sample: `SeriesKey(name, labels)`, a space, the value, and if
  `HasTimestamp` a space and the timestamp.
- Every line ends with `\n`. Nothing else is written: no comments, no blank lines.
- Values look like `strconv.FormatFloat(v, 'g', -1, 64)`: `1027`, `-3.5`, `1.234567e+06`,
  `2.5e-05`, `NaN`, `+Inf`, `-Inf`, the shortest text that parses back to the same float.
- `Write` does not modify its input. It returns an error that wraps the first error from `w`
  (`errors.Is` works). With no families it writes nothing.
- Round trip: for families from `Parse`, `Parse(Write(fams))` equals `fams`. For text that is
  already canonical, `Write(Parse(text))` is the same text, byte for byte.

### Rates

- `elapsed <= 0` returns a nil map and an error.
- **Cumulative** samples of `curr` (they only go up, until a restart): every sample of a
  `counter` family; `<family>_bucket`, `_sum` and `_count` of a `histogram`; `<family>_sum` and
  `_count` of a `summary`. Everything else (gauges, untyped, quantiles) is skipped.
- Series are matched by `SeriesKey`, so label order does not matter. A series of `curr` that is
  not in `prev` is skipped; one only in `prev` is not in the result.
- `increase = curr - prev`, but if `curr < prev` the counter was reset (the process restarted and
  began again at 0) and `increase = curr`. Rate = `increase / elapsed.Seconds()`,
  as PromQL's `irate()` does with its last 2 samples (`rate()` also extrapolates; not here).
- The result maps `SeriesKey` to the rate. An unchanged series is in it with rate 0. Timestamps on
  the sample lines are ignored.

## Examples

```text
# HELP http_requests_total Requests handled.
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 1027
http_requests_total{code="500",method="GET"} 3 1729080000000

Parse → [{Name: "http_requests_total", Help: "Requests handled.", Type: "counter", Samples: [
          {http_requests_total [{code 200} {method GET}] 1027},
          {http_requests_total [{code 500} {method GET}] 3, TimestampMs: 1729080000000, HasTimestamp: true}]}]
Write → the same 4 lines, with the labels of line 3 sorted: {code="200",method="GET"}

SeriesKey("log_total", [{msg `say "hi"` + newline}]) → log_total{msg="say \"hi\"\n"}
Parse("up 1\n# TYPE up gauge\n")                     → nil, *ParseError{Line: 2}
```

```text
prev (t=0):    # TYPE jobs_total counter        curr (t=15s):  # TYPE jobs_total counter
               jobs_total{queue="a"} 100                       jobs_total{queue="a"} 130
               jobs_total{queue="b"} 900                       jobs_total{queue="b"} 45   ← restart
                                                               jobs_total{queue="c"} 7    ← new

Rates(prev, curr, 15*time.Second) → {`jobs_total{queue="a"}`: 2, `jobs_total{queue="b"}`: 3}
```

## Run the tests

    go test ./exercises/17-promtext/
    go test -run TestSeriesKey -v ./exercises/17-promtext/                   # step 1: SeriesKey
    go test -run TestParse -v ./exercises/17-promtext/                       # step 2: Parse
    go test -run 'TestWrite|FuzzWriteRoundTrip' -v ./exercises/17-promtext/  # step 3: Write
    go test -run TestRates -v ./exercises/17-promtext/                       # step 4: Rates
    go test -run 'TestParseErrors/duplicate' -v ./exercises/17-promtext/     # a few cases
    go test -fuzz=FuzzWriteRoundTrip ./exercises/17-promtext/   # optional: Go invents inputs, Ctrl-C to stop

A stub that panics stops the whole test binary, so run one step at a time; the 4 filters do not
overlap. Step 2 splits further into `TestParseSampleLines`, `TestParseFamilies` and
`TestParseErrors`. The Parse tests catch a panic and give up after 5 s, so a lexer that loops
forever fails with a message. Most Write and Rates tests build their input with `Parse`.
`testdata/` holds two scrapes of a checkout service, taken 15 s apart.

## Docs you can use

- [Exposition formats](https://prometheus.io/docs/instrumenting/exposition_formats/) (Prometheus), the text format section: the spec for `Parse`, minus the quoted UTF-8 names
- [Data model](https://prometheus.io/docs/concepts/data_model/) and [metric types](https://prometheus.io/docs/concepts/metric_types/) (Prometheus)
- [PromQL functions](https://prometheus.io/docs/prometheus/latest/querying/functions/): `rate()`, `irate()`, `increase()`
- [`bufio.Scanner`](https://pkg.go.dev/bufio#Scanner), [`strconv`](https://pkg.go.dev/strconv), [`strings`](https://pkg.go.dev/strings) (`Builder`, `NewReplacer`, `CutSuffix`), [`slices`](https://pkg.go.dev/slices), [`io`](https://pkg.go.dev/io)
- [`errors`](https://pkg.go.dev/errors) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`
- [Go fuzzing](https://go.dev/security/fuzz/)
- [Errors are values](https://go.dev/blog/errors-are-values) (Go blog): writers and repeated error checks

## Stretch goals

- A command: `promrate -interval 15s http://localhost:8080/metrics` scrapes twice and prints the
  20 highest rates with `text/tabwriter`. Give the `http.Client` a timeout.
- Lint like `promtool check metrics` (counters without `_total`, families without HELP), plus
  histograms whose `le` values do not increase or that have no `+Inf` bucket.
- Average latency from a histogram, `rate(_sum) / rate(_count)`, and a p99 from the bucket rates
  (see exercise 03).
- Accept OpenMetrics too: `# EOF`, `_created` samples, counter TYPE lines without `_total`.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 17 hint 1
    go run ./cmd/peek 17 hint 2
    go run ./cmd/peek 17 hint 3
    go run ./cmd/peek 17 explanation
    go run ./cmd/peek 17 walkthrough
    go run ./cmd/peek 17 solution
