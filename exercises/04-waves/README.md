# 04 · The canary got the update twice

**Tier 1 · about 1.5 h** · Go: slices share backing arrays, `append` and capacity, `copy` and `slices.Clone`, `make` with a length and a capacity, `slices.SortFunc` · Problem solving: decide who owns each slice; write a test that catches sharing on purpose

Daniyar Seitkali, the CTO, planned the rollout of a new agent for HUSH, the on-call AI of
Vesper Labs, to the 300 older nodes of Hall 2 between 2 meetings, on a wet September afternoon:
`sort.Strings(names)`, waves of 50 with `names[i:i+50]`, and the canary node `canary-h3-01`
appended to wave 1. The deploy log showed 2 surprises. The canary got the update twice, once
in wave 1 and once in wave 2, and `node-144`, the node that should have opened wave 2, never
got it. And `node-10` went out before `node-9`, because a string sort compares the `1` of `10`
with the `9`. Daniyar read the log twice and said "But I never touched wave 2." He was right:
`append` did. The waves were windows into 1 array, wave 1 had spare capacity, and the canary
landed in the slot where wave 2 started. His action item: the next rollout is planned by code
that has tests. In this exercise you write that code: order the node names the way people read
them, drop the nodes to skip, and cut the waves so that no wave can change another.

## Task

Implement the stubs in `waves.go`:

```go
func Order(names []string) []string                              // natural order, in a new slice
func Without(names, skip []string) []string                      // the names not in skip, in a new slice
func Waves(names []string, canary, size int) ([][]string, error) // a canary wave, then waves of size names
```

Suggested order: `Order`, then `Without`, then `Waves`.

## Rules

### Who owns a slice

All 3 functions follow the same rule: the caller owns the input, and the caller owns the result.

- A function does not modify its input. It does not write into the array behind `names`, not even
  into its spare capacity beyond `len(names)`.
- Changing an element of a result, or appending to it, never changes the input or another result.
- Changing the input after the call never changes a result.

The tests check this on purpose. They pass slices with spare capacity, append to the results and
change their elements.

### Order

- A name is a **prefix** followed by a **number**. The number is the run of ASCII digits `0`–`9`
  at the end of the name, as long as possible. The run may be empty.

  | Name | Prefix | Number |
  |---|---|---|
  | `node-0412` | `node-` | `0412` |
  | `rack2-node10` | `rack2-node` | `10` |
  | `web` | `web` | none |
  | `v10a` | `v10a` | none |
  | `10` | empty | `10` |

- 2 names compare like this:
  1. The prefixes, in plain string order (`<` on strings, byte by byte).
  2. For equal prefixes, a name without a number comes first: `web` before `web1`.
  3. Then the smaller number value comes first: `node-9` before `node-10`.
  4. For equal values, plain string order: `node-007` before `node-07` before `node-7`.
- Only the run at the end is a number. Digits earlier in the name are part of the prefix and
  compare as text: `rack10-node2` comes before `rack2-node1`, because the prefix
  `rack10-node` is smaller than `rack2-node` as a string.
- Plain string order compares bytes, so letter case counts: `Web-2` comes before `web-1`.
- The tests use runs of at most 9 digits, so every number fits in an `int`.
- The result has the same names as the input, duplicates included, in this order.
- For empty input (nil or a slice of length 0) the result is empty: nil or an empty slice are both
  fine.

### Without

- `Without` returns the names of `names` that are not in `skip`, in the order of `names`.
- Names compare byte by byte: letter case counts, and a name must match whole.
- `Without` drops every copy of a name that is in `skip`, and keeps every copy of any other name.
- A name in `skip` that is not in `names` changes nothing. An empty `skip` (nil or length 0) gives
  a copy of `names`.
- When no name is left, the result is empty: nil or an empty slice are both fine.

The Go wiki shows a filter that reuses the array of its input. It starts with
`kept := names[:0]` and calls `kept = append(kept, name)` for every name to keep. It allocates
nothing, but it overwrites `names` while it runs. Filter `a` out of `[a b c]` that way, and the
caller's slice holds `[b c c]`. That is fine when nobody needs the input any more. Here the caller
still needs `names`.

### Waves

- `canary < 0` is an error, and `size < 1` is an error. The error message contains the bad value
  as a decimal number, for example `waves: canary -1 is negative` or
  `waves: size 0 is less than 1`. The wording around the number is yours.
- On an error, the waves are nil. The checks also run when `names` is empty.
- The first wave is the **canary wave**: the first `canary` names. With `canary` 0 there is no
  canary wave. When `names` is not empty and `canary` is `len(names)` or more, the result is 1
  wave with every name.
- The other names follow in waves of `size` names. The last wave may be shorter.
- No wave is empty. Every name is in exactly 1 wave. Put together, the waves hold `names` in their
  order, duplicates included: `Waves` does not sort or filter.
- Empty `names` (nil or length 0) with valid arguments give no waves (nil or an empty slice) and a
  nil error.
