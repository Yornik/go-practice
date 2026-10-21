# 07 · Which feeders went quiet?

**Tier 2 · about 1 h** · Go: `time.Parse` with `time.RFC3339`, comparing `time.Time` (`Before`, `After`, `Equal`), `Time.Sub` and `time.Duration`, sets as `map[string]struct{}`, `slices.Sorted(maps.Keys(m))`, passing `now` in · Problem solving: compare what you expect with what you see; put every boundary case on 1 side

Dot, who leads support, forwards a ticket from Mrs. Jansen, her most loyal customer: "My feeder
has been silent since Tuesday. Is it dead or is it the cat?" The Owl, the ceramic owl on the desk
of whoever is on call, sits on your desk this week, so the question is yours. The fleet dashboard
is all green. It draws 1 line per feeder that sends a heartbeat, so a feeder that sends nothing has
no line that can turn red. A dead feeder does not look broken. It simply disappears. Nobody knows
how many of the 40 000 feeders went quiet the same way. Ines Okafor, the senior SRE, wants a roll
call: the list of feeders that should report, next to the heartbeats that arrived. In this
exercise you parse the heartbeats and sort every feeder into healthy, stale, missing or unknown.

## Task

Implement the stubs in `rollcall.go`. `ErrMalformed`, `Heartbeat` and `Report` are given.

```go
var ErrMalformed = errors.New("rollcall: malformed heartbeat") // given

type Heartbeat struct {
	Feeder string
	At     time.Time
}
func ParseHeartbeat(line string) (Heartbeat, error) // "hq-window-01 2026-10-15T08:55:00Z"

type Report struct {
	Healthy  []string             // in the inventory, last heartbeat at most staleAfter before now
	Stale    []string             // in the inventory, last heartbeat more than staleAfter before now
	Missing  []string             // in the inventory, no heartbeat at all
	Unknown  []string             // sent heartbeats, not in the inventory
	LastSeen map[string]time.Time // latest heartbeat of every feeder that sent one
}
func RollCall(inventory []string, beats []Heartbeat, now time.Time, staleAfter time.Duration) Report
```

Suggested order: `ParseHeartbeat` first, then `RollCall` in the order of the test steps.

## Rules

### ParseHeartbeat

- A line has exactly 2 fields: `<feeder> <time>`. Any run of whitespace separates the fields, as
  `strings.Fields` splits them. Whitespace means spaces, tabs and the `\r` of a Windows line
  ending, also before the first field and after the last.
- The feeder name is the first field, exactly as written. ParseHeartbeat does not check it further.
- The time must parse with `time.Parse(time.RFC3339, ...)`, so it needs a zone: `Z`, `+02:00` or
  `-04:00`. Fractional seconds are fine: `08:58:03.5Z`, `08:29:59.999999999Z`. A date without a
  time, a time without a zone, month 13, `yesterday` and Unix seconds are malformed.
- `At` is the parsed time. `10:31:12+02:00` and `08:31:12Z` are the same instant. The tests compare
  times with `Equal`, so you may keep the zone of the line or convert the time to UTC.
- A malformed line, an empty line included, returns the zero `Heartbeat` and an error. The error
  wraps `ErrMalformed` (`errors.Is` works) and contains the whole line formatted with `%q`, for
  example `rollcall: malformed heartbeat: "feeder-0100 2026-10-15T08:51:00": ...`. The rest of the
  message is yours; the tests do not check it.

### RollCall: who is where

- The **inventory** lists the feeders that should report. The **heartbeats** show the feeders that
  did report.
- RollCall compares names exactly, letter case included: `FEEDER-0012` is not `feeder-0012`.
- RollCall ignores the empty name `""` everywhere, in the inventory and in the heartbeats. It is in
  no list and not in `LastSeen`.
- The **last heartbeat** of a feeder is the one with the latest `At`, not the last one in the
  slice. Heartbeats come in any order. Compare times as instants: `08:50Z` is later than
  `10:40+02:00` (which is `08:40Z`).
- Each feeder of the inventory is in exactly 1 of 3 lists. `last` is the `At` of its last
  heartbeat:

  | List | When |
  |---|---|
  | `Missing` | the feeder sent no heartbeat at all |
  | `Healthy` | `now.Sub(last) <= staleAfter` |
  | `Stale` | `now.Sub(last) > staleAfter` |

