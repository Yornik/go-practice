# 11 · The log line that is not there

**Tier 3 · about 1.5 h** · Go: `io.Reader`, `bufio.Scanner` and `Scanner.Err`, `io.Writer` and `fmt.Fprintf`, wrapping errors with `%w`, `strings.NewReader` and `testing/iotest` · Problem solving: count while you read; decide what a read error leaves behind

At 03:12 last night, with fog on the ramps of the Undercroft, `canary-h3-01` stopped logging. It
is the canary node in Hall 3 of Vesper Labs, the AI lab of Meridian. Its last line was
`INFO upload started file=step-4141.ckpt`, and its next full line came 47 minutes later. No error,
no warning, just nothing: `grep -c ERROR` finds 3 lines in the whole night, and none of them is
near 03:12. A process that stops writes no line about stopping, so a silence looks exactly like a
calm night. Every postmortem has a box "Was it Fuse?", for the grey cat who sleeps on the warmest
rack in the halls. Today the box says "probably": the canary runs warm. Noor Haddad, the principal
SRE, wrote 1 line under it: "The most important log line is the one that is not there." In this
exercise you write the morning report: it reads a log of any size, counts its lines and levels,
and lists the silences.

## Task

Implement the stubs in `nightlog.go`. The 2 struct types are given.

```go
type Gap struct {
	From time.Time // time of the valid line before the silence
	To   time.Time // time of the valid line after the silence
}

type Summary struct {
	Lines     int            // every line read, blank lines too
	Malformed int            // lines that are not blank and not valid
	Levels    map[string]int // valid lines per level, in upper case; never nil
	First     time.Time      // time of the first valid line; zero if there is none
	Last      time.Time      // time of the last valid line; zero if there is none
	Gaps      []Gap          // silences longer than minGap, in input order
}

func Summarize(r io.Reader, minGap time.Duration) (Summary, error) // reads r to the end
func Write(w io.Writer, s Summary) error                           // the text report
```

Suggested order: follow the 6 steps in "Run the tests".

## Rules

### Lines

- `Summarize` reads `r` until `io.EOF`. `io.EOF` is the normal end of the input, not an error.
- The input is split on `\n`. A last line without `\n` is still a line. `""` has 0 lines, `"\n"`
  has 1 blank line, `"a\n\nb"` has 3 lines.
- `Lines` counts every line: valid, blank and malformed.
- Any run of whitespace separates 2 fields, as in `strings.Fields`. Whitespace includes spaces,
  tabs and the `\r` of a Windows line ending. It may also come before the first field and after
  the last.
- A **blank** line has no fields. It counts in `Lines` and nowhere else.
- A **valid** line has at least 2 fields, and `time.Parse(time.RFC3339, …)` accepts the first one.
  The time needs a zone (`Z`, `+02:00`); fractional seconds (`00:00:01.250Z`) are fine. The second
  field is the level. The rest of the line is the message: it is not checked and may be absent.
  `2026-10-16T03:12:09Z INFO` is valid.
- A **malformed** line is any line that is not blank and not valid. It counts in `Lines` and
  `Malformed` and nowhere else: its time and its level do not count.

| Line | Kind |
|---|---|
| `2026-10-16T03:12:09Z INFO upload started` | valid, level `INFO` |
| `2026-10-16T03:12:09+02:00 warn gpud: fan controller slow` | valid, level `WARN` |
| `2026-10-16T03:12:09Z` | malformed: 1 field |
| `2026-10-16T03:12:09 INFO upload started` | malformed: no zone |
| `2026-10-16 03:12:09 INFO upload started` | malformed: the first field is not an RFC 3339 time |
| `U-Boot 2024.01 (Sep 30 2026 - 11:20:07 +0000)` | malformed |
| `""`, `"   "`, `"\t"` | blank |

### Levels, first and last

- The level of a valid line is its second field in upper case (`strings.ToUpper`): `warn`, `Warn`
  and `WARN` are 1 key, `WARN`. There is no list of known levels: `NOTICE` and `GET` are levels too.
- `Levels` counts the valid lines per level. It is never nil, also for an empty input or an input
  without valid lines.
- `First` and `Last` are the times of the first and the last valid line, **in input order**. They
  are not the earliest and the latest time: when the clock jumps back, `Last` can be before
  `First`. Without valid lines both are the zero `time.Time`.
