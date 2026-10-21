# 24 · Silence it for 2 days, not 2 seconds

**Tier 5 · about 1.5 h** · Go: scanning a string byte by byte, `strings.Builder`, integer overflow checks with `math.MaxInt64`, a custom error type with `Unwrap`, `errors.As` and `errors.Is`, `time.Duration`, a fuzz test · Problem solving: read the grammar in EBNF before you code; walk the input 1 byte at a time and always know your column

In December the `orders` database at Peckish, the bird-feeder company, moved to new disks. The
work would take 2 days, so Ines Okafor, the senior SRE, silenced the disk alerts first, as her
runbook says: `silence.sh disk 2d`. The script handed `2d` to Go's `time.ParseDuration`, which
knows hours, minutes and seconds but not days. It wrote `time: unknown unit "d" in duration "2d"`
to a log file that nobody reads. Then it dropped the unit it did not know, read `2` as seconds and
created a silence of 2 seconds. The silence ended before Ines had closed her laptop. Between 03:12
and 05:00 the alerts that she had silenced paged her 41 times. Prometheus and Alertmanager accept
`15d` and `2w`, so the team's scripts should too. A bad duration must stop the script with an
error that says where the input is wrong. In this exercise you write the parser that `silence.sh`
should have used, and the formatter that writes a duration back.

## Task

Implement the stubs in `durations.go`. The stub gives the sentinel errors and the fields of
`SyntaxError`.

```go
var (
	ErrNumber      = errors.New("expected a number")
	ErrMissingUnit = errors.New("missing unit")
	ErrUnknownUnit = errors.New("unknown unit")
	ErrUnitOrder   = errors.New("units out of order")
	ErrOverflow    = errors.New("duration out of range")
) // given

type SyntaxError struct {
	Input string // the whole input
	Col   int    // 1-based byte column of the problem; len(Input)+1 at the end of the input
	Err   error  // 1 of the 5 sentinels above
}
func (e *SyntaxError) Error() string // `duration "2x": column 2: unknown unit`
func (e *SyntaxError) Unwrap() error // returns Err

func Parse(s string) (time.Duration, error) // "2d", "1w2d3h", "90m", "1500ms", "0"
func Format(d time.Duration) string         // largest units first: "1w2d3h", "0s"
```

Suggested order: `SyntaxError` first (it is small), then `Parse` for valid input, then its errors,
then `Format`.

## Rules

### SyntaxError

- `Error()` returns `duration <Input>: column <Col>: <Err>`, with `Input` quoted by `%q` and `Err`
  written with its message. For example `duration "2x": column 2: unknown unit`, and for an input
  with a quote and a newline, `duration "1h\"\n": column 3: expected a number`.
- `Unwrap()` returns `Err`. So `errors.Is(err, ErrUnknownUnit)` is true for a `*SyntaxError` with
  that `Err`, also after a caller wrapped it with `fmt.Errorf("silence disk: %w", err)`, and
  `errors.As` finds the `*SyntaxError` through that wrapping.

### Parse: the grammar

In EBNF, the notation of the Go spec:

```ebnf
duration = "0" | term { term } .
term     = number unit .
number   = digit { digit } .
digit    = "0" … "9" .
unit     = "w" | "d" | "h" | "m" | "s" | "ms" .
```

| Unit | Means |
|---|---|
| `w` | week: 7 days |
| `d` | day: 24 hours |
| `h` | hour |
| `m` | minute |
| `s` | second |
| `ms` | millisecond |

- A unit is the whole run of ASCII letters (`a`–`z`, `A`–`Z`) right after the number. It must be 1
  of the 6 units above, in lower case. `1hm` has the unit `hm`, which is unknown; it is not `1h`
  followed by `m`. `1H`, `1us`, `5min` and `2mo` are unknown units too.
- The units appear from largest to smallest, as in the table, and each at most once: `1w2d3h`
  and `1w30m` are fine, `2h1d` and `1h1h` are not. `m` comes before `ms`: `1m1ms` is fine.
- A number may be larger than the next unit up: `90m`, `36h` and `1d25h` are fine. Leading zeros
  are fine, also many of them: `01h`, `007s`, and `0000000000000000000000001s` is 1 second.
- The input `0` alone means 0. Every other number needs a unit: `0s` and `0h0m` are fine, `00` and
  `1h0` are not.
