# 08 · Small fix, Friday, 17:40

**Tier 2 · about 1 h** · Go: a custom error type with `Error` and `Unwrap`, `errors.As`, `errors.Join`, `errors.Is` through a joined error, `time.Weekday` and `Time.Hour`, the nil error interface · Problem solving: turn each policy sentence into 1 check; report every problem, not only the first

At 17:40 on a Friday, with the trams under the Spire full of people on their way home, `hush-bot`,
HUSH's own deploy bot, shipped new alert thresholds. Its note said "small fix 🌙", its ticket
field said `small fix`, and its rollback field said `n/a`. Floor 61 had been empty for 10 minutes.
Nothing checked the note, so nothing stopped it. Noor Haddad, the principal SRE, spent her
Saturday rolling the thresholds back. Now every deploy note goes through a gate that checks each
field, and nothing goes out between Friday 15:00 and Monday 09:00 without an incident ticket. The
first version of the gate stopped at the first problem it found, so the bot's next note bounced
3 times, 1 field per try. Daniyar Seitkali, the CTO, asks for 1 thing: "Tell it all its mistakes
at once, not 1 per try. It learns fast." In this exercise you write the check behind the gate,
which reports every problem in a form that a human can read and a program can inspect.

## Task

Implement the stubs in `deploynote.go`. `Note`, the 3 sentinel errors and the `FieldError` struct
are given. You write the 2 methods of `FieldError` and `Check`.

```go
type Note struct {
	Service  string
	Version  string
	Ticket   string    // "VL-123", or "INC-45" for an incident fix
	Author   string
	Rollback string    // how to undo the deploy
	At       time.Time // planned time, in the team's zone
}

var (
	ErrMissing   = errors.New("missing")
	ErrBadFormat = errors.New("bad format")
	ErrFreeze    = errors.New("change freeze")
) // given

type FieldError struct {
	Field string // "Service", "Version", "Ticket", "Author", "Rollback" or "At"
	Err   error
}

func (e *FieldError) Error() string // "<Field>: <Err>"
func (e *FieldError) Unwrap() error // Err

// Check returns nil, or errors.Join of 1 *FieldError per field with a problem,
// in the field order above.
func Check(n Note) error
```

Suggested order: the 2 methods of `FieldError` first, then `Check` field by field, then the
freeze.

## Rules

### FieldError

- `Error()` returns `Field`, a colon, a space and `Err.Error()`: `Rollback: missing`.
- `Unwrap()` returns `Err`, the same error value. So `errors.Is(fe, ErrMissing)` is true when
  `Err` is or wraps `ErrMissing`. And `errors.As` finds a `*FieldError` that a caller wrapped
  again with `%w`.
- Both methods have a pointer receiver. `Check` puts `*FieldError` values in its result, and
  callers look for them with `var fe *FieldError; errors.As(err, &fe)`.

### Check: the fields

Each field reports **at most 1** problem. "Blank" means empty or only whitespace:
`strings.TrimSpace` returns `""`.

| Field | Problem | `Err` |
|---|---|---|
| `Service`, `Version`, `Author` | blank | `ErrMissing` |
| `Ticket` | blank | `ErrMissing` |
| `Ticket` | not blank and not a valid ticket | wraps `ErrBadFormat`, message `bad format: <ticket with %q>` |
| `Rollback` | blank, or `n/a`, `none` or `-` in any letter case, with or without whitespace around it | `ErrMissing` |
| `At` | the zero time (`At.IsZero()`) | `ErrMissing` |
| `At` | in the freeze, and `Ticket` is not a valid `INC-` ticket | `ErrFreeze` |

- A **valid ticket** is `VL-` or `INC-` followed by 1 to 6 ASCII digits (`0` to `9`). Nothing
  else: the prefix is uppercase, there is no whitespace before or after it, and no other byte.
  `Check` reads the ticket as written, so `" VL-12"` is a bad format. `VL-000123` is valid.
- For a bad ticket `Err` is `fmt.Errorf("%w: %q", ErrBadFormat, ticket)`. Its message is
  `bad format: "small fix"`; `%q` makes a space around the ticket visible.
- For the other problems `Err` is the sentinel itself: `ErrMissing` or `ErrFreeze`. So their
  messages are `Service: missing` and `At: change freeze`.
