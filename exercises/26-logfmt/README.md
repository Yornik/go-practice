# 26 · Parse logfmt log lines

**Tier 6 · about 1.5 h** · Go: strings as bytes vs runes, `strings.Builder`, a hand-written scanner, custom error types with `errors.Is`/`errors.As`/`%w`, `iota` enums with `String()`, `time.Parse` · Problem solving: write the grammar down before you code; see why split and regex fail

"Read every line that HUSH's copies wrote since September": the second action item of the New Year
postmortem. Many services log in logfmt: one line per event, `key=value` pairs separated by spaces.
Prometheus and Alertmanager log this way by default, Go's `log/slog` `TextHandler` writes it,
Heroku's router writes it, and Grafana Loki has a `| logfmt` parser for it. During an incident you
want a small tool that reads such lines and keeps only the errors, or only the slow database calls.
Daniyar Seitkali, the CTO, wrote one in 4 minutes with `strings.Split(line, " ")`, and it broke on
the first quoted value with a space in it: `msg="db timeout"`. In this exercise you write a real
parser for one line: it handles quotes and escapes, and it tells you exactly which column is broken
when a line is malformed.

## Task

Implement the stubs in `logfmt.go`:

```go
type Pair struct{ Key, Value string }
func Parse(line string) ([]Pair, error)       // pairs in line order; *SyntaxError on bad input

type SyntaxError struct{ Col int; Msg string }
func (e *SyntaxError) Error() string          // "logfmt: column <Col>: <Msg>"

type Level int                                 // Debug, Info, Warn, Error (constants are given)
func (l Level) String() string                 // "debug", "info", "warn", "error", or "Level(N)"
func ParseLevel(s string) (Level, error)       // case-insensitive; "warning" means Warn

type Record struct {
	Time   time.Time
	Level  Level
	Msg    string
	Fields map[string]string
}
func ParseRecord(line string) (Record, error)  // ts and level required; other keys go to Fields

var ErrUnknownLevel = errors.New("unknown level")          // given
var ErrMissingField = errors.New("missing required field") // given
```

Suggested order: `Parse` and `SyntaxError` first (most of the work), then `Level`, then
`ParseRecord`.

## Rules

### Parse: the syntax

- Whitespace means a space (0x20) or a tab (0x09), and a tab works everywhere a space does.
  Nothing else is whitespace: a `\r` (from a Windows line ending) is an ordinary byte of a key
  or value. `a=x\ry` is one pair with the value `x\ry`.
- A line is a list of pairs separated by whitespace. Any amount of whitespace is allowed between
  pairs, before the first pair and after the last one.
- A pair has one of three forms: `key=value`, `key=` or `key`.
- A **key** is 1 or more bytes, none of which is whitespace, `=` or `"`. So `http.status_code`,
  `k8s-pod` and `app.kubernetes.io/name` are keys.
- `key=` followed by whitespace or the end of the line has the empty value `""`. So `a= b=2` is two
  pairs: `a` with `""` and `b` with `2`.
- A bare `key` with no `=` also has the value `""`.
- A **bare value** (not quoted) runs from right after the `=` to the next whitespace or the end of
  the line. It may contain `=`: `filter=a=b` gives `filter` with `a=b`. A backslash in a bare value
  is an ordinary byte: `path=C:\temp` gives `C:\temp`. A bare value must not contain `"`.
