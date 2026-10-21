# 06 · Agent v2.10 is newer than v2.9

**Tier 2 · about 1 h** · Go: structs with methods and value receivers, a `String()` method, a compare function that returns -1, 0 or +1, sentinel errors with `%w` and `errors.Is`, `strings.Split`, `strconv.Atoi` · Problem solving: define the order before you code; test with the cases that a string comparison gets wrong

At 07:40, in your first week on the rotation, with the Key clipped to the steel ring on your
lanyard, Marisol Quispe Mamani, the support lead, has 212 new tickets. Ticket 212 reads "your AI
wrote a poem about my outage. it was a good poem. the outage was not." Vesper Labs ships HUSH,
its on-call AI, as an agent in each customer's cluster, and the nightly release script picks the
newest agent with `git tag | sort | tail -1`, a line copied from a blog post in 2022. `sort`
compares text, 1 character at a time, and `9` comes after `1`. So at 02:00 the script decided
that `v2.9.3` is newer than `v2.10.0` and "upgraded" 14 000 customer agents to `v2.9.3`, the old
build that ends every incident summary with a haiku. The tag list also holds
`v2.11.0-selfupdate`, a pre-release that nobody on the team remembers pushing. You roll the
agents forward and want a script that knows that 10 is more than 9. In this exercise you write
the version type behind that script, and the function that picks the newest release.

## Task

Implement the stubs in `semver.go`. The file already declares `ErrInvalid`, `ErrNoRelease` and the
`Version` type.

```go
var ErrInvalid = errors.New("semver: invalid version") // given
var ErrNoRelease = errors.New("semver: no release")    // given

type Version struct {
	Major, Minor, Patch int
	Pre                 string // pre-release without the "-", for example "rc.1"; "" for a release
}

func Parse(s string) (Version, error)       // "v2.10.0", "2.10.0", "v2.10.0-rc.1"
func (v Version) String() string            // "v2.10.0", "v2.10.0-rc.1"
func (v Version) IsRelease() bool           // Pre == ""
func Compare(a, b Version) int              // -1 if a < b, 0 if equal, +1 if a > b
func Latest(tags []string) (Version, error) // the newest release among the tags that parse
```

Suggested order: `Parse`, `String` and `IsRelease` first, then `Compare`, then `Latest`.

## Rules

### Parse

- A version has the form `[v]MAJOR.MINOR.PATCH[-PRE]`. The parts in brackets are optional.
- The `v` at the start is optional and lowercase. `V2.10.0` is an error.
- There are exactly 3 numbers, separated by `.`. `2.10` and `v2.10.0.1` are errors.
- Each number is 1 or more ASCII digits `0` to `9`:
  - no sign: `v2.-1.0` and `v2.+1.0` are errors;
  - no leading zero: `0` is fine, `02` and `00` are not;
  - no empty number: `v2..0` is an error;
  - it fits in an `int`: a major of 20 digits is an error.
- An optional pre-release follows the numbers: a `-` and then 1 or more bytes, each an ASCII
  letter (`A` to `Z`, `a` to `z`), a digit, `.` or `-`. `Pre` holds it without the leading `-`.
  `v2.1.0-` is an error. The first `-` starts the pre-release, and later ones belong to it:
  `v1.0.0-beta-2` has the `Pre` `beta-2`.
- Nothing else is allowed: no build metadata (`v2.1.0+build`), no whitespace before, after or
  inside the version, no other bytes.
- On success the error is `nil`. On error `Parse` returns the zero `Version` and an error that
  wraps `ErrInvalid` (`errors.Is` works) and contains the input formatted with `%q`, for example
  `semver: invalid version "v2.10": want 3 numbers, got 2`. The rest of the wording is yours.

### String and IsRelease

- `String` returns `v`, the 3 numbers joined with `.`, and, for a pre-release, `-` and `Pre`:
  `v2.10.0`, `v2.10.0-rc.1`. It always writes the lowercase `v`, also when the input had none.
- `Version` is a `fmt.Stringer`, so `fmt` uses `String`: `fmt.Sprint(v)` is `v2.10.0`.
- Round trip: for every `v` that `Parse` returns, `Parse(v.String())` returns `v` again.
- `IsRelease` reports whether `Pre` is `""`.

### Compare

- `Compare` returns exactly -1, 0 or +1: -1 when `a` is older than `b`, 0 when they are equal,
  +1 when `a` is newer.
- It compares `Major` first, then `Minor`, then `Patch`, as numbers: `v2.10.0` is newer than
  `v2.9.3`, and `v2.0.99` is older than `v2.1.0`.
- When the 3 numbers are equal, a release is newer than any pre-release of the same numbers:
  `v1.0.0-rc.1` is older than `v1.0.0`.
