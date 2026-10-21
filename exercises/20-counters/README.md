# 20 · Count photos without crashing

**Tier 4 · about 1 h** · Go: `sync.Mutex` and `defer` to unlock, data races and `go test -race`, `sync.WaitGroup` in tests, pointers to shared values, a map that many goroutines share, `io.Writer` · Problem solving: list what each lock protects; keep slow work out of it

At 11:58 on Black Friday, the busiest minute of the year at Peckish, the photo upload service died
with 1 log line: `fatal error: concurrent map writes`. Peckish sells bird feeders with a camera.
The feeders were 30% off, and thousands of new feeders sent their first photos at the same time.
The service counted those photos per region in a plain `map[string]int`, and every upload runs in
its own goroutine. On a normal day, 2 uploads almost never wrote to the map at the same moment. At
20 times the traffic, they did, and the Go runtime stopped the whole process; no `recover` catches
that. So the line that measured the busiest minute is the line that ended it. The hotfix at 12:03
deleted the counter, and the graph "Photos per region" now shows the biggest day in the history of
the company as a flat line. Ines Okafor, the senior SRE, wants the counter back, correct under any
load, with 1 more wish: a slow scrape of the numbers must never slow down an upload. In this
exercise you write a small registry of counters that many goroutines can use at the same time.

## Task

Implement the stubs in `counters.go`. `Sample` is given. You design the fields of `Registry` and
`Counter`.

```go
type Registry struct{ /* you design the fields */ }
func NewRegistry() *Registry
func (r *Registry) Counter(name string) *Counter // created on first use; same name, same *Counter

type Counter struct{ /* you design the fields */ }
func (c *Counter) Add(n int64)                   // panics if n < 0: counters only go up
func (c *Counter) Value() int64

type Sample struct { // given
	Name  string
	Value int64
}
func (r *Registry) Snapshot() []Sample           // every counter, sorted by name
func (r *Registry) Drain() []Sample              // like Snapshot, and sets every counter to 0
func (r *Registry) Write(w io.Writer) error      // "<name> <value>\n" per counter, sorted by name
```

Suggested order: `NewRegistry`, `Counter`, `Add` and `Value` first, then `Snapshot`, `Drain` and
`Write`. Then run everything with `-race`.

## Rules

### Registry and Counter

- `NewRegistry` returns an empty registry. Each registry has its own counters: the same name in 2
  registries gives 2 different counters.
- `Counter(name)` returns the counter for `name`. The first call for a name creates it with the
  value 0. Every later call for that name returns the same `*Counter` (the same pointer). This
  holds when 100 goroutines ask for the same new name at the same moment, and it holds after
  `Drain`.
- `Counter` does not check names. Any string is a name, also `""` or a name with a space.
- The registry never removes a counter.
- `Add(n)` adds `n` to the counter. `Add(0)` is allowed and changes nothing.
- `Add(n)` with `n < 0` panics: a counter only goes up. The panic message is yours. The value does
  not change, and the counter keeps working: later calls of `Add` and `Value` on it return
  normally.
- `Value()` returns the current value.

### Snapshot and Drain

- `Snapshot()` returns 1 `Sample` per counter of the registry, sorted by `Name` in Go's string
  order (byte by byte, like `<` on strings). It includes counters with the value 0. For an empty
  registry it returns a slice of length 0; nil and an empty slice are both fine.
- The slice is a copy that belongs to the caller. An `Add` or a new counter after `Snapshot` does
  not change it, and a change to the slice does not change any counter.
- `Drain()` returns what `Snapshot()` would return and sets every counter to 0.
- For each counter, reading the value and setting it to 0 are 1 step. An `Add` that runs at the
  same time as `Drain` counts either in the result of `Drain` or in the counter afterwards: never
  in both, and never in neither. So the sum of every drained value plus the final values equals
  the sum of every `Add`.
- After `Drain` every counter still exists, with the value 0. `Counter(name)` returns the same
  pointer as before, and `Add` on a pointer you got before `Drain` still counts.

### Write

- `Write(w)` writes 1 line per counter: the name, 1 space, the value in decimal, and `\n`. The
  lines are sorted by name, like `Snapshot`. With no counters the text is empty.
- The text holds the values of 1 snapshot taken when `Write` starts. A counter that another
  goroutine creates, or an `Add` that it makes, while `Write` is writing does not appear in this
  text. It appears in the next `Write`.
- `Write` does not change any counter.
- `Write` may call `w.Write` once or several times. When a call of `w.Write` returns an error,
  `Write` makes no more calls to `w` and returns that error, as it is or wrapped with `%w`, so
  that `errors.Is` finds it. Otherwise it returns `nil`.
- `Write` holds no lock while it calls `w.Write`. A scraper that reads slowly, or not at all, never
  blocks `Add`, `Value`, `Counter`, `Snapshot` or `Drain` in another goroutine.

### Goroutines

- Every method of `Registry` and `Counter` is safe for concurrent use by many goroutines.
- Your code must be free of data races. Run the tests with `-race`: the race detector reports 2
  goroutines that use the same memory without synchronisation, also on a run that happens to give
  the right totals.
- The tests check exact totals, not timings: 64 goroutines that call `Add(1)` 10 000 times each,
  on 1 counter and on 64 counters, must give exactly 640 000.

### What this exercise leaves out

- No labels and no text format. A name is just a string, and `Write` writes plain lines.
  42-promtext parses the Prometheus text format, with labels, `# HELP` and `# TYPE` lines.
- No gauges and no histograms: only counters that go up.

