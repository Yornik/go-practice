# 03 · Was it Kevin? Count the pages

**Tier 1 · about 1 h** · Go: structs as values, maps as counters and sets, maps with struct keys, `slices.SortFunc` with `cmp.Compare` and `cmp.Or`, the iterators of `maps.Keys` and `slices.Sorted`, `strconv.Atoi` · Problem solving: choose the key you group by; make the output the same on every run

Ines Okafor, the senior SRE, carried the Owl for the last 2 weeks of September. The Owl is a
ceramic owl that stands on the desk of whoever is on call. She was paged 61 times, 19 of them
between 22:00 and 07:00. Bram, the CTO, is sure that it was the database. Everybody else is sure
that it was Kevin, the squirrel who lives in the tree next to the demo feeder `hq-window-01`: the
team rule says that every incident is Kevin until the logs prove otherwise. Ines wants numbers
instead of feelings before you join the rotation. The pager exports every page as 1 line,
`2026-09-23 03:12 FeederOffline hq-window-01`, but its own report counts pages per day, not per
alert. It can tell you that 23 September was a bad day, but not which alert woke Ines at 03:12.
In this exercise you turn the export into that table: the pages per alert, the pages at night,
and the sources that sent them.

## Task

Implement the stubs in `pagenoise.go`. The stub already defines the 3 struct types.

```go
type Page struct {
	Day    string // "2026-09-14", as written; not checked
	Hour   int    // 0 to 23, the team's local time
	Minute int    // 0 to 59
	Alert  string
	Source string
}
func ParsePage(line string) (Page, error)     // "<day> <HH:MM> <alert> <source>"

type AlertCount struct {
	Alert   string
	Pages   int      // every page of this alert
	Night   int      // pages from 22:00 up to 06:59
	Sources []string // distinct sources, A to Z
}
func Rank(pages []Page) []AlertCount          // 1 AlertCount per alert, the noisiest first

type Pair struct{ Alert, Source string }
func CountPairs(pages []Page) map[Pair]int    // pages per alert and source
```

Suggested order: `ParsePage`, then `Rank` (first the counts, then the order), then `CountPairs`.

## Rules

### ParsePage

- A line has exactly 4 fields: `<day> <time> <alert> <source>`, for example
  `2026-09-23 03:12 FeederOffline hq-window-01`.