- So a last heartbeat that is exactly `staleAfter` old is healthy, and 1 ns older is stale. A
  heartbeat after `now` (the feeder's clock is ahead) has a negative age, so the feeder is healthy.
- `Unknown` lists every feeder that sent a heartbeat but is not in the inventory, whatever the age
  of its last heartbeat.
- `LastSeen` maps every feeder that sent a heartbeat, in the inventory or not, to the `At` of its
  last heartbeat, also when that is after `now`. It is never nil, also when it is empty.
- `now` and `staleAfter` are parameters: RollCall never reads the clock. The tests pass a
  `staleAfter` of 0 or more.

### RollCall: the lists

- RollCall sorts every list A to Z in plain string order (`<`, as `slices.Sort` sorts strings).
  Upper case letters sort before lower case: `FEEDER-0012`, `feeder-0002`, `hq-window-01`.
- A name appears at most once in a list, also when it appears several times in the inventory or
  sends many heartbeats.
- An empty list may be nil or an empty slice. The tests do not tell them apart.
- RollCall does not modify its inputs: after the call, the inventory and the heartbeats have the
  same order and content as before.

## Examples

```text
now        = 2026-10-15T09:00:00Z
staleAfter = 30m
inventory  = [hq-window-01 feeder-0217 feeder-0412 reserve-01 reserve-02 hq-window-01 ""]

line                                     ParseHeartbeat
hq-window-01 2026-10-15T08:55:00Z        ok
feeder-0412 2026-10-15T10:31:12+02:00    ok (08:31:12 UTC)
feeder-0217 2026-10-13T18:42:10+02:00    ok (Tuesday)
feeder-0412 2026-10-15T08:01:40Z         ok (older than the other heartbeat of feeder-0412)
reserve-01 2026-10-15T08:30:00Z          ok (exactly 30 min before now)
feeder-0501 2026-10-15T08:58:03.5Z       ok (not in the inventory)
hq-window-01 2026-10-15T09:01:30Z        ok (90 s after now)
feeder-0100 2026-10-15T08:51:00          error: no zone; wraps ErrMalformed
```

```text
RollCall(inventory, the 7 heartbeats that parsed, now, 30m) →
  Healthy:  [feeder-0412 hq-window-01 reserve-01]
  Stale:    [feeder-0217]
  Missing:  [reserve-02]
  Unknown:  [feeder-0501]
  LastSeen: feeder-0217   2026-10-13T16:42:10Z     (18:42:10+02:00 in the line)
            feeder-0412   2026-10-15T08:31:12Z     (10:31:12+02:00 in the line)
            feeder-0501   2026-10-15T08:58:03.5Z
            hq-window-01  2026-10-15T09:01:30Z
            reserve-01    2026-10-15T08:30:00Z
```

The latest heartbeat of `feeder-0412` is 08:31:12 UTC, 28 min 48 s before now, so it is healthy;
its older heartbeat does not count. `reserve-01` is exactly 30 min old and still healthy. The clock
of `hq-window-01` is 90 s ahead: it is healthy, and `LastSeen` keeps 09:01:30. `hq-window-01`
appears twice in the inventory and once in `Healthy`. RollCall ignores the empty name. The
example writes `LastSeen` in UTC, as `ExampleRollCall` prints it. A time in the zone of the line
is the same instant and passes too.

## Run the tests

    go test ./exercises/07-rollcall/
    go test -run TestParseHeartbeat -v ./exercises/07-rollcall/    # step 1: ParseHeartbeat
    go test -run TestRollCallSets -v ./exercises/07-rollcall/      # step 2: healthy, missing, unknown
    go test -run TestRollCallStale -v ./exercises/07-rollcall/     # step 3: stale, boundaries, zones
    go test -run TestRollCallInputs -v ./exercises/07-rollcall/    # step 4: duplicates, empty names, inputs
    go test -run TestRollCallFleet -v ./exercises/07-rollcall/     # step 5: the whole fleet

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. No 2 of the 5 filters above select the same test. In step 2 every heartbeat is
a few minutes old, so no feeder is stale yet.

Step 5 reads `testdata/inventory.txt` (500 feeders, 2 of them listed twice) and
`testdata/heartbeats.txt` (1434 lines in no particular order). It parses every line with
`ParseHeartbeat` and expects exactly lines 58, 413, 804 and 1121 to be malformed. Then it compares
the report with `testdata/report.golden` and prints the first line that differs. `ExampleRollCall`
runs the example above. It runs only in a plain `go test`; an example cannot recover a panic, so
a plain run crashes until all 5 steps work.

## Docs you can use

- [`time.Time`](https://pkg.go.dev/time#Time): an instant plus a location, and why `==` on 2
  `time.Time` values is not a time comparison
- [`time.Parse`](https://pkg.go.dev/time#Parse) and the [layout constants](https://pkg.go.dev/time#pkg-constants)
  such as `time.RFC3339`
- [`Time.Before`](https://pkg.go.dev/time#Time.Before), [`Time.After`](https://pkg.go.dev/time#Time.After),
  [`Time.Equal`](https://pkg.go.dev/time#Time.Equal), [`Time.Sub`](https://pkg.go.dev/time#Time.Sub)
  and [`time.Duration`](https://pkg.go.dev/time#Duration)
- [`strings.Fields`](https://pkg.go.dev/strings#Fields)
- [`errors.Is`](https://pkg.go.dev/errors#Is) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf)
  with the `%w` verb
- [`maps.Keys`](https://pkg.go.dev/maps#Keys) and [`slices.Sorted`](https://pkg.go.dev/slices#Sorted)
- [Go maps in action](https://go.dev/blog/maps) (Go blog), on sets and iteration order
- [Range Over Function Types](https://go.dev/blog/range-functions) (Go blog), on the iterator
  that `maps.Keys` returns
- [RFC 3339: Date and Time on the Internet: Timestamps](https://www.rfc-editor.org/rfc/rfc3339)
- [Prometheus: `absent()`](https://prometheus.io/docs/prometheus/latest/querying/functions/#absent),
  the alert for a series that is not there

## Stretch goals

- Make it a tool: `rollcall -stale 30m inventory.txt heartbeats.txt` prints the 4 lists and the
  number of malformed lines, and exits with status 1 when a feeder is stale or missing.
- Split `Stale` by age, so that support sees who went quiet when: 30 min to 1 h, 1 h to 1 day,
  more than 1 day.
- Flag feeders whose clock is ahead by more than 5 min in a `Skewed` list. A clock that is far
  ahead hides a dead feeder for exactly as long as the skew.
- Write `LastSeen` as a Prometheus metric, `feeder_last_heartbeat_timestamp_seconds{feeder="..."}`,
  and write the alert rule that finds stale feeders with `time() - ...`. Which feeders can that
  rule never find?

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 07 hint 1
    go run ./cmd/peek 07 hint 2
    go run ./cmd/peek 07 hint 3
    go run ./cmd/peek 07 explanation
    go run ./cmd/peek 07 walkthrough
    go run ./cmd/peek 07 solution
