# 01 · Zero errors since March

**Tier 1 · about 1 h** · Go: `strings.Fields`, `strings.Cut`, `strings.ToLower`, maps for lookup and counting, `switch`, returning `(value, error)` · Problem solving: list every spelling before you code; decide what "unknown" means

It is 21:40, rain runs down the glass of floor 61, and it is your first night at Vesper Labs, the AI
lab at the top of the Spire. Vesper Labs sells HUSH, an AI that holds the pager for 4000 companies
so that their engineers can sleep, and it has 1 rule, etched in the lobby glass: no AI writes the
code that watches the AI. Noor Haddad, the principal SRE, gives you a laptop, a lanyard with an
empty steel ring and a mug that says `RAW LINE FIRST`. Then she opens the panel "Errors per hour"
for the inference fleet. It has shown 0 since March, and the fleet is not that healthy. The
gateway logs `level=ERROR`, the GPU nodes log `lvl=err`, HUSH's own agent logs
`severity=critical`, and the panel counts only lines with exactly `level=error`. Noor wants the
real numbers from yesterday's logs. In this exercise you write the small library behind that
count: split a line into its keys and values, map 15 spellings to 5 levels, count them, and keep
the lines that matter.

## Task

Implement the stubs in `loglevels.go`:

```go
// Fields splits 1 line into its key=value words. The last occurrence of a key wins.
func Fields(line string) map[string]string

// Level returns "debug", "info", "warn", "error", "fatal" or "unknown".
func Level(line string) string

// CountLevels counts the lines per level, "unknown" included.
func CountLevels(lines []string) map[string]int

// AtLeast returns the lines at minLevel or more severe, in input order.
func AtLeast(lines []string, minLevel string) ([]string, error)
```

Suggested order: `Fields`, `Level`, `CountLevels`, `AtLeast`. Each one can use the ones before it.

## Rules

### Fields

