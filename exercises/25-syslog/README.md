# 25 · Read the feeders' syslog

**Tier 5 · about 1.5 h** · Go: indexing and slicing a string by byte, `strings.IndexByte`, `time.Parse` with a layout and `time.Date`, an `iota` enum with `String()`, a custom error type with `Unwrap`, `errors.Is`/`errors.As`, `%w` with 2 errors · Problem solving: follow a format spec field by field; decide what a missing year means

On 1 January at 00:05 the fleet dashboard at Peckish, the bird-feeder company, showed 30 000
error lines from the future. The feeders send classic syslog to a relay, and a classic syslog
timestamp has no year: `<11>Dec 31 23:58:02 feeder-0412 camd[311]: lens heater failed`. The relay
filled in the current year. So every line from New Year's Eve landed on 31 December 2027, 364 days
ahead, and the error graph for next New Year's Eve was already red. The relay had a second
surprise. For a line without a host name, it took the tag as the host. The fleet list now has a
feeder called `camd[311]:`, with more errors than any real feeder. Ines Okafor, the senior SRE,
opened the postmortem with "Step 0: breathe". It has 1 action item, with your name on it. In this
exercise you parse 1 syslog line field by field, choose the right year, and report exactly which
column of a broken line is wrong.

## Task

Implement the stubs in `syslog.go`. The stub gives the `Severity` constants, the fields of
`Message` and `ParseError`, and the 4 sentinel errors.

```go
type Severity int // Emergency, Alert, Critical, Error, Warning, Notice, Info, Debug: 0 to 7
func (s Severity) String() string // "emerg", "alert", "crit", "err", "warning", "notice", "info", "debug", or "Severity(N)"

type Message struct {
	Facility int       // priority / 8
	Severity Severity  // priority % 8
	Time     time.Time // in now's location
	Host     string
	App      string
	PID      int // 0 when the tag has no [pid]
	Text     string
}

var ErrPriority, ErrTimestamp, ErrHost, ErrTag error // "bad priority", "bad timestamp", "missing host", "bad tag"

type ParseError struct {
	Field string // "priority", "timestamp", "host" or "tag"
	Col   int    // 1-based byte column; len(line)+1 at the end of the line
	Err   error
}
func (e *ParseError) Error() string // "syslog: column 5: timestamp: <Err>"
func (e *ParseError) Unwrap() error // returns Err

func Parse(line string, now time.Time) (Message, error)
```

Suggested order: `String`, `Error` and `Unwrap` first, then `Parse` 1 field at a time, in the
order of the steps in "Run the tests".

## Rules

### Severity and ParseError

- `String()` returns `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug` for 0
  to 7 (the keywords of `syslog.conf`), and `Severity(N)` for any other value, for example
  `Severity(8)` or `Severity(-1)`. `fmt.Sprint(Warning)` is `warning`.
- `Error()` returns `syslog: column <Col>: <Field>: <Err>`, with `Err` written with its own
  message: `syslog: column 21: host: missing host`.
- `Unwrap()` returns `Err`. So `errors.Is` and `errors.As` look inside `Err`, also when `Err` wraps
  2 errors and when a caller wrapped the `*ParseError` with `%w`.

### Parse: the format

```text
<PRI>Mmm dd hh:mm:ss HOST TAG: TEXT
<11>Dec 31 23:58:02 feeder-0412 camd[311]: lens heater failed
```

| Part | Rule |
|---|---|
| PRI | At the start of the line: `<`, 1 to 3 ASCII digits, `>`. The value is 0 to 191. `Facility` is value / 8, `Severity` is value % 8. |
| Timestamp | The 15 bytes right after `>`, in the layout `time.Stamp` (`Jan _2 15:04:05`). What `time.Parse(time.Stamp, ...)` accepts is fine: `Oct  1` and `Oct 01` are both 1 October. |
| HOST | Exactly 1 space, then 1 or more bytes up to the next space or the end of the line. A HOST that ends with `:` is not a host: it is the tag of a line without a host. |
| TAG | Exactly 1 space, then the app name: 1 or more bytes that are not a space, `[` or `:`. Then an optional `[`, 1 or more ASCII digits, `]`: the PID. Then `:`. |
| TEXT | After the `:`, `Parse` skips 1 space if there is one. `Text` is the rest of the line as it is. It may be empty and may contain `:`, `[` and `]`. |

