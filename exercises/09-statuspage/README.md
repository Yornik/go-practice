# 09 · All systems operational (they were not)

**Tier 2 · about 1.5 h** · Go: `iota` constants, a `String()` method and `fmt.Stringer`, `switch`, pointer receivers that change a struct, the zero value of a type, `strings.EqualFold`, `strings.Join` · Problem solving: decide what the zero value means; derive the whole from its parts

On Sunday from 09:10 to 12:10 no photo reached Peckish, and `status.peckish.io` said "All systems
operational" the whole time. The status page had restarted at 09:00. From 09:10 the upload service
reported its trouble 180 times, once a minute: `status=partial outage`, with a space. The page only
knows `partial_outage`, so it skipped all 180 updates. "Photo uploads" kept the value it got at
the restart: the zero value. The status type was an `iota` list, and its first constant, the one
that is 0, was `Operational`. Dot, who leads support, answered 96 tickets like this one: "Your page
says all systems operational. A robin has been on my feeder for 3 hours and the app shows 0
photos." Ines Okafor, the senior SRE, wrote 1 line in the postmortem: "Unknown is not
operational." In this exercise you write the status type and the page behind `status.peckish.io`,
with its overall status and the banner that customers read.

## Task

Implement the stubs in `statuspage.go`. `Status` with its 6 constants, `ErrUnknownStatus` and
`ErrUnknownComponent` are given. You write the methods, `ParseStatus`, the `Page` fields and
`NewPage`.

```go
type Status int

const (
	Unknown Status = iota // the zero value: nobody has reported
	Operational
	Maintenance
	Degraded
	PartialOutage
	MajorOutage
) // given

var ErrUnknownStatus = errors.New("statuspage: unknown status")       // given
var ErrUnknownComponent = errors.New("statuspage: unknown component") // given

func (s Status) String() string            // "operational", "under_maintenance", ..., or "Status(N)"
func ParseStatus(s string) (Status, error) // the String words and 2 aliases, in any letter case

type Page struct{ /* you design the fields */ }

func NewPage(components ...string) *Page            // every component starts Unknown
func (p *Page) Set(component string, s Status) error // change 1 component
func (p *Page) Get(component string) (Status, bool)  // read 1 component
func (p *Page) Overall() Status                      // the most severe status on the page
func (p *Page) Banner() string                       // the text customers read
```

Suggested order: `String` and `ParseStatus` first, then `NewPage`, `Set` and `Get`, then
`Overall`, then `Banner`.

## Rules

### The 6 statuses

| Constant | Value | `String()` | Severity | Banner label |
|---|---|---|---|---|
| `Unknown` | 0 | `unknown` | 3rd | `Status unknown` |
| `Operational` | 1 | `operational` | 1st, the least severe | (none) |
| `Maintenance` | 2 | `under_maintenance` | 2nd | `Under maintenance` |
| `Degraded` | 3 | `degraded_performance` | 4th | `Degraded performance` |
| `PartialOutage` | 4 | `partial_outage` | 5th | `Partial outage` |
| `MajorOutage` | 5 | `major_outage` | 6th, the most severe | `Major outage` |

- The zero value of `Status` is `Unknown`. A `Status` that nobody set means "nobody has
  reported", never "operational".
- For the 5 known statuses, the words of `String()` are the component statuses of the
  Statuspage API. The Statuspage API has no `unknown`; that word is ours.
- Severity is not the numeric order. From least to most severe:
  `Operational < Maintenance < Unknown < Degraded < PartialOutage < MajorOutage`. Planned
  maintenance is less severe than not knowing; not knowing is less severe than a known problem.

### String

- `String()` returns the word from the table for the 6 constants, and `Status(N)` for any other
  value, for example `Status(-1)`, `Status(6)` or `Status(42)`.
- `Status` is a `fmt.Stringer`, so `fmt` uses `String`: `fmt.Sprint(Degraded)` is
  `degraded_performance`.

### ParseStatus

- `ParseStatus` accepts exactly the 6 words of `String()` and 2 aliases: `maintenance` means
  `Maintenance`, and `degraded` means `Degraded`.
- It ignores letter case: `OPERATIONAL`, `Major_Outage` and `Degraded` are fine.
- It accepts nothing else. It trims nothing, and it does not accept a space or a `-` in place of
  the `_`. These are errors: `""`, `ok`, `down`, `operational ` (with a space), `partial outage`,
  `partial-outage`, `under maintenance`, `major`, `Status(1)`, `1`.
- On success the error is `nil`. On error `ParseStatus` returns `Unknown` and an error that wraps
  `ErrUnknownStatus` (`errors.Is` works) and contains the input formatted with `%q`, for example
  `statuspage: unknown status "ok"`. The rest of the wording is yours.
- Round trip: `ParseStatus(s.String())` returns `s` and `nil` for each of the 6 constants.

### NewPage, Set and Get

- `NewPage` creates a page with the given components. Every component starts as `Unknown`.
- A name that appears more than once in `NewPage` is 1 component.
- `NewPage`, `Set` and `Get` compare names exactly, letter case included: `api` is not `API`, and
  `API ` (with a space) is not `API`. `NewPage` accepts any name, the empty name included.
- `Set(component, s)` changes the status of 1 component and returns `nil`. A later `Set` of the
  same component replaces the earlier status. `Set` accepts each of the 6 constants, `Unknown`
  included.
- `Set` returns an error in 2 cases, and then changes nothing on the page:
  - the component is not on the page: the error wraps `ErrUnknownComponent` and contains the name
    formatted with `%q`, for example `statuspage: unknown component "Payments"`;
  - `s` is not 1 of the 6 constants, such as `Status(42)`: the error wraps `ErrUnknownStatus`.