- Every time in the summary is the value that `time.Parse` returned, so it keeps the offset of the
  line: `2026-10-16T05:12:09+02:00` stays `+02:00`. Do not convert it to UTC. The tests compare
  times by their RFC 3339 text, offset included.

### Gaps

- Look at every 2 **consecutive valid lines**. Skip the blank and malformed lines between them:
  they neither start nor end a silence. When the later line's time minus the earlier line's time
  is **more than** `minGap`, append `Gap{From: earlier time, To: later time}`.
- Exactly `minGap` is not a gap. `minGap` plus 1 s is.
- The difference is between instants, so zones do not matter: from `03:12:09Z` to
  `05:59:41+02:00` is 47m32s.
- A backwards jump gives a negative difference and is never a gap. The next line is still compared
  with the line right before it: after `03:00`, `01:00`, the line `01:30` is 30 min after `01:00`.
- `minGap <= 0` reports no gaps.
- `Gaps` is in input order. When there are no gaps, it may be nil or empty.

### Errors

- On success the error is `nil`.
- A read error stops `Summarize`. It does not retry. It returns the summary of the lines read
  before the error and an error that wraps the reader's error (`errors.Is` works). When a `Read`
  call returns data and an error together, `Summarize` uses the data first. In the tests the error
  comes at the start of the input or right after a `\n`. No test has a partial line before a read
  error.
- A line longer than 64 KiB also stops `Summarize`. It returns the summary of the lines before the
  long line (the long line does not count) and an error that wraps `bufio.ErrTooLong`. 64 KiB is
  the default limit of `bufio.Scanner` (`bufio.MaxScanTokenSize`); you do not need to raise it
  (29-grep does). The tests use a line of 70 KiB; lines near the limit are not tested.
- The result does not depend on how `r` hands out the bytes. The tests try 1 byte per `Read`
  (`iotest.OneByteReader`), half of the bytes the caller asks for (`iotest.HalfReader`), and the
  last bytes in the same call as `io.EOF` (`iotest.DataErrReader`).
- The wording of the error messages is yours.

### Write

- `Write` writes exactly these lines, each ending with `\n`, and nothing else. The label and its
  spaces take 11 characters, so the values line up:

  ```text
  lines:     <Lines>
  malformed: <Malformed>
  first:     <First>
  last:      <Last>
  levels:    <Levels>
  gap:       <From> to <To> (<To.Sub(From)>)
  ```

- Times use the `time.RFC3339` layout (`Time.Format`), in the time's own zone:
  `2026-10-16T05:59:41+02:00` stays `+02:00`. The layout has no fractional seconds, so `Write`
  prints `00:00:01.250Z` as `00:00:01Z`.
- `first` and `last` are `-` when the time is zero (`Time.IsZero`).
- `levels` is `LEVEL=count` for every level, sorted A to Z (plain string order), separated by 1
  space: `ERROR=3 INFO=9001 WARN=207`. It is `-` when `Levels` is nil or empty.
- There is 1 `gap:` line per gap, in the order of `s.Gaps`, and none when there are no gaps. The
  duration is `To.Sub(From)` as `time.Duration` prints it: `47m32s`, `1h50m4s`.
- On success `Write` returns `nil`.
- When a write to `w` fails, `Write` returns an error that wraps it (`errors.Is` works) and does
  not call `w.Write` again. How many `Write` calls the report takes is up to you: the tests use a
  writer that accepts a number of bytes and then fails.

## Examples

```text
input, with minGap 10m:
 1  2026-10-16T03:10:01Z INFO heartbeat gpu_temp_c=41
 2  2026-10-16T03:11:01Z INFO heartbeat gpu_temp_c=41
 3  2026-10-16T03:11:58Z info batch queue=inference
 4  2026-10-16T03:12:09Z WARN upload slow ms=2140
 5  2026-10-16T03:12:
 6
 7  U-Boot 2024.01 (Sep 30 2026 - 11:20:07 +0000)
 8  2026-10-16T03:59:41Z INFO boot drv=2.10.0 reason=power-on
 9  2026-10-16T04:00:02Z ERROR gpud: fan controller failed

Summarize → Summary{
  Lines: 9, Malformed: 2,
  Levels: {"ERROR": 1, "INFO": 4, "WARN": 1},
  First: 2026-10-16T03:10:01Z, Last: 2026-10-16T04:00:02Z,
  Gaps: [{From: 2026-10-16T03:12:09Z, To: 2026-10-16T03:59:41Z}],
}, nil
```