- A space is the byte `0x20`. No other byte separates fields.
- `PID` is 0 when the tag has no `[pid]`. A PID too large for an `int` (`strconv.Atoi` fails) is
  an error.
- Every part is required. A line without a host or a tag is an error, never a guess.

### Parse: the year and the zone

- The clock in the line is a wall clock time in `now`'s location, and `Time` is in that location.
  `Dec 31 23:58:02` with `now` in `+01:00` is 23:58:02 at `+01:00`, not 23:58:02 UTC.
- The year is `now`'s year. If that puts the time **more than 7 days after** `now`, the year is
  the year before. 7 days is a `time.Duration` of 7 × 24 hours, compared with `Time.Sub`. Exactly
  7 days after `now` is still `now`'s year.
- The tests do not use 29 February. `time.Parse` accepts `Feb 29`, because it reads the timestamp
  in year 0, a leap year. In a year that is not a leap year, `time.Date` normalizes 29 February to
  1 March.

| `now` | Timestamp | `Time` |
|---|---|---|
| 2027-01-01 00:05 `+01:00` | `Dec 31 23:58:02` | 2026-12-31 23:58:02 `+01:00` |
| 2027-01-01 00:05 `+01:00` | `Jan  1 00:04:59` | 2027-01-01 00:04:59 `+01:00` |
| 2026-10-16 12:00 `+02:00` | `Oct 22 12:00:00` (6 days ahead) | 2026-10-22 12:00:00 `+02:00` |
| 2026-10-16 12:00 `+02:00` | `Oct 23 12:00:00` (exactly 7 days) | 2026-10-23 12:00:00 `+02:00` |
| 2026-10-16 12:00 `+02:00` | `Oct 24 12:00:00` (8 days ahead) | 2025-10-24 12:00:00 `+02:00` |

### Parse: errors

- On a malformed line `Parse` returns the zero `Message` and a `*ParseError`. It reads from left
  to right, stops at the first problem and reports only that one.
- `Field` names the part with the problem, and `Err` is that part's sentinel (`ErrPriority`,
  `ErrTimestamp`, `ErrHost`, `ErrTag`).
- For a timestamp that `time.Parse` rejects, `Err` wraps both `ErrTimestamp` and the
  `*time.ParseError`: `errors.Is(err, ErrTimestamp)` and `errors.As(err, &timeErr)` both work.
- `Col` is the 1-based **byte** column of the problem: the byte `line[Col-1]`, or `len(line)+1`
  when the problem is the end of the line.
- `Parse` checks the range of the priority only after it finds the `>`. So `<1234>` is a problem
  at the 4th digit, not a value above 191.
- `Parse` checks the length of the timestamp before its content. A line that ends inside the
  timestamp gets the column of the end, even when an earlier byte is wrong: `<13>Okt 16` gives
  column 11.

