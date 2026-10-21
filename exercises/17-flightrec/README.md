# 17 · Keep the last 500 events

**Tier 4 · about 1 h** · Go: generics (type parameters on types and functions), `any` and `comparable`, the zero value of a type parameter, `copy`, a constructor that panics, maps with pointer values · Problem solving: do the index arithmetic on paper first; decide what the caller may change

It is Black Friday week at Peckish, the company that sells bird feeders with a camera. Feeders are
30% off, the upload service takes 20 times its usual traffic, and the feeder firmware crashes about
once an hour. After each crash Ines Okafor, the senior SRE, says what she always says: "Show me the
log line." There is none. The upload service keeps 3 log files of 100 MB, and at this traffic that
is 9 minutes of history for all 40 000 feeders together. A crashed feeder reports in only when it
is back, 40 minutes later. By then its last lines have been gone for half an hour. Ines wants the
upload service to keep the last 500 events of every feeder in memory and to dump them when a feeder
reports a crash. The firmware team wants to keep log lines, the photo team wants to keep structs,
and by Wednesday each team has started its own copy. In this exercise you write 1 type that keeps
the last N values of any type, and a recorder that keeps 1 of them per feeder.

## Task

Implement the stubs in `flightrec.go`. You design the fields of both struct types.

```go
type Ring[T any] struct{ /* you design the fields */ }

func NewRing[T any](size int) *Ring[T] // panics if size < 1
func (r *Ring[T]) Add(v T)             // v is the newest value; when full, it overwrites the oldest
func (r *Ring[T]) Len() int            // values held, at most size
func (r *Ring[T]) Dropped() int        // values overwritten so far
func (r *Ring[T]) Oldest() (T, bool)   // the zero value and false when empty
func (r *Ring[T]) Newest() (T, bool)   // the zero value and false when empty
func (r *Ring[T]) Last(n int) []T      // up to n newest values, oldest first, in a new slice

type Recorder[K comparable, T any] struct{ /* you design the fields */ }

func NewRecorder[K comparable, T any](size int) *Recorder[K, T] // panics if size < 1
func (rec *Recorder[K, T]) Add(key K, v T)    // adds v to the ring of key
func (rec *Recorder[K, T]) Dump(key K) []T    // all values held for key, oldest first; nil if none
func (rec *Recorder[K, T]) Forget(key K) bool // removes key; true if it was there
func (rec *Recorder[K, T]) Len() int          // number of keys
```

Suggested order: `Ring` first (most of the work), then `Recorder`, which keeps 1 `Ring` per key.

## Rules

### Ring: size and Add

- `NewRing(size)` returns an empty ring that holds at most `size` values. It panics if `size < 1`
  (0 or negative): a ring that can hold nothing is a configuration bug. The panic message is
  yours; the tests only check that it panics.
- `Add(v)` stores `v` as the newest value. While the ring holds fewer than `size` values, nothing
  is lost. When it holds `size` values, `Add` overwrites the oldest value and `Dropped()` grows
  by 1.
- `Add` takes O(1) time (amortized O(1) is fine): its cost does not grow with `size`.
  `TestRingLargeStaysFast` makes 1 500 000 `Add` calls on a ring of size 500 000 and gives up
  after 20 s. An O(1) `Add` needs well under 1 s, also with `-race`; an `Add` that moves every
  stored value to make room needs minutes.
- `Len()` is the number of values held: the number of `Add` calls, but at most `size`.
- `Dropped()` is the number of values overwritten so far. `Len() + Dropped()` is always the
  number of `Add` calls.

### Ring: reading

- `Oldest()` returns the oldest value held and `true`. `Newest()` returns the value added last
  and `true`.
- On an empty ring both return the zero value of `T` and `false`: `0` for an `int`, `""` for a
  `string`, `nil` for a pointer, a struct with all fields zero for a struct.
- A stored zero value is a value: after `Add(0)`, `Oldest()` returns `0, true`. The bool tells
  "empty" and "holds a zero" apart.
- `Last(n)` returns the `n` newest values, oldest first. If the ring holds fewer than `n` values,
  it returns all of them, oldest first.
- `Last(n)` with `n <= 0`, and `Last` on an empty ring, return no values. Both nil and an empty
  slice are fine.

### Copies

- `Last` returns a new slice that shares no memory with the ring. Changing an element of the
  result, appending to the result, or adding to the ring later never changes the other side.
- The copy is shallow. If `T` is a pointer type, the result holds the same pointers as the ring.

### Recorder

- `NewRecorder(size)` returns an empty recorder. It panics if `size < 1`.
- `Add(key, v)` adds `v` to the ring of `key`. The first `Add` for a key creates its ring.
- Each key has its own ring of `size`: the values of 1 key never push out the values of another
  key.
- `Dump(key)` returns all values held for `key`, oldest first, in a new slice. The copy rules of
  `Last` apply.
