# 12 · 1 filter, not 6 greps

**Tier 3 · about 1.5 h** · Go: function types, closures that capture variables, functions that return functions, variadic parameters, injected randomness, `strings.Fields` · Problem solving: build behaviour from small pieces; decide what an empty list means

`grep gateway app.log | grep -v healthz | grep -v canary-h3-01 | grep -v hushd | grep -i error | head -50`
is step 3 of the runbook "inference slow". Daniyar Seitkali, the CTO, wrote most of it during an
incident in 2024, and nobody has dared to touch it since. Last night you ran it and got 50 lines.
37 were the gateway's stats line, `msg=stats gateway_requests=212 errors=0`, because
`grep -i error` matches those 5 letters anywhere, and `head` cut off the real errors. HUSH, the AI
that Vesper Labs sells, offered in the assistant panel: "I can write that for you. It would take
me 0.4 seconds." Noor Haddad, the principal SRE, closed the panel and read the 37 zeros: "It found
the word. It did not find the errors." The fix is 1 `-filter` flag in the team's own log tool: a
term matches a whole `key=value` word, and you can keep the first 50 lines or a random 1% of them.
In this exercise you write the library behind that flag: small line matchers, ways to combine
them, and a parser for 1 filter expression.

## Task

Implement the stubs in `linefilter.go`. The stub already declares the type `Matcher`.

```go
type Matcher func(line string) bool

func Contains(sub string) Matcher                     // the line contains sub
func Field(key, value string) Matcher                 // a word of the line is exactly key=value
func Not(m Matcher) Matcher                           // the opposite of m
func All(ms ...Matcher) Matcher                       // every matcher selects the line; true for none
func Any(ms ...Matcher) Matcher                       // at least 1 matcher selects it; false for none
func Limit(n int, m Matcher) Matcher                  // the first n lines that m selects, then none
func Sample(rate float64, rnd func() float64) Matcher // the line when rnd() < rate

func Parse(expr string) (Matcher, error)              // "gateway !healthz level=error|level=warn"
func Filter(lines []string, m Matcher) []string       // the selected lines, in order
```

Suggested order: `Contains` and `Field`, then `Not`, `All` and `Any`, then `Limit` and `Sample`,
then `Parse`, then `Filter`. The tests follow the same 5 steps.

## Rules

### Contains and Field

- `Contains(sub)` selects a line that contains `sub`, with the same letter case. `Contains("")`
  selects every line, the empty line too.
- `Field(key, value)` selects a line when 1 of its words is exactly `key=value`. White space
  separates the words, as `strings.Fields` splits them: spaces, tabs and also the `\r` of a
  Windows line ending. The word must match whole and with the same letter case: for
  `Field("level", "error")` the words `level=errors`, `loglevel=error`, `level=ERROR` and
  `msg=level=error` do not match.
- `Field` does not understand quotes. `level="error"` is not the word `level=error`, and
  `msg="disk full"` is the 2 words `msg="disk` and `full"`. Quoted values are the job of a real
  logfmt parser (26-logfmt).

### Not, All and Any

- `Not(m)` selects exactly the lines that `m` does not select. It calls `m` once per line.
- `All(ms...)` selects a line when every matcher selects it. It calls the matchers in order, each
  with the same line, and stops at the first one that returns `false`: the matchers after it are
  not called.
- `Any(ms...)` selects a line when at least 1 matcher selects it. It calls the matchers in order
  and stops at the first one that returns `true`.
- With no matchers, `All()` selects every line and `Any()` selects none. A nil slice passed as
  `All(list...)` counts as no matchers.
- `All` and `Any` keep their own copy of the list. If the caller passes a slice with `list...` and
  changes an element of it afterwards, the Matcher does not change.
- Where a matcher stands inside `All` or `Any` decides which lines it sees. This matters for
  `Limit`: see the Examples.

### Limit and Sample