- A line is a list of words separated by whitespace, exactly as
  [`strings.Fields`](https://pkg.go.dev/strings#Fields) splits it. Any run of spaces, tabs,
  newlines and other whitespace separates 2 words. This includes the `\r` of a Windows line
  ending. `Fields` ignores whitespace before the first word and after the last word.
- Each word splits at its **first** `=`. The part before it is the key, the part after it is the
  value. `query=a=b=c` is the key `query` with the value `a=b=c`.
- `key=` is the key with the empty value `""`.
- A word without `=` is a key with the value `""`. `boot` is the key `boot` with the value `""`.
- A word that starts with `=` has no key. `Fields` skips it: `=x`, `=` and `==x`.
- Keys are case-sensitive: `level`, `LEVEL` and `Level` are 3 different keys.
- If a key appears more than once, its **last** occurrence wins. This also holds when one
  occurrence is a bare key: `retry=3 retry` gives `retry` with `""`.
- Quotes are ordinary characters. The lines in this exercise have no values with spaces in them.
  `msg="disk full"` is 2 words: the key `msg` with the value `"disk`, and the bare key `full"`.
  Quoted values are the job of 26-logfmt.
- A line without words (empty or whitespace only) gives no entries. Both a nil map and an empty
  map are fine.

### Level

- `Level` looks for 3 keys in this order: `level`, `lvl`, `severity`. The first key that is
  **present** in the line decides, wherever it stands in the line. Its value decides even when it
  is unknown or empty: `level=bogus lvl=error` is `unknown`.
- The keys are case-sensitive, as in `Fields`: `LEVEL=error` has none of the 3 keys.
- If the deciding key appears more than once, its **last** occurrence decides, as in `Fields`:
  `level=info level=error` is `error`.
- The value matches the table below in any letter case: `ERROR`, `Error` and `eRr` are all
  `error`.

  | Level | Spellings |
  |---|---|
  | `debug` | `debug`, `dbg`, `trace` |
  | `info` | `info`, `information`, `notice` |
  | `warn` | `warn`, `warning` |
  | `error` | `error`, `err` |
  | `fatal` | `fatal`, `crit`, `critical`, `panic`, `emerg` |

- No key, an empty value or any other value gives `unknown`: `level=`, `level=errors`, `level=3`,
  `level=verbose`.

### CountLevels

- `CountLevels` counts the lines per level, as `Level` returns it. `unknown` is a level like the
  others, and `CountLevels` counts it.
- `CountLevels` skips a line without words (empty or whitespace only). It does not count it, not
  even as `unknown`.
- A line with at least 1 word counts, even when `Fields` gives no entries for it: `=x` is
  `unknown`.
- The map has no keys other than the 6 levels. A level with 0 lines may be absent or have the
  value 0.
- The map is never nil, also when there are no lines to count.

### AtLeast

- The levels are ordered `debug` < `info` < `warn` < `error` < `fatal`. `AtLeast` returns the
  lines whose level is `minLevel` or later in this order.
- `AtLeast` never returns a line with the level `unknown`, and never a line without words.
- The lines come back unchanged (trailing spaces included) and in input order.
- When no line matches, the error is nil and the slice is empty. Both a nil slice and an empty
  slice are fine.
- `minLevel` must be exactly 1 of the 5 lowercase names `debug`, `info`, `warn`, `error` and
  `fatal`. `AtLeast` does not accept the other spellings of the table here: `warning`, `err`,
  `WARNING`, `Warn`, `unknown`, `""` and ` warn` are all invalid.
- For an invalid `minLevel`, `AtLeast` returns a **nil** slice and an error. The message contains
  `minLevel` formatted with `%q`, for example `unknown minimum level "WARNING"`. The rest of the
  wording is yours.

## Examples

```text
Fields(`ts=2026-09-01T10:00:03Z service=gateway level=ERROR msg=disk_full`)
  → {"ts": "2026-09-01T10:00:03Z", "service": "gateway", "level": "ERROR", "msg": "disk_full"}

Fields("  lvl=info boot =x query=a=b=c lvl=err\t")
  → {"lvl": "err", "boot": "", "query": "a=b=c"}

Fields(`msg="disk full" level=warn`)
  → {"msg": `"disk`, `full"`: "", "level": "warn"}

Fields("")  → {} (or nil)
```

```text
Level("service=gateway level=ERROR msg=disk_full")     → "error"
Level("node=vl-gpu-0412 lvl=err")                      → "error"
Level("service=hushd severity=critical")               → "fatal"
Level("lvl=err level=info")                            → "info"     (level comes before lvl)
Level("level=info level=error")                        → "error"    (the last level wins)
Level("level=bogus lvl=error")                         → "unknown"  (level is present; its value decides)
Level("level lvl=error")                               → "unknown"  (the bare key level has the value "")
Level("LEVEL=error")                                   → "unknown"  (keys are case-sensitive)
Level("node=vl-gpu-0451 msg=boot")                     → "unknown"  (no level key)
```

```text
lines:
  service=gateway level=INFO msg=started
  node=vl-gpu-0412 lvl=err msg=ecc_error
  (an empty line)
  node=vl-gpu-0437 lvl=dbg msg=fan_event
  service=hushd severity=critical msg=model_oom
  node=vl-gpu-0451 msg=boot

CountLevels(lines) → {"debug": 1, "info": 1, "error": 1, "fatal": 1, "unknown": 1}

AtLeast(lines, "warn")
  → ["node=vl-gpu-0412 lvl=err msg=ecc_error",
     "service=hushd severity=critical msg=model_oom"], nil

AtLeast(lines, "WARNING") → nil, error `loglevels: unknown minimum level "WARNING"`
```

`testdata/fleet.log` holds 24 lines from the inference fleet and 3 blank lines. None of them has
exactly `level=error`, and 11 of them are at `error` or worse.

## Run the tests

    go test ./exercises/01-loglevels/
    go test -run TestFields -v ./exercises/01-loglevels/        # step 1: Fields
    go test -run TestLevel -v ./exercises/01-loglevels/         # step 2: Level
    go test -run TestCountLevels -v ./exercises/01-loglevels/   # step 3: CountLevels
    go test -run TestAtLeast -v ./exercises/01-loglevels/       # step 4: AtLeast
    go test -run TestLevelKeyOrder -v ./exercises/01-loglevels/ # one test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The 4 filters above do not overlap. Steps 3 and 4 call `Level`, so they need
step 2 to work. The example (`ExampleCountLevels`) runs only in a plain `go test`, so a plain run
crashes until all 4 steps work.

## Docs you can use

- [`strings.Fields`](https://pkg.go.dev/strings#Fields), [`strings.Cut`](https://pkg.go.dev/strings#Cut),
  [`strings.ToLower`](https://pkg.go.dev/strings#ToLower) and
  [`strings.TrimSpace`](https://pkg.go.dev/strings#TrimSpace)
- [Go maps in action](https://go.dev/blog/maps) (Go blog)
- [The Go spec: Index expressions](https://go.dev/ref/spec#Index_expressions), on reading a map
  with the "comma ok" form
- [The Go spec: Switch statements](https://go.dev/ref/spec#Switch_statements), on a `case` with
  several values
- [Effective Go: Maps](https://go.dev/doc/effective_go#maps) and
  [Multiple return values](https://go.dev/doc/effective_go#multiple-returns)
- [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) and the [`%q` verb](https://pkg.go.dev/fmt#hdr-Printing)
- [Testable Examples in Go](https://go.dev/blog/examples) (Go blog), on how `ExampleCountLevels`
  checks its output
- [RFC 5424, section 6.2.1](https://www.rfc-editor.org/rfc/rfc5424#section-6.2.1): the syslog
  severities, where `emerg`, `crit`, `err` and `notice` come from

## Stretch goals

- Turn it into a command: read a log from stdin with `bufio.Scanner` and print the count per
  level, from `debug` to `unknown`, in that fixed order. Try it on `testdata/fleet.log`.
- Count per service: a `map[string]map[string]int` from the value of `service` (or `node`) to
  the counts. Which service has the most errors in `testdata/fleet.log`?
- Accept numeric levels: syslog writes 0 to 7 (`level=3` is `err`), and the Node.js logger
  Bunyan writes 10 to 60 (`level=50` is `error`). Decide what to do when a line could mean both.
- Print the share of `unknown` lines. HUSH's agent moved from `level=` to `severity=` in March,
  the month the panel went to 0. A rising share tells you that a service changed how it logs,
  before a panel shows 0 for half a year.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 01 hint 1
    go run ./cmd/peek 01 hint 2
    go run ./cmd/peek 01 hint 3
    go run ./cmd/peek 01 explanation
    go run ./cmd/peek 01 walkthrough
    go run ./cmd/peek 01 solution