- `Dump(key)` returns **nil** for a key without values: a key that was never added, or a key that
  `Forget` removed. It never returns nil for a key that holds values.
- `Forget(key)` removes `key` and its values and returns `true`. For a key that is not there it
  returns `false` and changes nothing. A later `Add` for that key starts with an empty ring.
- `Len()` is the number of keys that hold values.
- A key can be of any comparable type: a `string`, an `int`, or a struct of comparable fields such
  as `struct{ Feeder string; Boot int }`. 2 struct keys are the same key when all their fields are
  equal.

### Concurrency

- Neither type is safe for concurrent use. A caller that shares one between goroutines must lock
  around it. The tests call every method from 1 goroutine.

## Examples

A ring of size 3:

```text
r := NewRing[string](3)
r.Oldest() = "", false         r.Last(2) = []
r.Add("boot"); r.Add("wifi")
r.Len() = 2   r.Dropped() = 0   r.Last(5) = [boot wifi]
r.Add("upload"); r.Add("retry"); r.Add("crash")
r.Len() = 3   r.Dropped() = 2
r.Oldest() = "upload", true    r.Newest() = "crash", true
r.Last(2) = [retry crash]      r.Last(3) = [upload retry crash]      r.Last(0) = []
```

A recorder with 2 values per key (this is `ExampleRecorder`):

```go
type entry struct {
	Seq int
	Msg string
}
rec := flightrec.NewRecorder[string, entry](2)
rec.Add("feeder-0412", entry{1, "boot"})
rec.Add("feeder-0007", entry{1, "boot"})
rec.Add("feeder-0412", entry{2, "upload"})
rec.Add("feeder-0412", entry{3, "crash"})

rec.Len()                   // 2
rec.Dump("feeder-0412")     // [{Seq:2 Msg:upload} {Seq:3 Msg:crash}]
rec.Dump("feeder-0007")     // [{Seq:1 Msg:boot}]
rec.Dump("hq-window-01")    // nil
```

## Run the tests

    go test ./exercises/17-flightrec/
    go test -run TestRingBasic -v ./exercises/17-flightrec/                   # step 1: Ring
    go test -run 'TestRingModel|TestRingLarge' -v ./exercises/17-flightrec/   # step 2: Ring against a model, and a fast Add
    go test -run Recorder -v ./exercises/17-flightrec/                        # step 3: Recorder
    go test -run TestRingBasicLast -v ./exercises/17-flightrec/               # one test

Run one step at a time; the 3 filters do not overlap. Every test recovers a "TODO: implement"
panic and reports it as a failure, so the other tests of the step still run. `TestRingModel`
compares your ring with a simple slice after each of 10 000 random calls and prints up to 10 of
the last calls when they disagree. `ExampleRecorder` runs in step 3 and in a plain `go test`. An example
cannot recover a panic, so a plain run crashes until `Recorder` works.

## Docs you can use

- [An Introduction To Generics](https://go.dev/blog/intro-generics) (Go blog)
- [Tutorial: Getting started with generics](https://go.dev/doc/tutorial/generics)
- The Go spec: [Type parameter declarations](https://go.dev/ref/spec#Type_parameter_declarations),
  [Type constraints](https://go.dev/ref/spec#Type_constraints),
  [Comparison operators](https://go.dev/ref/spec#Comparison_operators) (which types are comparable),
  [The zero value](https://go.dev/ref/spec#The_zero_value)
- The Go spec: [Arithmetic operators](https://go.dev/ref/spec#Arithmetic_operators), in particular
  the sign of the result of `%`
- The Go spec: [Appending to and copying slices](https://go.dev/ref/spec#Appending_and_copying_slices),
  and the builtins [`copy`](https://pkg.go.dev/builtin#copy) and [`min`](https://pkg.go.dev/builtin#min)
- [Go Slices: usage and internals](https://go.dev/blog/slices-intro) (Go blog)
- [Effective Go: Allocation with make](https://go.dev/doc/effective_go#allocation_make)
- [`runtime/trace.FlightRecorder`](https://pkg.go.dev/runtime/trace#FlightRecorder): the flight
  recorder that Go 1.25 added for execution traces

## Stretch goals

- Add `All() iter.Seq[T]`, so that a caller can range over the values, oldest first, without a
  copy. Decide what happens when the loop body calls `Add`.
- Make `Recorder` safe for concurrent use with a `sync.Mutex` (20-counters shows how), and serve `Dump` as
  JSON on a debug endpoint such as `/debug/flightrec?feeder=feeder-0412`.
- Forget the keys of feeders that sent nothing for an hour, with an injected clock (23-silences does
  the same for silences that have ended).
- Keep a time window instead of a count: every event of the last 10 minutes, however many that is.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 17 hint 1
    go run ./cmd/peek 17 hint 2
    go run ./cmd/peek 17 hint 3
    go run ./cmd/peek 17 explanation
    go run ./cmd/peek 17 walkthrough
    go run ./cmd/peek 17 solution
