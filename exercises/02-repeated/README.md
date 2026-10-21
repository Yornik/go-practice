# 02 · All quiet, 8599 times

**Tier 1 · about 1 h** · Go: slices and `append`, `for range` with an index, nil and empty slices, `strings.Cut`, `strconv.Itoa`, a slice of structs, maps · Problem solving: work small inputs out by hand before you code, the edge cases included

`hushd: all quiet`, 8600 times in 1 night. `hushd` is the agent of HUSH, the AI that Vesper Labs
sells to hold the pager for other companies. On the canary node `canary-h3-01` in Hall 3 it
reports its own calm about every 3 seconds, at INFO. Between midnight and 07:30 the node wrote
9000 log lines, and the other 400 are the ones that matter: the optical link went down 2 times and
the GPU daemon restarted 2 times. Noor Haddad, the principal SRE, scrolled for 10 minutes under the
hum of the Undercroft and found 1 of them. Nothing folds the repeats, so 1 chatty source buries
everything else. `uniq` does not help: the timestamp at the start makes every line unique. Classic
syslogd writes `last message repeated 8599 times`, but the agent writes its own log file. The calm
also takes breaks: 109 stretches, with a link reading or a dropped result in between. In this
exercise you fold each run of repeated messages into its first line and 1 marker line, and you
count how often each message appears.

## Task

Implement the stubs in `repeated.go`. The stub already declares the `Count` type.

```go
func Message(line string) string           // the text after the first space

func Collapse(lines []string) []string     // a run of repeats becomes its first line and 1 marker

type Count struct {
	Message string
	N       int
}
func CountMessages(lines []string) []Count // 1 Count per distinct message, first appearance first
```

Suggested order: `Message`, then `Collapse` (most of the work), then `CountMessages`.

## Rules

### Message

- A line is a **timestamp**, a space and a **message**:
  `2026-09-09T03:01:26Z netd: sfp0 link down`. The first space (the byte 0x20) separates them.
  Only a space does: a tab is an ordinary byte.
- `Message` returns the text after the first space. Further spaces belong to the message, also
  spaces at its start and its end: the message of `"00:01  a "` is `" a "`.
- The timestamp is the text before the first space. A line that starts with a space has an empty
  timestamp.
- A line without a space has no timestamp: its timestamp is empty, and its message is the whole
  line. An empty line has the empty message.
- `Message` trims nothing. 2 messages are the same only if they are equal byte for byte: letter
  case and trailing spaces count.

### Collapse

- A **run** is a group of 2 or more consecutive lines with the same message, as long as possible.
  The timestamps of the lines in a run do not matter: they may differ or be equal.
- Collapse writes the first line of a run unchanged. Instead of the rest of the run, it writes 1
  **marker** line:

  ```text
  <timestamp> last message repeated <N> times
  ```

  - `<timestamp>` is the timestamp of the **last** line of the run.
  - `<N>` is the length of the run minus 1: the number of lines the marker replaces.
  - For N = 1 the marker says `1 time`, not `1 times`.
  - When the timestamp of the last line is empty, the marker is
    `last message repeated <N> times`, with no space in front.
- Collapse writes every line outside a run unchanged.
- The result keeps the input order.
- Empty lines have the empty message and collapse like any other line.
- A marker line in the input is an ordinary line: Collapse compares its message like any other.
  It does not extend the run before it, and equal input markers in a row form a run of their own.
- Collapse does not modify the input slice.
- For empty input (nil or a slice of length 0) the result is empty: nil or an empty slice are both
  fine.

### CountMessages

- It returns 1 `Count` per distinct message. `N` is the number of lines with that message anywhere
  in the input, consecutive or not.
- The counts are in order of first appearance: the message that appears first comes first.
- A line without a timestamp counts with its whole line as the message. Empty lines count with the
  empty message.
- For empty input the result is empty: nil or an empty slice are both fine.

## Examples

```text
Message("2026-09-09T03:01:26Z netd: sfp0 link down") → "netd: sfp0 link down"
Message("2026-09-09T03:01:26Z  two spaces")          → " two spaces"
Message("booting")                                   → "booting"
Message(" hushd: all quiet")                         → "hushd: all quiet"
Message("2026-09-09T03:01:26Z\thushd")               → "2026-09-09T03:01:26Z\thushd" (no space)
Message("")                                          → ""
```

```text
Collapse([
  "2026-09-09T03:01:23Z hushd: all quiet",
  "2026-09-09T03:01:24Z hushd: all quiet",
  "2026-09-09T03:01:25Z hushd: all quiet",
  "2026-09-09T03:01:26Z netd: sfp0 link down",
  "2026-09-09T03:01:28Z hushd: all quiet",
  "2026-09-09T03:01:31Z netd: sfp0 link up after 41 s",
  "2026-09-09T03:01:34Z hushd: all quiet",
  "2026-09-09T03:01:36Z hushd: all quiet",
]) → [
  "2026-09-09T03:01:23Z hushd: all quiet",
  "2026-09-09T03:01:25Z last message repeated 2 times",
  "2026-09-09T03:01:26Z netd: sfp0 link down",
  "2026-09-09T03:01:28Z hushd: all quiet",
  "2026-09-09T03:01:31Z netd: sfp0 link up after 41 s",
  "2026-09-09T03:01:34Z hushd: all quiet",
  "2026-09-09T03:01:36Z last message repeated 1 time",
]
```