- `Service`, `Version`, `Author` and `Rollback` have no format rule: any value that is not blank
  is fine (and, for `Rollback`, not `n/a`, `none` or `-`).

### Check: the freeze

- The freeze runs from **Friday 15:00** to **Monday 09:00**. A time is frozen when it is:
  - a Friday with the hour 15 or later;
  - any time on a Saturday or a Sunday;
  - a Monday with the hour 8 or earlier.
- So Friday 14:59:59.999999999 is fine and Friday 15:00:00 is frozen. Monday 08:59:59.999999999
  is frozen and Monday 09:00:00 is fine. Monday 15:30 is fine: 15:00 matters on a Friday only.
- The weekday and the hour are those of `At` **in its own location**, as `At.Weekday()` and
  `At.Hour()` return them. Friday 15:30 in CET (UTC+1) is frozen. The same instant written in UTC
  is Friday 14:30, and that is fine. The caller passes `At` in the team's zone.
- A valid `INC-` ticket lifts the freeze: an incident fix may go out at any time. A `VL-` ticket
  does not. An invalid ticket does not either: `INC-` without digits, `inc-45` or a blank ticket
  gives a `Ticket` problem **and** an `At` problem.
- A zero `At` is missing, whatever the ticket.

### Check: the result

- With no problem, `Check` returns `nil`.
- With 1 or more problems, `Check` returns `errors.Join` of the `*FieldError` values, 1 per field
  with a problem, in the order `Service`, `Version`, `Ticket`, `Author`, `Rollback`, `At`. A
  single problem goes through `errors.Join` too. The tests read the list with the method
  `Unwrap() []error` that the joined error has, and every element must be a non-nil `*FieldError`.
- `errors.Join` puts a newline between the messages, so `err.Error()` has 1 line per problem.
- `errors.Is(err, ErrFreeze)` on the joined error is true when any of its field errors wraps
  `ErrFreeze`. `errors.As(err, &fe)` finds the **first** field error, in field order.

### The nil error

An `error` is an interface. An interface value is `nil` only when it holds no type and no value.
A nil pointer stored in an `error` is a value of type `*FieldError`, so the interface is **not**
nil:

```go
func check(n Note) error {
	var problem *FieldError // nil
	// ... nothing wrong, problem stays nil ...
	return problem // a non-nil error that holds a nil *FieldError
}

func main() {
	err := check(goodNote)
	fmt.Println(err == nil) // false
	fmt.Printf("%T\n", err) // *deploynote.FieldError
}
```

A caller that writes `if err != nil` then refuses a good note. `Check` must return the untyped
`nil` for a good note: not a nil `*FieldError` and not an empty list of errors inside an `error`.
The tests compare the result with `nil` and print its type with `%T` when it is not nil.

## Examples

```text
Check(Note{Service: "alert-rules", Version: "v1.5.0", Ticket: "VL-412",
           Author: "Wren Lindqvist", Rollback: "helm rollback alert-rules 41",
           At: Tuesday 10 November 2026 10:00 CET})
  → nil

Check(Note{Service: "alert-rules", Version: "v1.5.0", Ticket: "small fix",
           Author: "hush-bot", Rollback: "n/a", At: Friday 6 November 2026 17:40 CET})
  → errors.Join(
      &FieldError{Field: "Ticket",   Err: fmt.Errorf("%w: %q", ErrBadFormat, "small fix")},
      &FieldError{Field: "Rollback", Err: ErrMissing},
      &FieldError{Field: "At",       Err: ErrFreeze},
    )
  err.Error():
    Ticket: bad format: "small fix"
    Rollback: missing
    At: change freeze
  errors.Is(err, ErrFreeze) → true
  errors.As(err, &fe)       → true, fe.Field == "Ticket"

The same note with Ticket "INC-45" and a Rollback → nil (the incident ticket lifts the freeze)
Check(Note{})  → 6 problems, each ErrMissing, from Service to At
```

| `At` | Frozen |
|---|---|
| Thursday 23:59:59 CET | no |
| Friday 14:59:59 CET | no |
| Friday 15:00:00 CET | yes |
| Saturday or Sunday, any time | yes |
| Monday 08:59:59 CET | yes |
| Monday 09:00:00 CET | no |
| Friday 15:30 CET | yes |
| the same instant in UTC: Friday 14:30 UTC | no |