- Any run of whitespace separates 2 fields, exactly as
  [`strings.Fields`](https://pkg.go.dev/strings#Fields) splits them. Whitespace includes spaces,
  tabs, newlines and the `\r` of a Windows line ending. It may also come before the first field
  and after the last.
- A line with fewer or more than 4 fields is an error. An empty line is an error too.
- The time is exactly 5 bytes: 2 ASCII digits (`0` to `9`), a colon and 2 ASCII digits. The hour
  is from `00` to `23`, the minute from `00` to `59`. `Hour` and `Minute` hold them as numbers:
  `03:12` gives `Hour` 3 and `Minute` 12.
- Every other time is an error. For example: `3:12` (no leading zero), `24:00`, `12:60`, `12-30`,
  `1230`, `12:30:00`, `+1:00` and `12:+5` (a sign is not a digit), `ab:cd`.
- `ParsePage` keeps the day, the alert and the source exactly as written, letter case included.
  It does not check the day: `yesterday` is a day like any other.
- The pager exports every time in the team's local time, so there are no time zones in this
  exercise. 07-rollcall brings the `time` package.
- On an error, `ParsePage` returns the zero `Page` and an error. The message contains the whole
  line formatted with `%q`, for example
  `pagenoise: time "3:12" is not HH:MM from 00:00 to 23:59: "2026-09-20 3:12 FeederOffline hq-window-01"`.
  The rest of the wording is yours.

### Rank

- `Rank` returns 1 `AlertCount` per distinct alert in `pages`.
- `Pages` is the number of pages of that alert.
- `Night` is the number of those pages at night. A page is at night when its hour is 22 or 23 or
  from 0 to 6: from 22:00 up to and including 06:59. `21:59` and `07:00` are day; `22:00` and
  `06:59` are night. The minute does not matter, only the hour.
- `Sources` holds every source that sent a page of that alert, each source once, sorted A to Z.
- Alerts and sources compare exactly as written: `FeederOffline` and `feederoffline` are 2 alerts.
- `Rank` sorts the result by:
  1. `Pages`, most first.
  2. `Night`, most first.
  3. `Alert`, A to Z.
- "A to Z" is plain string order (`<`, byte by byte), for alerts and for sources. So upper case
  comes before lower case: `DiskFull` < `Zombie` < `diskFull`.
- The same input always gives the same result, in the same order.
- `Rank` does not modify `pages`.
- For no pages (a nil or an empty slice), the result has length 0. The result may be nil or an
  empty slice.

### CountPairs

- `CountPairs` counts the pages per alert and source: the key is a `Pair` of the page's alert and
  source, the value is the number of pages with that alert and that source.
- The map has only keys with at least 1 page.
- The map is never nil, also when there are no pages.

## Examples

```text
ParsePage("2026-09-23 03:12 FeederOffline hq-window-01")
  → Page{Day: "2026-09-23", Hour: 3, Minute: 12, Alert: "FeederOffline", Source: "hq-window-01"}, nil

ParsePage("2026-09-25\t22:00\tFeederOffline\thq-window-01\r")
  → Page{Day: "2026-09-25", Hour: 22, Minute: 0, Alert: "FeederOffline", Source: "hq-window-01"}, nil

ParsePage("2026-09-20 3:12 FeederOffline hq-window-01")              → Page{}, error
ParsePage("2026-09-25 11:30 PhotoStoreDiskFull photo-store-2 acked") → Page{}, error (5 fields)
```

```text
pages, from these 7 lines:
  2026-09-23 03:12 FeederOffline hq-window-01
  2026-09-23 07:50 PushQueueBacklog push-1
  2026-09-23 14:05 PhotoStoreDiskFull photo-store-2
  2026-09-23 22:00 FeederOffline feeder-0412
  2026-09-24 06:30 PushQueueBacklog push-1
  2026-09-24 11:20 PhotoStoreDiskFull photo-store-3
  2026-09-24 23:41 FeederOffline hq-window-01

Rank(pages) →
  {Alert: FeederOffline       Pages: 3  Night: 3  Sources: [feeder-0412 hq-window-01]}
  {Alert: PushQueueBacklog    Pages: 2  Night: 1  Sources: [push-1]}
  {Alert: PhotoStoreDiskFull  Pages: 2  Night: 0  Sources: [photo-store-2 photo-store-3]}

CountPairs(pages) →
  {FeederOffline hq-window-01}: 2
  {FeederOffline feeder-0412}: 1
  {PushQueueBacklog push-1}: 2
  {PhotoStoreDiskFull photo-store-2}: 1
  {PhotoStoreDiskFull photo-store-3}: 1
```

`PushQueueBacklog` and `PhotoStoreDiskFull` both have 2 pages. `PushQueueBacklog` has 1 at night
(06:30) and comes first.

`testdata/pages.log` is the export of the 2 weeks: 63 lines with 61 pages. Line 17 and line 40
are malformed. `TestTwoWeeksRanking` and `TestTwoWeeksPairs` parse every line, check that exactly
those 2 lines fail, and compare the full result.

## Run the tests

    go test ./exercises/03-pagenoise/
    go test -run TestParsePage -v ./exercises/03-pagenoise/                    # step 1: ParsePage
    go test -run TestRankCounts -v ./exercises/03-pagenoise/                   # step 2: Rank, the counts
    go test -run 'TestRankOrder|TestRankNight' -v ./exercises/03-pagenoise/    # step 3: Rank, order and night
    go test -run TestCountPairs -v ./exercises/03-pagenoise/                   # step 4: CountPairs
    go test -run TestTwoWeeks -v ./exercises/03-pagenoise/                     # step 5: the 2 weeks of pages
    go test -run TestParsePageBadTime -v ./exercises/03-pagenoise/             # 1 test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The 5 step filters above do not overlap. Step 2 ignores the order of the
result; step 3 checks it. Step 5 needs steps 1 to 4. The example (`ExampleRank`) runs only in a
plain `go test` and needs steps 1 to 3. A plain run calls every function, so it crashes until
steps 1 to 4 work.

## Docs you can use

- [A Tour of Go: Structs](https://go.dev/tour/moretypes/2) and [Maps](https://go.dev/tour/moretypes/19)
- [Go maps in action](https://go.dev/blog/maps) (Go blog), in particular "Exploiting zero values",
  "Key types" and "Iteration order"
- [The Go spec: Map types](https://go.dev/ref/spec#Map_types) and
  [Comparison operators](https://go.dev/ref/spec#Comparison_operators), on which types can be a
  map key
- [`strings.Fields`](https://pkg.go.dev/strings#Fields) and [`strconv.Atoi`](https://pkg.go.dev/strconv#Atoi)
- [`slices.SortFunc`](https://pkg.go.dev/slices#SortFunc), [`slices.Sorted`](https://pkg.go.dev/slices#Sorted)
  and [`maps.Keys`](https://pkg.go.dev/maps#Keys)
- [`cmp.Compare`](https://pkg.go.dev/cmp#Compare) and [`cmp.Or`](https://pkg.go.dev/cmp#Or)
- [Range Over Function Types](https://go.dev/blog/range-functions) (Go blog), on the iterators
  that `maps.Keys` returns and `slices.Sorted` reads
- [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) and the [`%q` verb](https://pkg.go.dev/fmt#hdr-Printing)
- [Google SRE book: Monitoring Distributed Systems](https://sre.google/sre-book/monitoring-distributed-systems/),
  on which alerts deserve a page

## Stretch goals

- Turn it into a command: read the export from stdin with `bufio.Scanner`, print the ranking as a
  table, and print each malformed line with its line number. Try it on `testdata/pages.log`.
- Count the pages per hour of the day in a `[24]int` and print a small bar chart with `#`
  characters. When does the pager ring most?
- Find the noisiest source of each alert with `CountPairs`. Break ties A to Z, so the answer is
  the same on every run.
- Count the pages per on-call shift instead of per day. A shift starts at 09:00, so a page at
  03:12 on 24 September belongs to the shift of 23 September.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 03 hint 1
    go run ./cmd/peek 03 hint 2
    go run ./cmd/peek 03 hint 3
    go run ./cmd/peek 03 explanation
    go run ./cmd/peek 03 walkthrough
    go run ./cmd/peek 03 solution