- Every wave is independent: appending to a wave, or changing one of its names, never changes
  another wave or `names`.

## Examples

```text
Order(["node-10", "node-9", "web", "node-007", "node-7", "web1", "checkout-2"])
  → ["checkout-2", "node-007", "node-7", "node-9", "node-10", "web", "web1"]

Order(["rack2-node1", "rack10-node2", "10", "9"])
  → ["9", "10", "rack10-node2", "rack2-node1"]

Without(["node-1", "canary-h3-01", "node-2", "node-1"], ["canary-h3-01", "node-404"])
  → ["node-1", "node-2", "node-1"]
```

With `names` = `node-1` to `node-10`:

```text
Waves(names, 1, 3)  → [[node-1] [node-2 node-3 node-4] [node-5 node-6 node-7] [node-8 node-9 node-10]], nil
Waves(names, 0, 4)  → [[node-1 ... node-4] [node-5 ... node-8] [node-9 node-10]], nil
Waves(names, 12, 3) → [[node-1 ... node-10]], nil
Waves(names, 1, 50) → [[node-1] [node-2 ... node-10]], nil
Waves(nil, 1, 50)   → [], nil
Waves(names, -1, 3) → nil, error "waves: canary -1 is negative"
Waves(nil, 1, 0)    → nil, error "waves: size 0 is less than 1"
```

A complete plan, with the canary node of Hall 3 in a wave of its own:

```go
plan := append([]string{"canary-h3-01"}, Order(Without(hall2, skip))...)
w, err := Waves(plan, 1, 50)
```

What a caller may do with the result:

```go
backing := []string{"node-1", "node-2", "node-3", "node-4", "SPARE"}
names := backing[:4]           // length 4, capacity 5
w, _ := Waves(names, 1, 2)     // [[node-1] [node-2 node-3] [node-4]]
w[0] = append(w[0], "canary-h3-01")
w[2] = append(w[2], "node-5")
w[1][0] = "changed"
// w is [[node-1 canary-h3-01] [changed node-3] [node-4 node-5]]
// backing is still [node-1 node-2 node-3 node-4 SPARE]
```

## Run the tests

    go test ./exercises/04-waves/
    go test -run TestOrder -v ./exercises/04-waves/               # step 1: Order
    go test -run TestWithout -v ./exercises/04-waves/             # step 2: Without
    go test -run TestWaves -v ./exercises/04-waves/               # step 3: Waves
    go test -run TestWavesIndependent -v ./exercises/04-waves/    # one test

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The 3 step filters above do not overlap.

`TestWavesErrors` calls `Waves` with a 5 s limit. A loop that steps by a size of 0 never ends, so
the test tries the bad values with empty `names` first. The 3 examples (`ExampleOrder`,
`ExampleWithout`, `ExampleWaves`) match none of the step filters. They run in a plain `go test`,
or with `-run Example`. An example cannot recover a panic, so these runs crash until all 3 steps
work.

## Docs you can use

- [A Tour of Go: Slices are like references to arrays](https://go.dev/tour/moretypes/8),
  [Slice length and capacity](https://go.dev/tour/moretypes/11) and
  [Creating a slice with make](https://go.dev/tour/moretypes/13)
- [Go Slices: usage and internals](https://go.dev/blog/slices-intro) (Go blog)
- [Arrays, slices (and strings): The mechanics of 'append'](https://go.dev/blog/slices) (Go blog)
- [The Go spec: Slice expressions](https://go.dev/ref/spec#Slice_expressions), including full
  slice expressions, and [Appending to and copying slices](https://go.dev/ref/spec#Appending_and_copying_slices)
- [`slices`](https://pkg.go.dev/slices): `Clone`, `Contains`, `SortFunc`
- [`cmp`](https://pkg.go.dev/cmp): `Compare`, `Or`
- [`strconv.Atoi`](https://pkg.go.dev/strconv#Atoi)
- [Go wiki: SliceTricks](https://go.dev/wiki/SliceTricks), the filter "without allocating" and
  what it does to its input
- [Google SRE workbook: Canarying releases](https://sre.google/workbook/canarying-releases/)

## Stretch goals

- Make `Without` fast for a long skip list: 40 000 names and 5000 to skip. Write a benchmark and
  measure before and after.
- Waves by percentage, as progressive delivery tools do it: `WavesByPercent(names, []int{1, 10,
  50, 100})` gives a wave up to 1% of the names, then up to 10%, and so on.
- Natural order for numbers of any length: compare 2 runs of digits without converting them to an
  `int`.
- Natural order for every run of digits, so that `rack2-node10` comes before `rack10-node2`,
  the way `sort -V` and `ls -v` do it.
- Make it a tool: `waves -canary canary-h3-01 -size 50 -skip node-404 < hall2.txt` prints 1
  line per wave.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 04 hint 1
    go run ./cmd/peek 04 hint 2
    go run ./cmd/peek 04 hint 3
    go run ./cmd/peek 04 explanation
    go run ./cmd/peek 04 walkthrough
    go run ./cmd/peek 04 solution
