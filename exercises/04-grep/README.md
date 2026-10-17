# 04 · Streaming log filter with context

**Tier 1 · about 2 h** · Go: `io.Reader` and `io.Writer` contracts, `bufio.Scanner` and `Scanner.Buffer`, wrapping a reader, `testing/iotest`, a ring buffer, `%w` · Problem solving: keep memory constant while you stream; a small state machine for context and separators

A node went `NotReady` at 03:12. You have a 6 GB journal export from it, or a `kubectl logs -f`
stream that never ends, and you want every error with 2 lines around it, like
`grep -C 2 level=error`, but matched on a parsed field (`status>=500`) instead of a regex. The
first version breaks in three ways. `os.ReadFile` plus `strings.Split` needs 6 GB of RAM in a
debug container with a 256 MiB limit. `bufio.Scanner` stops at the first line over 64 KiB (a JSON
body logged on one line), and the loop ends as if the file were done unless you check
`Scanner.Err`. And when the stream breaks halfway, you want the output so far and an error that
says what happened. Here you write the filter as a library function: it reads once, holds only a
few lines, and reports what it did.

## Task

Implement the stubs in `grep.go`:

```go
type Options struct {
	Match        func(line string) bool // required; gets the line without its "\n"
	Before       int                    // context lines before each match, like grep -B
	After        int                    // context lines after each match, like grep -A
	MaxLineBytes int                    // longest accepted line; 0 means DefaultMaxLineBytes (1 MiB)
}

type Stats struct {
	Lines     int   // lines read and passed to Match
	Matches   int   // lines for which Match returned true
	BytesRead int64 // bytes read from the input
}

const DefaultMaxLineBytes = 1 << 20               // given
var ErrLineTooLong = errors.New("line too long") // given

// Filter writes the matching lines of r, with their context, to w.
func Filter(r io.Reader, w io.Writer, opt Options) (Stats, error)

type CountingReader struct{ /* you design the fields */ }
func NewCountingReader(r io.Reader) *CountingReader
func (c *CountingReader) Read(p []byte) (int, error) // the wrapped Read's n and err, unchanged
func (c *CountingReader) N() int64                   // bytes read so far
```

Suggested order: follow the 6 steps in "Run the tests".

## Rules

### CountingReader

- `Read` calls the wrapped reader's `Read` once and returns its `n` and `err` unchanged. `io.EOF`
  stays `io.EOF`: callers compare it with `==`.
- `N` is the sum of every `n` so far, including bytes that came in the same call as an error.
  It need not be safe for concurrent use.

### Lines and stats

- The input is split on `\n`. A last line without `\n` is still a line. `""` has 0 lines, `"\n"`
  has 1 empty line, `"a\n\nb"` has 3 lines.
- `Match` is called exactly once for every line, in input order, with the line without its `\n`.
  Lines that end up as context are passed to `Match` too.
- Every line Filter writes ends with `\n`, also a last line that had none. Otherwise lines are
  written exactly as they were read.
- `Stats.Lines` counts the lines passed to `Match`, `Stats.Matches` the lines for which it
  returned true, `Stats.BytesRead` the bytes read from `r` (after a successful run: the input
  size).
- The result does not depend on how `r` hands out the bytes: one per `Read`
  (`iotest.OneByteReader`), half of what was asked for (`iotest.HalfReader`), or the last bytes
  in the same call as `io.EOF` (`iotest.DataErrReader`).
- A `\r` before the `\n` is not tested: keep it or drop it.

### Context and separators

- With `Before` and `After` both 0, only the matching lines are written, and never a `--` line.
- Each match is written with up to `Before` lines before it and up to `After` lines after it.
  Near the start and the end of the input there are fewer.
- Every line is written at most once, in input order. Overlapping windows merge.
- A line that matches is a match, also inside the After window of an earlier match: it starts a
  new After window.
- A *group* is a run of written lines that are adjacent in the input. When `Before` or `After` is
  above 0, a line `--` goes between two groups. There is no `--` before the first group or after
  the last one. Windows that touch (no line skipped between them) form one group.
- `reference` in `context_test.go` is these rules in code: a slow version that keeps all lines in
  memory. `TestContextRandom` and `FuzzContext` compare `Filter` with it.

This is what GNU grep does, with one exception: `grep -A 0` prints `--` lines, Filter does not.

### Streaming and memory

- Filter reads `r` once, front to back. It keeps at most `Before` lines plus the current line,
  and its buffers. `TestStreamConstantMemory` filters 64 MiB and fails if the heap grows by more
  than 32 MiB.
- Output is written as soon as it is known: when a line is read, its output (a `--`, the waiting
  Before lines, the line itself) goes to `w` before Filter reads further. Do not wrap `w` in a
  `bufio.Writer`; a caller who wants buffered output does that. `TestStreamOutputBeforeInputEnds`
  feeds the input through an `io.Pipe`, like `kubectl logs -f | yourtool`.
- Filter needs no goroutines. If you start one, it must be gone when Filter returns, also after
  an early return on an error. `TestErrWrite` checks this.

### Long lines