- A **quoted value** starts with a `"` right after the `=` and ends at the next `"` that is not
  escaped. It may contain whitespace and `=`. Inside it these escapes are replaced:
  `\"` becomes `"`, `\\` becomes `\`, `\n` becomes a newline, `\t` becomes a tab. Any other byte
  after a backslash is an error. `Value` holds the result without the quotes. `k=""` is an empty
  value.
- Right after a closing quote there must be whitespace or the end of the line.
- Bytes are copied as they are: UTF-8 text in keys and values comes through unchanged. Parse does
  not validate UTF-8.
- Pairs come back in the order of the line. Duplicate keys are kept, each as its own pair.
- An empty line or a whitespace-only line gives no pairs and no error (a nil or an empty slice
  are both fine).
- On success the error is `nil`.

### Parse: errors

- On malformed input `Parse` returns a **nil** slice and a `*SyntaxError`. It reads the line left
  to right, stops at the first problem it meets and reports only that one.
- `Col` is the 1-based **byte** column of the offending byte: `line[Col-1]`. Count bytes, not
  runes: `é` is 2 bytes. Leading whitespace counts too.
- `Msg` is a short, non-empty description. The wording is yours; the tests do not check it.
- `Error()` returns `logfmt: column <Col>: <Msg>`, for example `logfmt: column 5: unterminated quoted value`.
- `Parse` never panics, and `Col` is always between 1 and `len(line)`.

| Problem | Example | `Col` points at | `Col` |
|---|---|---|---|
| Quoted value with no closing quote (an escaped `\"` does not close it) | `msg="db timeout`, `msg="abc\"` | the opening `"` | 5 |
| Backslash as the last byte inside a quoted value (also unterminated) | `msg="abc\` | the opening `"` | 5 |
| Unknown escape (not `\"` `\\` `\n` `\t`) | `msg="a\qb"` | the `\` | 7 |
| `=` with no key before it | `a=1 =2` | the `=` | 5 |
| `"` where a key byte is expected | `ke"y=v`, `"msg"=hi` | the `"` | 3, 1 |
| `"` inside a bare value | `msg=db"timeout` | the `"` | 7 |
| Anything but whitespace or end of line after a closing quote | `k="a"b` | the byte after the quote | 6 |

### Level

- `String()` returns `debug`, `info`, `warn`, `error` for the 4 constants, and `Level(N)` for any
  other value, for example `Level(-1)` or `Level(42)`. `fmt` uses it: `fmt.Sprint(Warn)` is `warn`.
- `ParseLevel` accepts exactly `debug`, `info`, `warn`, `warning` and `error`, in any letter case
  (`WARN`, `Warning`, `eRRoR`). `warning` means `Warn`. Anything else is an error: `fatal`, `trace`,
  `err`, `warnings`, `""`.
- The error wraps `ErrUnknownLevel` (`errors.Is` works) and contains the input formatted with `%q`,
  for example `unknown level "fatal"`.
- `ParseLevel(l.String()) == l` for the 4 known levels.

### ParseRecord

- It parses the line with `Parse`. A syntax error is returned so that `errors.As(err, &syntaxErr)`
  finds the `*SyntaxError` with its column. Returning it as is or wrapping it with `%w` are both fine.
- If a key appears more than once, its **last** occurrence wins. This holds for every key,
  including `ts`, `level` and `msg`.
- `ts` is required. It is parsed as RFC 3339 (`time.RFC3339`). Fractional seconds
  (`12:00:00.250Z`) and offsets (`+02:00`) are fine. The tests compare times with `Time.Equal`.
- `level` is required. It is parsed with `ParseLevel`.
- `msg` is optional. When it is absent, `Msg` is `""`.
- `Fields` holds every other key with its value. `ts`, `level` and `msg` are not in it. A bare key
  is in it with the value `""`. `Fields` is never nil, also when it is empty.
- `ts` is checked first, then `level`, whatever their order in the line. The first problem is
  reported: `level=fatal ts=yesterday` gives the `ts` error. Every error about these two keys
  starts with the key name and a colon, and wraps the cause with `%w`:

  | Problem | Message starts with | Wraps |
  |---|---|---|
  | `ts` absent | `ts: ` | `ErrMissingField` |
  | `ts` not RFC 3339 | `ts: ` | the `*time.ParseError` from `time.Parse` (`errors.As` works) |
  | `level` absent | `level: ` | `ErrMissingField` |
  | `level` unknown | `level: ` | the `ParseLevel` error (`errors.Is(err, ErrUnknownLevel)`, the `%q` input is in the message) |

- A key with an empty value is present, not missing: `ts=` is a bad timestamp and `level=` is an
  unknown level. Neither wraps `ErrMissingField`.

### FuzzParse

The test file has a fuzz test. Plain `go test` runs it on its seed lines. It checks properties
that follow from the rules above, for any input:

- `Parse` does not panic. On error the error is a `*SyntaxError`, `Col` is between 1 and
  `len(line)`, and the slice is nil.
