# 13 · Merge logs from many pods in time order

**Tier 3 · about 2 h** · Go: `iter.Seq` and `iter.Seq2`, range-over-func, `iter.Pull` and its `stop`, `container/heap`, `bufio.Scanner`, comparing `time.Time` · Problem solving: k-way merge, laziness and bounded memory, cleanup when the consumer stops early

Checkout failed for some users between 12:00 and 12:05. The checkout Deployment has 20 pods, and
`kubectl logs --timestamps` gives you 20 files, each in time order. To see what happened you need
one timeline: which pod logged the first timeout, which pod retried, when the database failover
started. `cat *.log | sort` works on small files, but a sort must read everything before it prints
the first line: with 20 files of 2 GB that is slow and eats memory, and with `kubectl logs -f` it
never prints anything, because the input never ends. Each file is already sorted, so you only
need to compare the next line of each of the 20 streams, again and again. That is a k-way merge,
the thing `sort -m` does. Logs from a log shipper (Fluent Bit, Promtail) have a
second problem: lines arrive slightly out of order, because they travel in batches over different
paths. A small reorder window fixes that, the same way Loki and Prometheus accept samples that
arrive a bit late. In this exercise you build all three pieces as lazy Go iterators.

## Task

Implement the stubs in `kubectl.go`, `reorder.go` and `logmerge.go`:

```go
type Entry struct {
	Time   time.Time // when the line was written
	Source string    // where it comes from, for example the pod name
	Line   string    // the message, without the timestamp and without the newline
}

// ParseKubectl parses the output of kubectl logs --timestamps.
func ParseKubectl(source string, r io.Reader) iter.Seq2[Entry, error]

type ParseError struct { Source string; Line int; Err error } // given, with Error and Unwrap

// Reorder repairs a stream that is slightly out of order, with a bounded window.
func Reorder(src iter.Seq[Entry], window time.Duration, stats *ReorderStats) iter.Seq[Entry]

type ReorderStats struct{ Late int } // entries that came out after a later entry

// Merge merges streams that are each in time order into one stream in time order.
func Merge(sources ...iter.Seq[Entry]) iter.Seq[Entry]
```

The three parts are independent. The suggested order goes from the simplest iterator to the
hardest: ParseKubectl produces a sequence, Reorder transforms one, Merge combines many.

## Rules

### All three

- Calling the function does nothing: no source is started, no byte is read. The work starts when
  you range over the result.
- They are lazy: they pull from their input only when they need to, so they work on endless input
  (`kubectl logs -f`). The tests use endless sources and count what was pulled.
- When the loop over a result stops early (`break`, `return`), the function stops its input before
  the loop ends. For an `iter.Seq` input that means: its yield returns false, so it returns and its
  deferred calls run (a source closes its file or HTTP body there). Never call yield again after
  it returned false; Go panics with `range function continued iteration after function for loop
  body returned false`.

### ParseKubectl

- Each line is a timestamp, one space and the message:
  `2026-10-16T12:00:00.123456789Z GET /healthz 200`.
- The timestamp is everything before the first space (the whole line if it has no space). Parse
  it with `time.RFC3339Nano`: the fraction of a second is optional and can have any number of
  digits, and an offset such as `+02:00` works as well as `Z`.
- The message is everything after the first space, unchanged: a stack trace line keeps its leading
  spaces. A line that is only a timestamp, with or without a space after it, has the message `""`.
- Each good line yields an Entry with `Source` set to `source`, and a nil error.
- A last line without a trailing newline is parsed like any other.
- An empty line is skipped: it yields nothing.
- A line whose timestamp does not parse (including a line that has no timestamp at all) yields a
  `*ParseError` with `Source`, the 1-based line number `Line` and the reason `Err`. Line numbers
  count every line, empty ones too. Parsing goes on with the next line. When the error is not nil,
  ignore the Entry.