| `Field` | Problem | `Col` points at | Examples (`Col`) |
|---|---|---|---|
| `priority` | the line does not start with `<` | the first byte | `""` (1), `13>Oct...` (1), ` <13>Oct...` (1) |
| `priority` | no digit after `<` | that byte, or the end | `<>Oct...` (2), `<a>Oct...` (2), `<` (2) |
| `priority` | no `>` after 1 to 3 digits | that byte, or the end | `<1234>Oct...` (5), `<13Oct...` (4), `<13` (4) |
| `priority` | the value is above 191 | the first digit | `<192>Oct...` (2) |
| `timestamp` | fewer than 15 bytes after `>` | the end | `<13>Oct 16 03:12` (17), `<13>` (5) |
| `timestamp` | `time.Parse` rejects the 15 bytes | the first byte of the timestamp | `<13>Okt 16...` (5), `<13>Oct 1 03:12:44 ...` (5) |
| `timestamp` | the byte after the 15 bytes is not a space | that byte | `<13>Oct 16 03:12:44.250 ...` (20) |
| `host` | no host after the timestamp | the end, or the byte where the host should start | `<11>Dec 31 23:58:02` (20), `<11>Dec 31 23:58:02  feeder-0412...` (21) |
| `host` | the word after the timestamp ends with `:` | the first byte of that word | `<11>Dec 31 23:58:02 camd[311]: lens...` (21) |
| `tag` | no tag after the host, or no app name | the end, or the byte where the app name should start | `... feeder-0412` (32), `... feeder-0412 [311]: ok` (33) |
| `tag` | no `:` after the app name or the `]` | that byte, or the end | `... feeder-0412 camd lens` (37), `... feeder-0412 camd[311] ok` (42) |
| `tag` | after `[`, not 1 or more digits and then `]` | that byte, or the end | `... feeder-0412 camd[12a]: ok` (40), `... feeder-0412 camd[]: ok` (38), `... feeder-0412 camd[311: ok` (41) |
| `tag` | a PID too large for an `int` | the first digit | `... feeder-0412 camd[99999999999999999999]: ok` (38) |

In the `tag` rows, `...` stands for `<11>Dec 31 23:58:02`: the host `feeder-0412` is columns 21
to 31, and the tag starts at column 33.

### Real lines and FuzzParse

- `TestParseRealRFC3164` uses the 5 example lines of RFC 3164, section 5.4 (see Examples).
- `TestParseRealFeedersLog` parses `testdata/feeders.log`, read at 00:05 on 1 January 2027 in
  `+01:00`, and compares each result with `testdata/feeders.golden`, in the format of `render` in
  `real_test.go`.
- `FuzzParse` checks for any input that `Parse` does not panic. On error, `Parse` returns the zero
  `Message` and a `*ParseError`. Its `Col` is 1 to `len(line)+1`, its `Field` is 1 of the 4 names,
  and `errors.Is` finds that field's sentinel. On success, `Facility` is 0 to 23, `Severity` 0 to
  7, `Host` and `App` follow the rules above, `PID >= 0`, `Text` is the end of the line, and `Time`
  is at most 7 days after `now`.

## Examples

```text
now = 2027-01-01 00:05:00 +01:00

Parse("<11>Dec 31 23:58:02 feeder-0412 camd[311]: lens heater failed", now)
  → Message{Facility: 1, Severity: Error, Time: 2026-12-31 23:58:02 +01:00,
            Host: "feeder-0412", App: "camd", PID: 311, Text: "lens heater failed"}, nil

Parse("<14>Jan  1 00:02:00 feeder-0031 seedd: seed level 3%: refill soon", now)
  → Message{Facility: 1, Severity: Info, Time: 2027-01-01 00:02:00 +01:00,
            Host: "feeder-0031", App: "seedd", PID: 0, Text: "seed level 3%: refill soon"}, nil

Parse("<11>Dec 31 23:58:02 camd[311]: lens heater failed", now)
  → Message{}, "syslog: column 21: host: missing host"

Parse("<13>Okt 16 03:12:44 feeder-0412 camd: ok", now)
  → Message{}, `syslog: column 5: timestamp: bad timestamp: parsing time "Okt 16 03:12:44" ...`
```

The RFC 3164 examples. Only the first has every part, so the other 4 are errors:

```text
<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8
  → Facility 4, Critical, Host "mymachine", App "su", PID 0
<13>Feb  5 17:32:18 10.0.0.99 Use the BFG!                          → tag, column 34
<165>Aug 24 05:34:00 CST 1987 mymachine myproc[10]: %% It's time... → tag, column 30
<0>1990 Oct 22 10:52:01 TZ-6 scapegoat.dmz.example.org 10.1.2.3 ... → timestamp, column 4
<0>Oct 22 10:52:12 scapegoat 1990 Oct 22 10:52:01 TZ-6 ...          → tag, column 34
```

