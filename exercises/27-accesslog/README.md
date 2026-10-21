# 27 · Find the endpoints that hurt

**Tier 6 · about 1 h** · Go: maps with struct keys and pointer values, `slices.SortFunc` with `cmp.Or`, deterministic order from a map, `strings.Fields` and `strings.Cut`, `strconv.Atoi` · Problem solving: split into parse → aggregate → rank; keep cardinality bounded

Adaeze Nwosu, the CEO, checks her buzzing glucose monitor, then signs the order that pulls HUSH,
Vesper Labs' AI on-call engineer, from sale on 9 January. HUSH's 40 000 companies cannot lose their
on-call overnight, so they leave it in waves through the console, and the console's ingress 5xx rate
climbs. You have 20 minutes of access log from the ingress pods and one question: which endpoints
fail? The first idea is `awk '{print $2, $3}' | sort | uniq -c | sort -rn`. It fails, because paths
contain IDs: `/api/v1/users/7`, `/api/v1/users/42`, `/api/v1/orders/9f1c2a7e-...`. Every user and
every order becomes its own row, and the endpoint that failed 500 times is spread over 500 rows
of 1. In monitoring the same problem is called label cardinality: put the raw path in a Prometheus
label and every ID becomes a new time series. The fix is the same in both places: turn the path into
a route template (`/api/v1/users/:id`) before you group. In this exercise you write that
normalization and a report that ranks the routes by server errors.

## Task

Implement the stubs in `accesslog.go`. The 3 struct types are given.

```go
func NormalizePath(p string) string           // "/users/42/?tab=keys" → "/users/:id"

type RouteStats struct {
	Method        string
	Route         string  // NormalizePath of the path
	Requests      int
	Client4xx     int
	Server5xx     int
	ErrorRate     float64 // Server5xx / Requests
	MaxDurationMs int
}
type LineError struct {
	Line int   // 1-based line number
	Err  error // never nil
}
type Report struct {
	Total     int            // valid lines
	ByClass   map[string]int // "1xx" .. "5xx" → valid lines
	Routes    []RouteStats   // worst first, cut to top
	Malformed []LineError    // in input order
}
func Analyze(lines []string, top int) Report  // top <= 0 means all routes
```

## Rules

### NormalizePath

- Drop the query string: everything from the first `?` on.
- Split the rest on `/`. Ignore empty segments, so repeated slashes (`//`) and a trailing slash
  disappear.
- Replace a segment that consists only of the ASCII digits `0`–`9` with `:id`. Its length does not
  matter: `0`, `007` and a 25-digit ID all become `:id`. `-1`, `1.5`, `12a` and `v1` are kept.
- Replace a UUID with `:uuid`: 8-4-4-4-12 hex digits (`0`–`9`, `a`–`f`, `A`–`F`) joined by `-`,
  36 characters in all. Near misses are kept as they are: no hyphens, a wrong length, hyphens in
  other places, a non-hex letter, braces around it.
- Keep every other segment exactly as it is (letter case too).
- The result is `/` followed by the segments joined with `/`. With no segments it is `/`.

| Input | Output |
|---|---|
| `/api/v1/users/42?verbose=1` | `/api/v1/users/:id` |
| `/users/123/orders/9F1C2A7E-0B3D-4C1A-9E2F-5D6C7B8A9E0F` | `/users/:id/orders/:uuid` |
| `//api///v1//users/` | `/api/v1/users` |
| `/users/abc123` | `/users/abc123` |
| `/?utm_source=mail`, `""`, `///` | `/` |
| `users/42` | `/users/:id` |

### Analyze: reading lines

- A line has 5 fields: `<time> <method> <path> <status> <duration_ms>`, for example
  `2026-10-16T12:00:01Z GET /api/v1/users/7 500 340`.
- Fields are separated by any run of whitespace, as `strings.Fields` splits them: spaces, tabs and
  the `\r` of a Windows line ending, also before the first field and after the last.
- A line that is empty or whitespace only is skipped. It is not malformed and not counted.
- A line is **malformed** if any of these is true:
  - it does not have exactly 5 fields;
  - `time.Parse(time.RFC3339, …)` fails on the time: a zone (`Z`, `+02:00`) is required,
    fractional seconds (`12:00:00.250Z`) are fine;
  - the status is not a whole number (`strconv.Atoi`) from 100 to 599;
  - the duration is not a whole number of milliseconds (`strconv.Atoi`) or is negative. `12ms`,
    `12.5` and `-` are malformed; `0` is fine.
- The method and the path are not checked. `PURGE` is a method like any other.
- A malformed line adds a `LineError` to `Malformed` and is otherwise ignored: it is not in `Total`,
  `ByClass` or any route, not even partly. The analysis goes on with the next line.
- `LineError.Line` is 1-based: `lines[Line-1]` is the bad line. Skipped empty lines count too, so
  it matches the line number in the file. `Malformed` is in input order.
- `LineError.Err` is never nil. Its message is yours; the tests do not check it.

### Analyze: counting

- `Total` is the number of valid lines.
- `ByClass` counts valid lines per status class: `1xx` for 100–199, `2xx`, `3xx`, `4xx`, `5xx` for
  500–599. It has no other keys. A class without lines may be absent or have the value 0.