- Lines longer than 64 KiB must work (JSON dumps, one-line stack traces); the tests use 300 KiB.
  `bufio.Scanner` stops at 64 KiB by default (04-grep). Which larger limit you pick is up to you;
  1 MiB is reasonable.
- If reading r fails, the sequence first yields the lines before the failure, then one error that
  wraps the read error (`errors.Is` works), and ends.
- It reads r only as far as it needs for the next pair: buffering is fine, reading everything first
  is not. It does not close r.

### Reorder

Reorder keeps a buffer of the entries it pulled from `src` and has not yielded yet. `newest` is
the latest Time of all entries pulled so far.

1. Pull one entry from src and add it to the buffer.
2. Yield, oldest first, every buffered entry whose Time is at least `window` before `newest`
   (`newest - Time >= window`). Then go back to 1.
3. When src ends, yield the rest of the buffer, oldest first.

- Entries with equal Time come out in the order they arrived.
- `window <= 0`: every entry is yielded right after it was pulled, so the output is in arrival
  order. Late entries (next rule) still count.
- An entry is **late** if its Time is before the latest Time that Reorder has already yielded (the
  latest, not only the previous entry's). Late entries are not dropped: step 2 yields them at once,
  because they are more than the window behind. If `stats` is not nil, Reorder adds 1 to
  `stats.Late` before it yields a late entry, so your loop body sees the new count. `stats` may be
  nil.
- The guarantee: if no entry arrives more than `window` behind the newest entry pulled before it,
  the output is in time order and `stats.Late` stays 0.
- On an early stop, Reorder stops src and drops what is still in the buffer.

### Merge

- Each source must be in time order (non-decreasing Time). Merge does not check this and does not
  repair it; that is Reorder's job.
- The result yields every entry of every source exactly once, in time order. Times are compared
  as instants: `12:00:05Z` and `14:00:05+02:00` are equal. Use `Before`, `Equal` or `Compare`,
  not `==` (it also compares the time zone).
- Equal times: the source that comes first in the argument list goes first; within one source, its
  own order is kept. In other words, the result is exactly a **stable** sort by Time of all sources
  concatenated in argument order. A test checks this on 300 random inputs.
- Empty sources are skipped. `Merge()` with no sources yields nothing.
- At any moment, also while your loop body runs, at most 1 entry per source has been pulled from
  that source and not yet yielded. (Merge has to see the next entry of every source to know which
  one is the oldest, so 1 is also the minimum.)
- Merge pulls the next entry of a source only after your loop body for that source's previous
  entry has returned. Pulling it before the yield would hold back a line Merge already has: with
  `-f`, until that pod logs again, which can take minutes.
- Each range over the result ranges over the sources again: with sources that can be ranged twice
  (like `slices.Values`), ranging twice over one Merge result yields the same entries twice.
- On an early stop, every source that Merge started has been stopped, and its deferred calls have
  run exactly once, before your loop ends.

## Examples

ParseKubectl, with `source` = `"api-7d9f"`:

```text
2026-10-16T12:00:00.5Z GET /healthz 200                  → {12:00:00.5  api-7d9f "GET /healthz 200"}, nil
2026-10-16T12:00:01Z     at Checkout.pay(Checkout.java:42) → {12:00:01  api-7d9f "    at Checkout.pay(Checkout.java:42)"}, nil
                                                          → (empty line: nothing)
--- log rotated ---                                      → *ParseError{Source: "api-7d9f", Line: 4, Err: <from time.Parse>}
```

Reorder with a window of 2 s. The numbers are seconds after 12:00:00, in arrival order:

| pull | arrives | newest | yields after this pull | buffer after |
|---|---|---|---|---|
| 1 | 10 | 10 | | 10 |
| 2 | 12 | 12 | 10 | 12 |
| 3 | 11 | 12 | | 11 12 |
| 4 | 15 | 15 | 11, 12 | 15 |
| 5 | 13 | 15 | 13 (exactly 2 s behind: in time) | 15 |
| 6 | 9 | 15 | 9 (late: 13 was already yielded; Late becomes 1) | 15 |
| 7 | 16 | 16 | | 15 16 |
| end | | | 15, 16 | |

Output: `10 11 12 13 9 15 16`, `stats.Late` = 1.

Merge of 3 sources:

```text
source 0: 12:00:01 a   12:00:03 b   12:00:05 c
source 1: 12:00:02 d   12:00:03 e
source 2: (empty)

Merge → a d b e c      (b and e have equal times: source 0 goes first)
```

`Example_pipeline` in `example_test.go` puts ParseKubectl and Merge together: two saved
`kubectl logs --timestamps` outputs become one timeline. (kubectl output is already in order per
pod, so it needs no Reorder; Reorder is for shipped logs.)

## Run the tests

    go test ./exercises/13-logmerge/
    go test -run TestParseKubectl -v ./exercises/13-logmerge/   # step 1: ParseKubectl
    go test -run TestReorder -v ./exercises/13-logmerge/        # step 2: Reorder
    go test -run TestMerge -v ./exercises/13-logmerge/          # step 3: Merge
    go test -run Example -v ./exercises/13-logmerge/            # step 4: ParseKubectl + Merge
    go test -run TestReorderRules/late -v ./exercises/13-logmerge/  # only the subtests about late entries

The 4 step filters do not overlap. The tests run your code in a goroutine with a 5 s limit and catch
panics, so a stub you have not written yet fails with `panicked: TODO: implement`, and an endless
loop fails with a message instead of hanging. `Example_pipeline` is the exception: it needs
ParseKubectl and Merge, and a panic there crashes the test binary, so run the steps one at a time
until the end. Nothing here runs in parallel, so `-race` is not needed (it does no harm either).

## Docs you can use

- [`iter`](https://pkg.go.dev/iter): `Seq`, `Seq2`, `Pull`. Read the whole package
  documentation; the sections "Single-Use Iterators" and "Pulling Values" matter here.
- [Range Over Function Types](https://go.dev/blog/range-functions) (Go blog)
- The spec: [For statements with range clause](https://go.dev/ref/spec#For_range)
- [`container/heap`](https://pkg.go.dev/container/heap) and its `IntHeap` and `PriorityQueue`
  examples
- [`bufio.Scanner`](https://pkg.go.dev/bufio#Scanner) and
  [`Scanner.Buffer`](https://pkg.go.dev/bufio#Scanner.Buffer)
- [`time.Time.Compare`](https://pkg.go.dev/time#Time.Compare), `Before`, `Equal`,
  [`time.Parse`](https://pkg.go.dev/time#Parse) and
  [`time.RFC3339Nano`](https://pkg.go.dev/time#pkg-constants)
- [`strings.Cut`](https://pkg.go.dev/strings#Cut)
- [`slices.Values`](https://pkg.go.dev/slices#Values), handy to build a source in your own tests
- `kubectl logs --help`: the flags `--timestamps`, `--prefix`, `-f` and `-l`

## Stretch goals

- A command: `logmerge api-0.log api-1.log ...` reads saved `kubectl logs --timestamps` files,
  merges them and prints each line with its pod name, like `stern`. Add `--since` and `--until`:
  thanks to laziness, `--until` can stop reading early.
- A quiet pod blocks Merge on endless input: with `-f`, Merge cannot yield the next line until
  it knows the quiet pod's next timestamp, which may come in an hour. How would you fix that? Look
  up "watermarks" in stream processing.
- Tune the window: add `MaxBuffered` (the most entries held at once) and `MaxLateness` (how far
  behind the worst late entry was) to `ReorderStats`. A clock that jumps forward on one host makes
  everything after it late; how would you notice?
- A cap on memory: an option that yields the oldest entry when the buffer holds more than N
  entries, even if the window says wait.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 13 hint 1
    go run ./cmd/peek 13 hint 2
    go run ./cmd/peek 13 hint 3
    go run ./cmd/peek 13 explanation
    go run ./cmd/peek 13 walkthrough
    go run ./cmd/peek 13 solution