The single line at 03:01:28 is not a run and gets no marker. The run at the end gets its marker
too.

```text
Collapse(["booting", "booting", "booting"])  → ["booting", "last message repeated 2 times"]
Collapse([" a", " a"])                       → [" a", "last message repeated 1 time"]
Collapse(["00:01 a", "", "", "", "00:05 b"]) → ["00:01 a", "", "last message repeated 2 times", "00:05 b"]
Collapse(["00:01 a", "00:02 a "])            → ["00:01 a", "00:02 a "]   (a trailing space makes a different message)
Collapse([])                                 → [] or nil
```

Marker lines in the input are ordinary lines:

```text
Collapse([
  "00:01 a",
  "00:02 a",
  "00:03 last message repeated 1 time",
]) → [
  "00:01 a",
  "00:02 last message repeated 1 time",
  "00:03 last message repeated 1 time",
]

Collapse([
  "00:01 x",
  "00:02 last message repeated 1 time",
  "00:03 last message repeated 1 time",
  "00:04 last message repeated 1 time",
]) → [
  "00:01 x",
  "00:02 last message repeated 1 time",
  "00:04 last message repeated 2 times",
]
```

```text
CountMessages(["00:01 b", "00:02 a", "00:03 b", "00:04 c", "00:05 a", "00:06 b"])
  → [{Message: "b", N: 3} {Message: "a", N: 2} {Message: "c", N: 1}]
CountMessages(["", "00:01 a", "", "00:02 "])
  → [{Message: "", N: 3} {Message: "a", N: 1}]
```

`testdata/canary-h3-01.log` holds the 9000 lines of the night. `TestCollapseNight` compares your
`Collapse` with `testdata/canary-h3-01.golden`, the expected 373 lines. `TestCountMessagesNight`
expects 40 distinct messages whose counts add up to 9000. It also checks the first 5 counts, in
order of first appearance: `gpud: started, driver 2.9.3` 3 times, `hushd: all quiet` 8600 times,
`uploadd: queue full, result dropped` 344 times, `netd: sfp0 rx -6.4 dBm` 4 times and
`coold: coolant level 62%` 1 time.

## Run the tests

    go test ./exercises/02-repeated/
    go test -run TestMessage -v ./exercises/02-repeated/          # step 1: Message
    go test -run TestCollapse -v ./exercises/02-repeated/         # step 2: Collapse
    go test -run TestCountMessages -v ./exercises/02-repeated/    # step 3: CountMessages
    go test -run TestCollapseNight -v ./exercises/02-repeated/    # 1 test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The 3 step filters do not overlap. The 2 examples (`ExampleCollapse`,
`ExampleCountMessages`) run only in a plain `go test`. That run crashes until all 3 steps work.

## Docs you can use

- [A Tour of Go: Slices](https://go.dev/tour/moretypes/7), in particular
  [Appending to a slice](https://go.dev/tour/moretypes/15) and
  [Nil slices](https://go.dev/tour/moretypes/12)
- [Go Slices: usage and internals](https://go.dev/blog/slices-intro) (Go blog)
- [The Go spec: For statements with range clause](https://go.dev/ref/spec#For_range)
- [`strings.Cut`](https://pkg.go.dev/strings#Cut)
- [`strconv.Itoa`](https://pkg.go.dev/strconv#Itoa)
- [Go maps in action](https://go.dev/blog/maps) (Go blog)
- [Go Code Review Comments: Declaring Empty Slices](https://go.dev/wiki/CodeReviewComments#declaring-empty-slices)
- [`uniq`](https://man7.org/linux/man-pages/man1/uniq.1.html), the Unix tool that does a part of
  this job

## Stretch goals

- Make it a tool: a command `collapse` that reads a log from stdin with `bufio.Scanner` and writes
  the collapsed lines to stdout. Run it on `testdata/canary-h3-01.log` and compare with the golden
  file. Its neighbour `canary-h3-02` writes the same kind of log.
- Write the marker the way rsyslog does: `message repeated 2 times: [hushd: all quiet]`.
- Treat messages that differ only in their numbers as the same message, so that
  `coold: coolant level 62%` and `coold: coolant level 60%` collapse into 1 run.
- Sort the result of `CountMessages` by `N`, most first, like `sort | uniq -c | sort -rn`. Keep the
  order of first appearance for equal counts (`slices.SortStableFunc`).

## Stuck?

Try for at least 25 minutes first. Then reveal 1 level at a time:

    go run ./cmd/peek 02 hint 1
    go run ./cmd/peek 02 hint 2
    go run ./cmd/peek 02 hint 3
    go run ./cmd/peek 02 explanation
    go run ./cmd/peek 02 walkthrough
    go run ./cmd/peek 02 solution