- Nothing else is allowed: no whitespace (also not at the start or the end, and not a trailing
  newline), no sign (`-1h`, `+1h`), no decimal point (`1.5h`), no other byte.
- The result is the sum of the terms. It must fit in a `time.Duration`, which is an `int64` of
  nanoseconds: at most `math.MaxInt64` ns, about 292 years. The largest value that `Parse` can
  return is `106751d23h47m16s854ms` (the same value is `15250w1d23h47m16s854ms`).
- On success the error is `nil`. `Parse` never returns a negative duration.

### Parse: errors

- On malformed input `Parse` returns `0` and a `*SyntaxError`. `Input` is the whole input `s`.
- `Parse` reads the input from left to right, stops at the first problem and reports only that
  one. It checks each term in this order: the number, then whether a unit follows, then whether
  the unit is known, then the order of the units, and last the range.
- `Col` is the 1-based **byte** column: the problem is the byte `s[Col-1]`. When the problem is
  the end of the input, `Col` is `len(s)+1`. So `1 <= Col <= len(s)+1` always holds.

| `Err` | When | `Col` points at | Examples (`Col`) |
|---|---|---|---|
| `ErrNumber` | a term must start, but the next byte is not a digit, or the input ends | that byte, or `len(s)+1` | `""` (1), `h` (1), `-1h` (1), ` 1h` (1), `1h ` (3), `1h,30m` (3) |
| `ErrMissingUnit` | the number is not followed by a letter | the byte after the number, or `len(s)+1` | `12` (3), `00` (3), `1h30` (5), `1.5h` (2), `1 h` (2), `1µs` (2) |
| `ErrUnknownUnit` | the run of letters is not 1 of the 6 units | the first letter of the run | `2x` (2), `2mo` (2), `1H` (2), `1us` (2), `1h2x` (4) |
| `ErrUnitOrder` | the unit is not smaller than every unit before it | the first letter of the unit | `1h2d` (4), `1h1h` (4), `1ms1s` (5), `1d2h3d` (6) |
| `ErrOverflow` | the sum up to and including this term is above `math.MaxInt64` ns | the first digit of the term | see below |

`µ` is 2 bytes in UTF-8 and neither of them is an ASCII letter, so `1µs` is a missing unit.

Overflow examples. A term that does not fit alone: `10000000000000000000s` (1), `106752d` (1),
`15251w` (1), `9223372037s` (1), `1h99999999999999999999s` (3). A sum that crosses the limit in
its last term: `106751d23h47m16s855ms` (17), `1w106751d` (3), `106751d24h` (8). The range comes
last, so a large number with a problem in its unit reports the unit:
`99999999999999999999` is `ErrMissingUnit` at 21, `99999999999999999999x` is `ErrUnknownUnit` at
21, and `1s99999999999999999999h` is `ErrUnitOrder` at 23.

### Format

- `Format` truncates `d` toward zero to whole milliseconds, as `d.Truncate(time.Millisecond)` does.
- It writes each unit that is not zero, from `w` to `ms`, as the number followed by the unit name,
  with nothing in between: `1w2d3h`, `1h30m`, `2s500ms`, `3w5s`. Each unit takes as much as it can
  before the next one: `36h` is `1d12h`, `61s` is `1m1s`.
- When nothing is left after the truncation (any value above -1 ms and below 1 ms), it writes
  `0s`.
- A negative value gets a `-` before the units: `-90m` is `-1h30m`. -1.5 ms is `-1ms`, and
  -999 µs is `0s`, without a minus. No test uses `math.MinInt64`.
- `math.MaxInt64` is `15250w1d23h47m16s854ms`.
- For every `d >= 0`: `Parse(Format(d)) == d.Truncate(time.Millisecond)`.

### FuzzParse

The tests have a fuzz test. Plain `go test` runs it on its seeds. It checks properties that follow
from the rules above, for any input:

- `Parse` does not panic. On error the error is a `*SyntaxError` with `Input == s`,
  `1 <= Col <= len(s)+1`, an `Err` that is 1 of the 5 sentinels, and the duration is 0.
- On success the duration is `>= 0`, and `Parse(Format(d))` gives `d` back.

## Examples

A duration is written here as `time.Duration` prints it, and an error as its message.