| `Ticket` | Result |
|---|---|
| `VL-1`, `VL-412`, `VL-999999`, `INC-0`, `INC-45` | valid |
| `""`, `"   "` | `Ticket: missing` |
| `VL-`, `VL-1234567`, `VL123`, `VL--12`, `VL-12a`, `vl-12`, `INC45` | `Ticket: bad format: "<ticket>"` |
| `VL-+12` (a sign is not a digit), `VL-١٢` (Arabic-Indic digits are not ASCII digits) | `Ticket: bad format: "<ticket>"` |
| `" VL-12"`, `"VL-12 "` (whitespace counts) | `Ticket: bad format: "<ticket>"` |

## Run the tests

    go test ./exercises/08-deploynote/
    go test -run TestFieldError -v ./exercises/08-deploynote/      # step 1: FieldError
    go test -run TestCheckFields -v ./exercises/08-deploynote/     # step 2: 1 field at a time
    go test -run TestCheckFreeze -v ./exercises/08-deploynote/     # step 3: the freeze
    go test -run TestCheckAll -v ./exercises/08-deploynote/        # step 4: every problem at once
    go test -run TestCheckFieldsTicket -v ./exercises/08-deploynote/   # one test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The test names are chosen so that the 4 filters above do not overlap. Steps 2
to 4 call the methods of `FieldError` through `errors.Is` and `errors.As`, so finish step 1
first. Step 2 already expects the result of `errors.Join`, for a single problem too. Its notes
plan the deploy for a Tuesday, so the freeze does not get in the way until step 3. The example
(`ExampleCheck`) runs only in a plain `go test`; an example cannot recover a panic, so a plain run
crashes until all 4 steps work.

## Docs you can use

- [A Tour of Go: Errors](https://go.dev/tour/methods/19)
- [`errors`](https://pkg.go.dev/errors): `errors.New`, `errors.Is`, `errors.As`, `errors.Join`,
  and the package overview on wrapping
- [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) and the `%w` verb, and the [`%q` verb](https://pkg.go.dev/fmt#hdr-Printing)
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors) (Go blog)
- [Go 1.20 release notes: wrapping multiple errors](https://go.dev/doc/go1.20#errors)
- [Go FAQ: Why is my nil error value not equal to nil?](https://go.dev/doc/faq#nil_error)
- [`time.Time.Weekday`](https://pkg.go.dev/time#Time.Weekday), [`time.Weekday`](https://pkg.go.dev/time#Weekday),
  [`time.Time.Hour`](https://pkg.go.dev/time#Time.Hour), [`time.Time.IsZero`](https://pkg.go.dev/time#Time.IsZero)
  and [`time.Time.In`](https://pkg.go.dev/time#Time.In)
- [`strings`](https://pkg.go.dev/strings): `TrimSpace`, `EqualFold`, `CutPrefix`, `HasPrefix`

## Stretch goals

- Take the team's zone as a parameter: `Check(n Note, loc *time.Location) error` checks the freeze
  with `n.At.In(loc)`, so `Check` judges a note written in UTC by the clock in Amsterdam. Load the zone with
  `time.LoadLocation("Europe/Amsterdam")` and test the weekend in October when the clocks go back.
  Meridian's clocks go back that weekend too, and that night has 1 hour that happens twice.
- Add extra freeze windows from a list, for example Launch Week and 24 December to 2 January.
  Each window has a start, an end and a reason, and the error says which window blocked the
  deploy. Launch Week brings HUSH 5 to 40 000 companies, and nobody wants a small fix that week.
- Write the bot's command: read a note as JSON from stdin, print 1 line per problem, and exit
  with status 1 when there is any. Walk the joined error to put each field name first.
  `hush-bot` reads the reply too, so every line must make sense on its own.
- Give the program more than a sentinel: add the bad value to a `BadFormatError` type with the
  value and the expected pattern, and find it with `errors.As`.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 08 hint 1
    go run ./cmd/peek 08 hint 2
    go run ./cmd/peek 08 hint 3
    go run ./cmd/peek 08 explanation
    go run ./cmd/peek 08 walkthrough
    go run ./cmd/peek 08 solution