- On success every key is non-empty and has no whitespace, `=` or `"`.
- Round trip: if you write the pairs back as `key="value"` (with `\` and `"` escaped) and parse
  that line, you get the same pairs.

## Examples

```text
Parse(`ts=2026-10-16T12:00:00Z level=error msg="db timeout" latency_ms=532`)
  → [{ts 2026-10-16T12:00:00Z} {level error} {msg db timeout} {latency_ms 532}], nil

Parse(`svc= dryrun msg="plan=long-quiet at=\"03:12\" silence=87600h" svc=hushd`)
  → [{svc ""} {dryrun ""} {msg `plan=long-quiet at="03:12" silence=87600h`} {svc hushd}], nil

Parse(`   `)            → [], nil
Parse(`msg="db timeout`) → nil, *SyntaxError{Col: 5}  "logfmt: column 5: unterminated quoted value"
Parse(`msg=héllo k="a`)  → nil, *SyntaxError{Col: 14} (é is 2 bytes)
Parse(`k="a"b`)          → nil, *SyntaxError{Col: 6}
```

```text
ParseRecord(`ts=2026-10-16T14:00:00.250+02:00 level=WARNING msg="slow query" db=orders retry`)
  → Record{Time: 2026-10-16 14:00:00.25 +02:00 (the same instant as 12:00:00.25 UTC),
           Level: Warn, Msg: "slow query", Fields: {"db": "orders", "retry": ""}}, nil

ParseRecord(`level=info msg=x`)                → error "ts: missing required field"
ParseRecord(`ts=2026-10-16 level=info`)        → error "ts: parsing time ..." (wraps *time.ParseError)
ParseRecord(`ts=2026-10-16T12:00:00Z level=`)  → error `level: unknown level ""`
```

## Run the tests

    go test ./exercises/26-logfmt/
    go test -run 'TestParse|TestSyntaxError|FuzzParse' -v ./exercises/26-logfmt/   # step 1: Parse
    go test -run TestLevel -v ./exercises/26-logfmt/                               # step 2: Level
    go test -run TestRecord -v ./exercises/26-logfmt/                              # step 3: ParseRecord
    go test -run TestParseErrors -v ./exercises/26-logfmt/                         # one test
    go test -fuzz=FuzzParse ./exercises/26-logfmt/      # optional: let Go invent inputs, Ctrl-C to stop

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The test names are chosen so that the 3 filters above do not overlap. The 2
examples (`ExampleParse`, `ExampleParseRecord`) run only in a plain `go test`; an example cannot
recover a panic, so a plain run crashes until all 3 steps work.

## Docs you can use

- [Strings, bytes, runes and characters in Go](https://go.dev/blog/strings) (Go blog)
- [`strings`](https://pkg.go.dev/strings), in particular `strings.Builder` and `strings.ToLower`
- [`errors`](https://pkg.go.dev/errors): `errors.New`, `errors.Is`, `errors.As`
- [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) and the `%w` verb
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors) (Go blog)
- [Effective Go: Errors](https://go.dev/doc/effective_go#errors)
- [The Go spec: Iota](https://go.dev/ref/spec#Iota)
- [`time.Parse`](https://pkg.go.dev/time#Parse) and the [layout constants](https://pkg.go.dev/time#pkg-constants) such as `time.RFC3339`
- [`fmt.Stringer`](https://pkg.go.dev/fmt#Stringer)
- [Go fuzzing](https://go.dev/security/fuzz/) and the [fuzzing tutorial](https://go.dev/doc/tutorial/fuzz)
- [logfmt](https://brandur.org/logfmt) by Brandur Leach, the article that first documented the format

## Stretch goals

- Write the inverse, `Format(pairs []Pair) string`, that quotes a value only when it needs it
  (empty, or has whitespace, `=`, `"` or a control byte). Add a round-trip test.
- Turn it into a command: read stdin with `bufio.Scanner`, keep lines with `-level warn` or a
  `-match db=orders` filter, print them unchanged. Watch out: `bufio.Scanner` gives up on lines
  of 64 KiB or more unless you call `Scanner.Buffer`.
- Accept what other encoders write: `slog` uses the key `time` instead of `ts` and quotes a key
  that contains a space or `=`, and some encoders emit `\r` and `\uXXXX` escapes (see
  `strconv.Unquote`).
- Make `Parse` allocate less: return substrings of the line when a quoted value has no escapes.
  Measure with a benchmark and `go test -bench . -benchmem`.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 26 hint 1
    go run ./cmd/peek 26 hint 2
    go run ./cmd/peek 26 hint 3
    go run ./cmd/peek 26 explanation
    go run ./cmd/peek 26 walkthrough
    go run ./cmd/peek 26 solution