- Lines over 64 KiB (the default limit of `bufio.Scanner`) work. The tests use 100 KiB.
- `MaxLineBytes` is the longest allowed line in bytes, not counting the `\n`. 0 means
  `DefaultMaxLineBytes` (1 MiB). A line of exactly `MaxLineBytes` is fine. One byte more is too
  long, with or without a `\n` after it.
- A too-long line stops Filter. The error wraps `ErrLineTooLong` and contains `line N`, where N is
  the 1-based number of the long line, for example `grep: line 3: line too long (limit 10 bytes)`.
  The lines before it are processed and written as usual, so `Stats.Lines` is N-1.
- Filter notices a too-long line while it reads it, not after: with a 64 MiB line and
  `MaxLineBytes` 1000, it must stop within the first 1 MiB.

### Errors

- Invalid options: `Match` is nil, or `Before`, `After` or `MaxLineBytes` is negative. Filter
  returns an error without calling `Read`, writes nothing and returns zero `Stats`.
- `io.EOF` is the normal end of the input, not an error.
- A read error stops Filter. It returns an error that wraps it (`errors.Is` works) and does not
  retry. Lines read before the error are processed and written as usual, `Stats` counts them, and
  `BytesRead` includes bytes that came in the same call as the error. In the tests the error
  comes right after a `\n`; a partial line before a read error is not tested.
- A write error stops Filter at once, whether the failed write was a matching line, a context
  line or a `--`. Filter returns an error that wraps it, does not call `Write` again and does not
  pass more lines to `Match`. `Stats` counts the lines passed to `Match` up to then, including
  the one being handled when the write failed.
- The wording of the messages is yours, except for `line N` above.

## Examples

`Match` is `strings.Contains(line, "ERROR")` in all of them.

```text
Before 1, After 1 (grep -C 1)
  input   "a\nb\nERROR 1\nc\nd\ne\nERROR 2\nf"
  output  b
          ERROR 1
          c
          --          d is skipped
          e
          ERROR 2
          f           written with "\n", although the input has none
  Stats{Lines: 8, Matches: 2, BytesRead: 27}, nil

After 2 (grep -A 2)
  input   "ERROR 1\na\nERROR 2\nb\nc\nd\n"
  output  "ERROR 1\na\nERROR 2\nb\nc\n"      ERROR 2 starts a new After window

MaxLineBytes 10
  input   "ok\nERROR\n0123456789X\nERROR again\n"
  output  "ERROR\n"
  Stats{Lines: 2, Matches: 1, ...}, error "grep: line 3: line too long (limit 10 bytes)"
```

`example_test.go` has a runnable `ExampleFilter`.

## Run the tests

    go test ./exercises/04-grep/
    go test -run TestCountingReader -v ./exercises/04-grep/   # step 1: CountingReader
    go test -run TestLines -v ./exercises/04-grep/            # step 2: lines and stats, no context
    go test -run Context -v ./exercises/04-grep/              # step 3: context and separators
    go test -run TestLongLine -v ./exercises/04-grep/         # step 4: long lines and the limit
    go test -run TestErr -v ./exercises/04-grep/              # step 5: bad options, read and write errors
    go test -run TestStream -v ./exercises/04-grep/           # step 6: streaming and memory
    go test -fuzz=FuzzContext ./exercises/04-grep/            # optional: random inputs, Ctrl-C to stop

A function that panics can stop the whole test binary, so while you work, run only the step you
are on. The 6 filters above do not overlap.

## Docs you can use

- [`io.Reader`](https://pkg.go.dev/io#Reader) and [`io.Writer`](https://pkg.go.dev/io#Writer):
  read the contracts word by word
- [`bufio.Scanner`](https://pkg.go.dev/bufio#Scanner), [`Scanner.Buffer`](https://pkg.go.dev/bufio#Scanner.Buffer),
  [`bufio.ErrTooLong`](https://pkg.go.dev/bufio#pkg-variables), and the alternative
  [`bufio.Reader.ReadSlice`](https://pkg.go.dev/bufio#Reader.ReadSlice)
- [`testing/iotest`](https://pkg.go.dev/testing/iotest)
- [`errors`](https://pkg.go.dev/errors) and [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`
- [Go Slices: usage and internals](https://go.dev/blog/slices-intro) (Go blog)
- [GNU grep manual](https://www.gnu.org/software/grep/manual/grep.html), section "Context Line Control"

## Stretch goals

- Line numbers like `grep -n`: `12:` before a matching line, `11-` before a context line.
- `MaxCount` like `grep -m`: stop after N matches, but still write the After lines of the last one.
- Make `CountingReader.N` safe to call from another goroutine (`sync/atomic`) and print the
  progress (bytes read out of the file size) every second while Filter runs on a big file.
- A command around it: files or stdin, a `regexp` match, and `os.Stdout` in a `bufio.Writer` that
  you flush at the end. Time it on a 1 GB file with and without the buffer.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 04 hint 1
    go run ./cmd/peek 04 hint 2
    go run ./cmd/peek 04 hint 3
    go run ./cmd/peek 04 explanation
    go run ./cmd/peek 04 walkthrough
    go run ./cmd/peek 04 solution