- Valid lines are grouped by method plus `NormalizePath(path)`. `GET /users/:id` and
  `DELETE /users/:id` are 2 routes. There is one `RouteStats` per group:

  | Field | Value |
  |---|---|
  | `Method`, `Route` | the group |
  | `Requests` | valid lines in the group |
  | `Client4xx` | lines with status 400–499 |
  | `Server5xx` | lines with status 500–599 |
  | `ErrorRate` | `Server5xx / Requests` as a `float64` from 0 to 1. 4xx do not count. |
  | `MaxDurationMs` | the largest duration in the group |

### Analyze: ranking

- `Routes` is sorted by:
  1. `Server5xx`, most first. The count, not `ErrorRate`: 3 failures out of 100 hurt more users
     than 2 out of 2.
  2. `Requests`, most first.
  3. `Route`, A to Z (plain string order, `<`).
  4. `Method`, A to Z.
- The same input always gives the same `Routes`, in the same order.
- If `top > 0`, `Routes` has at most `top` entries: the first ones of the sorted list. If `top <= 0`,
  it has all routes.
- `top` cuts `Routes` only. `Total`, `ByClass` and `Malformed` always describe the whole input.
- When there is nothing to report, `Routes` and `Malformed` may be nil or empty.

## Examples

```text
lines (with line numbers):
 1  2026-10-16T12:00:00Z GET /api/v1/users/42 200 12
 2  2026-10-16T12:00:01Z GET /api/v1/users/7?expand=orders 500 340
 3  2026-10-16T12:00:01Z POST /api/v1/orders 503 1200
 4
 5  2026-10-16T12:00:02Z GET /api/v1/users/9/ 404 8
 6  2026-10-16T12:00:02Z POST /api/v1/orders 201 95
 7  2026-10-16T12:00:03Z GET /healthz 200 1
 8  2026-10-16T12:00:03Z GET /api/v1/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f 200 30
 9  2026-10-16T12:00:04Z GET /api/v1/users/42 200 abc
```

```text
Analyze(lines, 3) →
  Total:     7
  ByClass:   2xx: 4, 4xx: 1, 5xx: 2
  Routes:
    GET  /api/v1/users/:id     Requests 3  4xx 1  5xx 1  ErrorRate 0.333  Max 340
    POST /api/v1/orders        Requests 2  4xx 0  5xx 1  ErrorRate 0.5    Max 1200
    GET  /api/v1/orders/:uuid  Requests 1  4xx 0  5xx 0  ErrorRate 0      Max 30
  Malformed: [{Line: 9, Err: <bad duration>}]
```

The first 2 routes both have 1 server error; the users route has more requests.
`GET /api/v1/orders/:uuid` and `GET /healthz` both have 0 errors and 1 request, so the route
decides: `/api/...` sorts before `/healthz`, and `top` 3 cuts `/healthz`. Line 4 is skipped,
line 9 is malformed, and neither is in `Total`.

## Run the tests

    go test ./exercises/27-accesslog/
    go test -run TestNormalizePath -v ./exercises/27-accesslog/    # step 1: NormalizePath
    go test -run TestAnalyzeLines -v ./exercises/27-accesslog/     # step 2: Total and Malformed
    go test -run TestAnalyzeCounts -v ./exercises/27-accesslog/    # step 3: routes and ByClass
    go test -run TestAnalyzeRank -v ./exercises/27-accesslog/      # step 4: order and top
    go test -run TestIncidentLog -v ./exercises/27-accesslog/      # step 5: a real incident log

A stub that panics stops the whole test binary, so run only the step you are on. Step 2 needs
only `Total` and `Malformed` to work (and, for an input without valid lines, no routes and no class
counts); step 3 ignores the order of `Routes`. `ExampleAnalyze` runs the example above and needs
everything.

## Docs you can use

- [`strings`](https://pkg.go.dev/strings): `Fields`, `Cut`, `Split`, `Builder`
- [`strconv.Atoi`](https://pkg.go.dev/strconv#Atoi)
- [`time.Parse`](https://pkg.go.dev/time#Parse) and [`time.RFC3339`](https://pkg.go.dev/time#pkg-constants)
- [`slices`](https://pkg.go.dev/slices): `SortFunc`
- [`cmp`](https://pkg.go.dev/cmp): `Compare`, `Or`
- [Go maps in action](https://go.dev/blog/maps) (Go blog)
- [The Go spec: Map types](https://go.dev/ref/spec#Map_types) and [Assignment statements](https://go.dev/ref/spec#Assignment_statements)
- [Prometheus: metric and label naming](https://prometheus.io/docs/practices/naming/), the section on labels

## Stretch goals

- Stream instead of loading everything: `AnalyzeReader(r io.Reader, top int) (Report, error)` with
  `bufio.Scanner`, so a 5 GB log does not need to fit in memory.
- Recognise more ID shapes: long hex hashes, ULIDs, e-mail addresses. Make the rules a list of
  functions `func(seg string) (string, bool)` so a team can add its own.
- Add `-since` and `-until`: analyse only the incident window. This is why the time must be valid.
- Count status `0` as a failure, not as a malformed line. Envoy logs `0` when no response was sent
  at all: line 25 of `testdata/incident.log` is a 30 s timeout that this report leaves out.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 27 hint 1
    go run ./cmd/peek 27 hint 2
    go run ./cmd/peek 27 hint 3
    go run ./cmd/peek 27 explanation
    go run ./cmd/peek 27 walkthrough
    go run ./cmd/peek 27 solution