`ExampleParse` in the tests uses `errors.As` and `Col` to point at the broken byte:

```text
syslog: column 21: host: missing host
<11>Dec 31 23:58:02 camd[311]: lens heater failed
                    ^
```

## Run the tests

    go test ./exercises/25-syslog/
    go test -run 'TestSeverity|TestParseError' -v ./exercises/25-syslog/   # step 1: String, Error, Unwrap
    go test -run TestParsePriority -v ./exercises/25-syslog/              # step 2: the priority
    go test -run TestParseTimestamp -v ./exercises/25-syslog/             # step 3: the timestamp
    go test -run TestParseYear -v ./exercises/25-syslog/                  # step 4: the year and the zone
    go test -run TestParseHostTag -v ./exercises/25-syslog/               # step 5: host, tag and text
    go test -run 'TestParseReal|FuzzParse' -v ./exercises/25-syslog/      # step 6: real lines, any input
    go test -fuzz=FuzzParse ./exercises/25-syslog/      # optional: let Go invent inputs, Ctrl-C to stop

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The 6 filters above do not overlap. Steps 2 to 5 check only the fields they add:
while you work on step 2, `Parse` may return after the priority with a nil error. Their error
tests want the zero `Message`. Step 6 checks every field. `ExampleParse` runs only in a plain
`go test`; an example cannot recover a panic, so a plain run crashes until all steps work.

## Docs you can use

- [Strings, bytes, runes and characters in Go](https://go.dev/blog/strings) (Go blog)
- [`strings.IndexByte`](https://pkg.go.dev/strings#IndexByte) and [`strconv.Atoi`](https://pkg.go.dev/strconv#Atoi)
- [`time.Parse`](https://pkg.go.dev/time#Parse), [`time.ParseError`](https://pkg.go.dev/time#ParseError)
  and the [layout constants](https://pkg.go.dev/time#pkg-constants), in particular `time.Stamp`
- [`time.Date`](https://pkg.go.dev/time#Date), on the location and on normalization, and
  [`Time.Sub`](https://pkg.go.dev/time#Time.Sub)
- [The Go spec: Iota](https://go.dev/ref/spec#Iota) and [`fmt.Stringer`](https://pkg.go.dev/fmt#Stringer)
- [`errors`](https://pkg.go.dev/errors) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf), on
  `%w` used more than once
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors) (Go blog)
- [Go fuzzing](https://go.dev/security/fuzz/)
- [RFC 3164: The BSD syslog Protocol](https://www.rfc-editor.org/rfc/rfc3164), section 4.1 (the
  format) and section 5.4 (the examples)

## Stretch goals

- Make it a tool: `syslog2logfmt` reads syslog lines on stdin and writes each one as a logfmt
  line: `ts=2026-12-31T23:58:02+01:00 level=err host=feeder-0412 app=camd pid=311 msg="lens heater
  failed"`. Its output is the input of 26-logfmt.
- Accept RFC 5424 lines too: `<11>1 2026-12-31T23:58:02.413+01:00 feeder-0412 camd 311 - - lens
  heater failed`. The `1` after `>` is the version; this format has a year and a zone.
- Handle a feeder clock that runs a little fast at New Year: `Jan  1 00:00:30` read at
  `Dec 31 23:59:50` belongs to next year.
- Listen for real with `net.ListenPacket("udp", ":5140")`, 1 line per packet, and count the lines
  per host and severity.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 25 hint 1
    go run ./cmd/peek 25 hint 2
    go run ./cmd/peek 25 hint 3
    go run ./cmd/peek 25 explanation
    go run ./cmd/peek 25 walkthrough
    go run ./cmd/peek 25 solution