Line 6 is blank. Lines 5 and 7 are malformed. The 1 min between lines 1 and 2 is not a gap; the
47m32s between lines 4 and 8 is. `Write` turns the summary into:

```text
lines:     9
malformed: 2
first:     2026-10-16T03:10:01Z
last:      2026-10-16T04:00:02Z
levels:    ERROR=1 INFO=4 WARN=1
gap:       2026-10-16T03:12:09Z to 2026-10-16T03:59:41Z (47m32s)
```

`Write(w, Summary{})` writes:

```text
lines:     0
malformed: 0
first:     -
last:      -
levels:    -
```

## Run the tests

    go test ./exercises/11-nightlog/
    go test -run TestSummarizeCounts -v ./exercises/11-nightlog/    # step 1: lines, levels, first and last
    go test -run TestSummarizeGaps -v ./exercises/11-nightlog/      # step 2: gaps
    go test -run TestSummarizeReaders -v ./exercises/11-nightlog/   # step 3: any io.Reader
    go test -run TestSummarizeErrors -v ./exercises/11-nightlog/    # step 4: read errors and long lines
    go test -run TestWrite -v ./exercises/11-nightlog/              # step 5: the report
    go test -run TestNight -v ./exercises/11-nightlog/              # step 6: a real night

Run one step at a time; the 6 filters do not overlap. Steps 1 to 4 need only `Summarize`, step 5
only `Write`. Every call into the package runs with a 5 s timeout and a recover. A
"TODO: implement" panic or a loop that never ends then fails 1 test with a message, and the run
goes on. Step 6 reads `testdata/canary-h3-01.log`, compares your report with
`testdata/canary-h3-01.report`, and checks the gaps again with a `minGap` of 5m. `ExampleWrite`
runs the example above, only in a plain `go test`. An example cannot recover a panic, so a plain
run crashes until both functions work.

## Docs you can use

- [`io.Reader`](https://pkg.go.dev/io#Reader) and [`io.Writer`](https://pkg.go.dev/io#Writer):
  read the contracts word by word, in particular what `n > 0` together with an error means
- [`bufio.Scanner`](https://pkg.go.dev/bufio#Scanner), the
  [line-reading example](https://pkg.go.dev/bufio#example-Scanner-Lines),
  [`Scanner.Err`](https://pkg.go.dev/bufio#Scanner.Err) and
  [`bufio.ErrTooLong`](https://pkg.go.dev/bufio#pkg-variables)
- [`strings.Fields`](https://pkg.go.dev/strings#Fields), [`strings.ToUpper`](https://pkg.go.dev/strings#ToUpper)
  and [`strings.NewReader`](https://pkg.go.dev/strings#NewReader)
- [`time.Parse`](https://pkg.go.dev/time#Parse), [`time.RFC3339`](https://pkg.go.dev/time#pkg-constants),
  [`Time.Sub`](https://pkg.go.dev/time#Time.Sub), [`Time.Format`](https://pkg.go.dev/time#Time.Format)
  and [`Time.IsZero`](https://pkg.go.dev/time#Time.IsZero)
- [`fmt.Fprintf`](https://pkg.go.dev/fmt#Fprintf) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`
- [`slices.Sorted`](https://pkg.go.dev/slices#Sorted) and [`maps.Keys`](https://pkg.go.dev/maps#Keys)
- [`testing/iotest`](https://pkg.go.dev/testing/iotest): the readers the tests wrap your input in
- [Errors are values](https://go.dev/blog/errors-are-values) (Go blog), on checking many write errors
- [A Tour of Go: Readers](https://go.dev/tour/methods/21)

## Stretch goals

- Make it a tool: `nightlog -gap 10m < canary-h3-01.log` reads `os.Stdin` and writes the report to
  `os.Stdout`. 15-checkdisk shows how to keep such a `main` testable.
- Count the lines per hour and print a small histogram with `#` characters. A silence then shows
  as an empty row.
- Treat a backwards jump as an event of its own: report it on a `jump:` line. A node whose clock
  resets to 1970 after a reboot makes one.
- Read several files in one run, each with its own report, and a total at the end. 38-logmerge
  reads many sources at once.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 11 hint 1
    go run ./cmd/peek 11 hint 2
    go run ./cmd/peek 11 hint 3
    go run ./cmd/peek 11 explanation
    go run ./cmd/peek 11 walkthrough
    go run ./cmd/peek 11 solution