```text
Parse("2d")          → 48h0m0s, nil
Parse("1w2d3h")      → 219h0m0s, nil
Parse("90m")         → 1h30m0s, nil
Parse("0")           → 0s, nil
Parse("2x")          → 0s, `duration "2x": column 2: unknown unit`
Parse("1h2d")        → 0s, `duration "1h2d": column 4: units out of order`
Parse("12")          → 0s, `duration "12": column 3: missing unit`
Parse("1h ")         → 0s, `duration "1h ": column 3: expected a number`
Parse("106752d")     → 0s, `duration "106752d": column 1: duration out of range`

Format(90 * time.Minute)          → "1h30m"
Format(219 * time.Hour)           → "1w2d3h"
Format(1500 * time.Microsecond)   → "1ms"
Format(-36 * time.Hour)           → "-1d12h"
Format(0)                         → "0s"
```

`ExampleParse` in the tests shows how a caller uses `errors.As` and `Col` to point at the bad byte:

```text
duration "1h2d": column 4: units out of order
1h2d
   ^
```

## Run the tests

    go test ./exercises/24-durations/
    go test -run TestSyntaxError -v ./exercises/24-durations/    # step 1: the error type
    go test -run TestParseValid -v ./exercises/24-durations/     # step 2: Parse, valid input
    go test -run TestParseErrors -v ./exercises/24-durations/    # step 3: Parse, errors and columns
    go test -run TestParseOverflow -v ./exercises/24-durations/  # step 4: Parse, out of range
    go test -run TestFormat -v ./exercises/24-durations/         # step 5: Format
    go test -run FuzzParse -v ./exercises/24-durations/          # step 6: properties for any input
    go test -fuzz=FuzzParse ./exercises/24-durations/     # optional: let Go invent inputs, Ctrl-C to stop

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The test names are chosen so that the 6 filters above do not overlap. Steps 3 and
4 check the error with `errors.Is` and `errors.As`, so they need step 1. Step 5 uses `Parse` for
the round trip, and step 6 uses both functions. The 2 examples (`ExampleParse`, `ExampleFormat`)
run only in a plain `go test`. A plain run runs every step, so it crashes until all steps work.

## Docs you can use

- [The Go spec: Notation](https://go.dev/ref/spec#Notation), how to read the EBNF above
- [Strings, bytes, runes and characters in Go](https://go.dev/blog/strings) (Go blog)
- [`time.Duration`](https://pkg.go.dev/time#Duration), its unit constants and
  [`Duration.Truncate`](https://pkg.go.dev/time#Duration.Truncate)
- [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration), to see what it accepts and what not
- [`math` constants](https://pkg.go.dev/math#pkg-constants) such as `math.MaxInt64`
- [The Go spec: Integer overflow](https://go.dev/ref/spec#Integer_overflow)
- [`strings.Builder`](https://pkg.go.dev/strings#Builder) and [`strconv`](https://pkg.go.dev/strconv)
- [`errors`](https://pkg.go.dev/errors): `errors.Is`, `errors.As`, and what `Unwrap` does
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors) (Go blog)
- [Go fuzzing](https://go.dev/security/fuzz/) and the [fuzzing tutorial](https://go.dev/doc/tutorial/fuzz)
- [Prometheus: querying basics](https://prometheus.io/docs/prometheus/latest/querying/basics/),
  the part on time durations: the format this exercise follows, plus `y` for years

## Stretch goals

- Make it a flag: a type `Duration` with `String()` and `Set(string) error` satisfies
  `flag.Value`, so `flag.Var(&d, "for", "silence length")` accepts `-for 2d`. Then write the
  `silence.sh` replacement that exits with status 2 and the error message on a bad duration.
- Accept `y` (365 days) as Prometheus does. Decide where it goes in the order and add test cases.
- Implement `encoding.TextUnmarshaler` and `encoding.TextMarshaler`, so a JSON or YAML config can
  hold `"retention": "15d"`.
- Add a friendlier message for the most common mistakes: for `1.5h` suggest `1h30m`, for `1H`
  suggest `1h`. Keep the sentinel and the column as they are.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 24 hint 1
    go run ./cmd/peek 24 hint 2
    go run ./cmd/peek 24 hint 3
    go run ./cmd/peek 24 explanation
    go run ./cmd/peek 24 walkthrough
    go run ./cmd/peek 24 solution