- When the 3 numbers are equal and both are pre-releases, `Pre` decides with plain string order
  (Go's `<` on strings, byte by byte): `alpha` < `alpha.1` < `beta` < `rc.1` < `rc.2`, and
  `RC.1` < `rc.1`.
- So `v2.10.0` and `2.10.0` compare as 0: they parse to the same `Version`.
- `slices.SortFunc(versions, semver.Compare)` sorts versions oldest first.

This order is simpler than [SemVer 2.0.0](https://semver.org/#spec-item-11). SemVer splits a
pre-release at the dots and compares numeric parts as numbers, so in SemVer `rc.10` is newer than
`rc.9`. Plain string order says the opposite: the same mistake as the release script. SemVer also
forbids empty parts (`rc..1`) and leading zeros in numeric parts (`rc.01`). This exercise does not
check them. Every test compares pre-releases where both rules give the same answer. The full rule
is a stretch goal.

### Latest

- `Latest` parses each tag with `Parse`. It skips tags that `Parse` rejects and tags that are
  pre-releases.
- It returns the newest of the remaining releases, as `Compare` orders them, and `nil`. The order
  of the tags does not matter.
- Tags that parse to the same version, such as `v2.10.0` and `2.10.0`, give the same `Version`,
  so it does not matter which one wins.
- If no release is left (a nil or empty slice, only invalid tags, only pre-releases), `Latest`
  returns the zero `Version` and an error that wraps `ErrNoRelease` (`errors.Is` works).
- `Latest` does not change the slice it gets.

## Examples

The examples use a short notation. Inside an expression, `Parse(x)` stands for the `Version` that
`Parse` returns for `x`, and `[...]` stands for a `[]string`.

```text
Parse("v2.10.0")         → Version{Major: 2, Minor: 10, Patch: 0, Pre: ""}, nil
Parse("2.9.3")           → Version{Major: 2, Minor: 9, Patch: 3, Pre: ""}, nil
Parse("v2.10.0-rc.1")    → Version{Major: 2, Minor: 10, Patch: 0, Pre: "rc.1"}, nil
Parse("v2.10")           → Version{}, error `semver: invalid version "v2.10": ...` (wraps ErrInvalid)
Parse("v02.1.0")         → Version{}, error (leading zero)
Parse("v2.1.0+build.5")  → Version{}, error (build metadata)

Parse("2.10.0").String()                             → "v2.10.0"
Compare(Parse("v2.9.3"), Parse("v2.10.0"))           → -1
Compare(Parse("v1.0.0"), Parse("v1.0.0-rc.1"))       → +1
Compare(Parse("v1.0.0-alpha"), Parse("v1.0.0-beta")) → -1

Latest(["v2.9.3", "v2.10.0", "v2.11.0-rc.1", "latest"]) → v2.10.0, nil
Latest(["v2.11.0-rc.1", "latest"])                      → Version{}, error (wraps ErrNoRelease)
```

`testdata/tags.txt` holds the 40 tags of the agent repository on the night of the incident, in the
order `git tag` prints them. `sort | tail -1` gives `v2.9.3`. `Latest` gives `v2.10.0`.

## Run the tests

    go test ./exercises/06-semver/
    go test -run 'TestParse|TestString|TestIsRelease' -v ./exercises/06-semver/   # step 1: Parse, String, IsRelease
    go test -run TestCompare -v ./exercises/06-semver/                           # step 2: Compare
    go test -run TestLatest -v ./exercises/06-semver/                            # step 3: Latest
    go test -run TestParseInvalid -v ./exercises/06-semver/                      # one test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The test names are chosen so that the 3 filters above do not overlap. Steps 2
and 3 build their versions with `Parse`, so finish step 1 first. The example (`ExampleLatest`)
runs only in a plain `go test`; an example cannot recover a panic, so a plain run crashes until
all 3 steps work.

## Docs you can use

- [A Tour of Go: Methods](https://go.dev/tour/methods/1) and [Stringers](https://go.dev/tour/methods/17)
- [The Go spec: Method declarations](https://go.dev/ref/spec#Method_declarations), on value and pointer receivers
- [`strings`](https://pkg.go.dev/strings): `CutPrefix`, `Cut`, `Split`, `Compare`
- [`strconv.Atoi`](https://pkg.go.dev/strconv#Atoi) and [`strconv.NumError`](https://pkg.go.dev/strconv#NumError): read what Atoi accepts
- [`errors`](https://pkg.go.dev/errors): `errors.New`, `errors.Is`
- [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) and the `%w` verb, and the [`%q` verb](https://pkg.go.dev/fmt#hdr-Printing)
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors) (Go blog)
- [`cmp`](https://pkg.go.dev/cmp): `Compare`, `Or`
- [`slices.SortFunc`](https://pkg.go.dev/slices#SortFunc)
- [Semantic Versioning 2.0.0](https://semver.org/), items 2, 9 and 11
- [`golang.org/x/mod/semver`](https://pkg.go.dev/golang.org/x/mod/semver): how the Go toolchain
  compares module versions. Read the docs, not the source, until you are done.

## Stretch goals

- Implement the full SemVer 2.0.0 precedence for pre-releases (`rc.9` < `rc.10`, numeric parts
  before letters) and its rules for pre-release parts. Add tests for the cases where it differs
  from plain string order.
- Accept build metadata (`v2.10.0+build.5`), keep it in a new field, and ignore it in `Compare`,
  as SemVer says.
- Write the command that replaces the line from the blog post before the script runs again at
  02:00: `git tag | latest-tag` prints the newest release and exits with status 1 when there is
  none. Then try `sort -V` (GNU sort's version sort) on `testdata/tags.txt`: which tag does it put
  last, and why is that still wrong?

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 06 hint 1
    go run ./cmd/peek 06 hint 2
    go run ./cmd/peek 06 hint 3
    go run ./cmd/peek 06 explanation
    go run ./cmd/peek 06 walkthrough
    go run ./cmd/peek 06 solution