- `Limit(n, m)` selects the first `n` lines that `m` selects, and no line after those. After it
  has selected `n` lines, it no longer calls `m`.
- With `n <= 0` it selects no line and never calls `m`.
- Every Matcher that `Limit` returns has its own count. 2 calls of `Limit` with the same `m` give 2
  independent counts.
- `Sample(rate, rnd)` selects a line when `rnd() < rate`. `rnd` returns a number from 0 up to but
  not including 1, as `math/rand/v2.Float64` does. The tests pass a function that returns
  scripted numbers.
- The Matcher from `Sample` calls `rnd` exactly once per line, with 2 exceptions: with
  `rate <= 0` it selects no line, and with `rate >= 1` it selects every line. In both cases it
  never calls `rnd`.
- The matchers from `Limit` and `Sample`, and every Matcher built from them, keep state between
  calls. A Matcher is not safe for concurrent use.

### Parse

- The expression is a list of **terms** separated by white space, as `strings.Fields` splits it.
  The expression selects a line when every term selects it. An empty or white-space-only
  expression selects every line.
- A term is 1 or more **alternatives** separated by `|`. The term selects a line when at least 1
  alternative selects it.
- A `!` at the start of a term negates the term: it selects a line when none of its alternatives
  selects it. The `!` belongs to the whole term: `!healthz|readyz` drops the lines with `healthz`
  and the lines with `readyz`.
- An alternative that contains `=` works like `Field`. Its first `=` splits it: `q=a=b` is
  `Field("q", "a=b")`. Any other alternative works like `Contains`: `gateway` is
  `Contains("gateway")`.
- A `!` anywhere but at the start of a term is an ordinary byte: `wow!` is `Contains("wow!")`.
- Terms are case-sensitive, like `Contains` and `Field`.

### Parse: errors

`Parse` returns a nil Matcher and an error when a term breaks 1 of these rules:

| Problem | Bad terms |
|---|---|
| a `!` with nothing after it | `!` |
| an empty alternative | `a\|\|b`, `\|a`, `a\|`, `\|`, `!\|a` |
| an alternative that starts with `!` (a `!` negates only the whole term) | `!!x`, `a\|!b` |
| an alternative with `=` but an empty key or an empty value | `=v`, `k=`, `=`, `!k=`, `level=error\|=warn` |

- The error message contains the whole bad term, quoted with `%q`: for `gateway a||b` it contains
  `"a||b"`. The rest of the wording is yours; the tests do not check it.
- `Parse` checks the terms from left to right and reports only the first bad term. For
  `gateway a||b =v` the error names `"a||b"` and not `"=v"`.

### Filter

- `Filter(lines, m)` returns the lines that `m` selects, in their input order.
- It calls `m` exactly once per line, from the first line to the last. So
  `Filter(lines, Limit(50, m))` is `head -50`.
- It never changes `lines`: not an element, not the order.
- It returns nil when it selects no line, also for an empty or nil `lines`.

## Examples

The runbook pipeline as 1 expression, plus `head`:

```text
m, _ := Parse("gateway !healthz !host=canary-h3-01 level=error|level=warn")
Filter(lines, Limit(2, m))

lines:
  03:10:02 level=info svc=gateway msg=stats errors=0                    no: not level=error or level=warn
  03:10:03 level=error svc=gateway host=vl-gpu-0412 msg=timeout         yes
  03:10:05 level=error svc=gateway host=canary-h3-01 msg=ecc_corrected  no: host=canary-h3-01
  03:10:05 level=error svc=gateway path=/healthz                        no: healthz
  03:10:08 level=warn svc=gateway host=vl-gpu-0448 msg=slow             yes
  03:10:10 level=error svc=gateway host=vl-gpu-0469 msg=disk_full       no: the limit of 2 is reached
```

Where a `Limit` stands inside `All` decides what it counts:

```text
lines:                        svc=hushd level=error   svc=gateway level=error   svc=gateway level=error
All(gateway, Limit(1, err))   false                   true                      false
All(Limit(1, err), gateway)   false                   false                     false
```

with `gateway = Contains("svc=gateway")` and `err = Contains("level=error")`. In the second row
`All` asks the `Limit` first, so the hushd line uses up its count of 1.

`Sample(0.1, rnd)` on 4 lines, with `rnd` returning 0.05, 0.1, 0.5 and 0.95:

```text
true, false, false, false        (0.1 < 0.1 is false)
```

`Parse` errors:

```text
Parse("gateway !")         → nil, an error that contains "!"
Parse("!!x")               → nil, an error that contains "!!x"
Parse("gateway a||b =v")   → nil, an error that contains "a||b"
Parse("k=")                → nil, an error that contains "k="
```

## Run the tests

    go test ./exercises/12-linefilter/
    go test -run TestBasic -v ./exercises/12-linefilter/                    # step 1: Contains and Field
    go test -run TestCombine -v ./exercises/12-linefilter/                  # step 2: Not, All and Any
    go test -run TestStateful -v ./exercises/12-linefilter/                 # step 3: Limit and Sample
    go test -run TestParse -v ./exercises/12-linefilter/                    # step 4: Parse
    go test -run TestFilter -v ./exercises/12-linefilter/                   # step 5: Filter
    go test -run TestStatefulLimitInsideAll -v ./exercises/12-linefilter/   # one test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The 5 step filters above do not overlap. Each step also uses the functions of the
steps before it. `TestParseRunbook` runs the runbook expression on `testdata/gateway.log` and
names the lines it expects by their line number. `ExampleParse` runs only in a plain `go test`
and needs `Parse`, `Limit` and `Filter`.

## Docs you can use

- [A Tour of Go: Function values](https://go.dev/tour/moretypes/24) and [Function closures](https://go.dev/tour/moretypes/25)
- [The Go spec: Function types](https://go.dev/ref/spec#Function_types), [Function literals](https://go.dev/ref/spec#Function_literals) and [Passing arguments to `...` parameters](https://go.dev/ref/spec#Passing_arguments_to_..._parameters)
- [Codewalk: First-Class Functions in Go](https://go.dev/doc/codewalk/functions/)
- [Fixing For Loops in Go 1.22](https://go.dev/blog/loopvar-preview) (Go blog), on what a closure in a loop captures
- [`strings`](https://pkg.go.dev/strings): `Contains`, `Fields`, `Split`, `Cut`, `CutPrefix`
- [`slices`](https://pkg.go.dev/slices): `Clone`
- [`math/rand/v2`](https://pkg.go.dev/math/rand/v2): `Float64`, and `rand.New(rand.NewPCG(1, 2))` for a seeded source
- [Grafana Loki: Log queries](https://grafana.com/docs/loki/latest/query/log_queries/), the section on line filter expressions

## Stretch goals

- Make it a tool: a command that reads stdin and prints the selected lines, with
  `-filter 'gateway !healthz level=error'`, `-head 50` and `-sample 0.01`. `flag.Func`
  (15-checkdisk) lets `Parse` check the expression while `FlagSet.Parse` runs. Step 3 of the
  runbook then becomes 1 command instead of 6.
- Add a case-insensitive term, for example `~error`, that compares with `strings.EqualFold` on
  words or lower-cases the line once.
- Add a regular expression term with `regexp`. Compile it once in `Parse`, not once per line.
- Sample by request, not by line: keep every line of 1% of the request IDs, with a hash of the
  `request_id` value (`hash/fnv`) instead of `rnd`. Then a sampled request is complete.
- After 26-logfmt, make `Field` understand quoted values.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 12 hint 1
    go run ./cmd/peek 12 hint 2
    go run ./cmd/peek 12 hint 3
    go run ./cmd/peek 12 explanation
    go run ./cmd/peek 12 walkthrough
    go run ./cmd/peek 12 solution