- When both are wrong, `Set` checks the component first: the error wraps `ErrUnknownComponent`.
- `Get(component)` returns the status of the component and `true`. For a name that is not on the
  page it returns `Unknown` and `false`.
- The zero value of `Page` (`&Page{}`) is a page without components, the same as `NewPage()`: `Get`
  returns `Unknown, false`, and `Set` returns the `ErrUnknownComponent` error. It does not panic.
- A `Page` does not need to be safe for concurrent use.

### Overall

- `Overall` returns the most severe status of all components, by the severity above.
- A page without components returns `Unknown`: nobody has reported anything.
- `Overall` always describes the page as it is now: after a component recovers, `Overall` goes
  down again.

### Banner

- When every component is `Operational`, the banner is `All systems operational`.
- A page without components shows `Status unknown`.
- Otherwise the banner has 1 group for each status that at least 1 component has, except
  `Operational`. `Banner` joins the groups with `; ` (a semicolon and a space), the most severe
  status first.
- A group is the label from the table, `: `, and the names of its components joined with `, ` (a
  comma and a space). The banner leaves out operational components.
- `Banner` sorts the names in a group A to Z in plain string order (Go's `<` on strings, byte by
  byte). So upper case letters come before lower case ones (`Push` before `api`), and
  `photo-store-10` comes before `photo-store-2`.
- The same page always gives the same banner.

## Examples

```text
p := NewPage("Photo uploads", "API", "Checkout", "Push notifications")

p.Get("API")       → Unknown, true
p.Get("Payments")  → Unknown, false
p.Overall()        → Unknown
p.Banner()         → "Status unknown: API, Checkout, Photo uploads, Push notifications"

Set each of the 4 components to Operational:
p.Overall()        → Operational
p.Banner()         → "All systems operational"

p.Set("Photo uploads", MajorOutage)      → nil
p.Set("Push notifications", Degraded)    → nil
p.Set("Checkout", Maintenance)           → nil
p.Overall()        → MajorOutage
p.Banner()         → "Major outage: Photo uploads; Degraded performance: Push notifications; Under maintenance: Checkout"

p.Set("Payments", Operational)  → error `statuspage: unknown component "Payments"` (wraps ErrUnknownComponent)
p.Set("API", Status(42))        → error (wraps ErrUnknownStatus); "API" stays Operational

NewPage().Banner()              → "Status unknown"
```

```text
ParseStatus("partial_outage")    → PartialOutage, nil
ParseStatus("Under_Maintenance") → Maintenance, nil
ParseStatus("DEGRADED")          → Degraded, nil
ParseStatus("partial outage")    → Unknown, error `statuspage: unknown status "partial outage"` (wraps ErrUnknownStatus)

MajorOutage.String()             → "major_outage"
Status(42).String()              → "Status(42)"
```

## Run the tests

    go test ./exercises/09-statuspage/
    go test -run TestStatus -v ./exercises/09-statuspage/         # step 1: String and ParseStatus
    go test -run TestPageSet -v ./exercises/09-statuspage/        # step 2: NewPage, Set and Get
    go test -run TestPageOverall -v ./exercises/09-statuspage/    # step 3: Overall
    go test -run TestPageBanner -v ./exercises/09-statuspage/     # step 4: Banner
    go test -run TestStatusParseErrors -v ./exercises/09-statuspage/   # one test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The test names are chosen so that the 4 filters above do not overlap. Steps 3
and 4 build their pages with `NewPage` and `Set`, so finish step 2 first. The example
(`ExamplePage`) runs only in a plain `go test`; an example cannot recover a panic, so a plain run
crashes until all 4 steps work.

## Docs you can use

- [The Go spec: Iota](https://go.dev/ref/spec#Iota) and [Effective Go: Constants](https://go.dev/doc/effective_go#constants), which shows an `iota` type with a `String` method
- [The Go spec: The zero value](https://go.dev/ref/spec#The_zero_value)
- [A Tour of Go: Pointer receivers](https://go.dev/tour/methods/4) and [Stringers](https://go.dev/tour/methods/17)
- [Effective Go: Pointers vs. Values](https://go.dev/doc/effective_go#pointers_vs_values)
- [`fmt.Stringer`](https://pkg.go.dev/fmt#Stringer)
- [`strings`](https://pkg.go.dev/strings): `EqualFold`, `Join`
- [`slices`](https://pkg.go.dev/slices): `Sort`
- [`errors`](https://pkg.go.dev/errors): `errors.New`, `errors.Is`, and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`
- [Go maps in action](https://go.dev/blog/maps) (Go blog), the section on iteration order
- [Statuspage: top-level status and incident impact calculations](https://support.atlassian.com/statuspage/docs/top-level-status-and-incident-impact-calculations/): the component statuses and how Atlassian Statuspage combines them

## Stretch goals

- Implement `encoding.TextUnmarshaler` (`UnmarshalText`) on `*Status`, so that `encoding/json`
  reads a status update such as `{"component": "API", "status": "major_outage"}` straight into a
  struct. Add `MarshalText` for the way back. 31-envconfig uses `encoding.TextUnmarshaler` in full.
- Give each component the time of its last update, and show a component as `Unknown` when its
  last update is older than 15 minutes. Pass `now` in, as in 07.
- Write the JSON that the Statuspage API returns from `/api/v2/summary.json`: the components with
  their status words, and a page status with an `indicator` (`none`, `minor`, `major`,
  `critical`). Decide which `indicator` a page with an `Unknown` component gets.
- Serve the banner over HTTP with `net/http`, as a small page that the office screen can show.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 09 hint 1
    go run ./cmd/peek 09 hint 2
    go run ./cmd/peek 09 hint 3
    go run ./cmd/peek 09 explanation
    go run ./cmd/peek 09 walkthrough
    go run ./cmd/peek 09 solution