## Examples

```go
r := counters.NewRegistry()
r.Counter("uploads_eu").Add(3)
r.Counter("uploads_us").Add(2)
r.Counter("uploads_eu").Add(1)
r.Counter("rejected") // created with the value 0
r.Write(os.Stdout)
```

```text
rejected 0
uploads_eu 4
uploads_us 2
```

This is `ExampleRegistry_Write`. `ExampleRegistry_Drain` shows `Drain`:

```text
eu := r.Counter("uploads_eu"); eu.Add(4)
r.Counter("uploads_us").Add(2)
r.Drain()      → [{uploads_eu 4} {uploads_us 2}]
r.Snapshot()   → [{uploads_eu 0} {uploads_us 0}]
eu.Add(5)
r.Snapshot()   → [{uploads_eu 5} {uploads_us 0}]
```

`Write` while another goroutine counts, with `uploads_eu` at 3 and `uploads_us` at 2:

```text
goroutine 1                          goroutine 2
r.Write(w) starts
  w.Write is slow ...                r.Counter("uploads_new").Add(1)   returns at once
                                     r.Counter("uploads_us").Add(1)    returns at once
r.Write(w) returns nil; w got "uploads_eu 3\nuploads_us 2\n"
r.Write(w) again; w gets "uploads_eu 3\nuploads_new 1\nuploads_us 3\n"
```

## Run the tests

    go test -race ./exercises/20-counters/
    go test -run TestCounter -v ./exercises/20-counters/                  # step 1: NewRegistry, Counter, Add, Value
    go test -run TestRegistry -v ./exercises/20-counters/                 # step 2: 1 counter per name, Snapshot, Drain, Write
    go test -race -run TestConcurrent -v ./exercises/20-counters/         # step 3: many goroutines at once
    go test -race -run TestWriteDoesNotBlock -v ./exercises/20-counters/  # step 4: a slow scraper

Run 1 step at a time; the 4 filters do not overlap. In steps 1, 2 and 4 every call into your
code runs in a goroutine of its own with a recover and a timeout of 5 s or more. So a
"TODO: implement" panic, or a lock that is never released, fails 1 test with a message instead of
crashing or hanging the run.

In step 3 many goroutines call your code directly and at the same moment. The test sets
`GOMAXPROCS` to at least 4, so that they really run in parallel, releases them all with 1 closed
channel, and waits for them with a `sync.WaitGroup`. Each goroutine recovers a panic. A plain map
without a lock can still crash the whole test binary with `fatal error: concurrent map writes`,
exactly as in the story: no recover catches that.

Run step 3 with `-race`. Without it, a missing lock can pass by luck. But not every concurrency bug
is a data race, so `-race` alone does not prove your code right. Step 3 catches the other bugs by
their results, and only when the goroutines really overlap. On a machine with 1 CPU a pass proves
less, so add `-count=10` there to run each test 10 times.

In step 4 the writer that `Write` gets stops in its first `w.Write` call. While it waits, another
goroutine calls `Counter("uploads_new").Add(1)` and `Counter("uploads_us").Add(1)`. If those calls
need a lock that `Write` holds, they wait too, and after 5 s the test fails with
"Add and Counter blocked while Write was writing".

`ExampleRegistry_Write` and `ExampleRegistry_Drain` run in every `go test` without `-run`, such as
the first command above, with or without `-race`. An example cannot recover a panic, so such a run
crashes until steps 1 and 2 work.

## Docs you can use

- A Tour of Go: [sync.Mutex](https://go.dev/tour/concurrency/9)
- [`sync.Mutex`](https://pkg.go.dev/sync#Mutex) and [`sync.WaitGroup`](https://pkg.go.dev/sync#WaitGroup)
- [Effective Go: Defer](https://go.dev/doc/effective_go#defer)
- [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [Go maps in action: Concurrency](https://go.dev/blog/maps#concurrency) (Go blog), and the Go FAQ:
  [Why are map operations not defined to be atomic?](https://go.dev/doc/faq#atomic_maps)
- [The Go Memory Model](https://go.dev/ref/mem): the introduction and its "Advice" section are
  enough for now
- [`slices.SortFunc`](https://pkg.go.dev/slices#SortFunc) and [`strings.Compare`](https://pkg.go.dev/strings#Compare)
- [`io.Writer`](https://pkg.go.dev/io#Writer) and [`fmt.Fprintf`](https://pkg.go.dev/fmt#Fprintf)
- [`go vet`](https://pkg.go.dev/cmd/vet): its `copylocks` check reports a copied mutex
- Prometheus: [Metric types: Counter](https://prometheus.io/docs/concepts/metric_types/#counter)

## Stretch goals

- Replace the mutex in `Counter` with [`atomic.Int64`](https://pkg.go.dev/sync/atomic#Int64)
  (`Add`, `Load` and `Swap`). Compare both with a benchmark that uses `b.RunParallel`.
- Most calls of `Counter` ask for a name that already exists. Try a `sync.RWMutex`: look up under
  `RLock`, and create under `Lock`. Why must you look up a second time after `Lock`?
- Serve `Write` over HTTP: a handler for `GET /metrics` that any `curl` can scrape.
- Add a `Gauge` with `Set` and `Add` that can go down, and think about what `Drain` should do
  with it.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 20 hint 1
    go run ./cmd/peek 20 hint 2
    go run ./cmd/peek 20 hint 3
    go run ./cmd/peek 20 explanation
    go run ./cmd/peek 20 walkthrough
    go run ./cmd/peek 20 solution
