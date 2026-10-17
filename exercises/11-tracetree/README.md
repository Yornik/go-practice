# 11 · Make sense of a distributed trace

**Tier 3 · about 2.5 h** · Go: a map of pointers as an index, recursion over a tree, `slices.SortFunc` with `cmp.Or`, `time.Duration` arithmetic, `strings.Builder` with `fmt.Fprintf` · Problem solving: build a tree from flat records; reason about overlapping time intervals; work out timelines on paper

After a deploy the checkout p99 went from 400 ms to 1.3 s. You export one slow trace from Tempo
or Jaeger: 40 spans in a flat list, each with a span ID, a parent ID, a service, a name, a start and
an end. You want answers you can paste into the incident channel: which service spent the time
itself, and which chain of calls made the request slow. The shortcuts give wrong answers. Summing
durations per service counts the same milliseconds several times, because a parent's duration
includes its children's. "The slowest span" is always the root. And the data is messy: children
arrive before their parents (a span is exported when it ends), a collector can drop a parent,
calls run in parallel, and host clocks disagree, so a child can stick out of its parent. You build
the tree, compute self time per span and per service, find the critical path, and print the trace.

## Task

Implement the stubs in `tracetree.go`, `analyze.go` and `render.go`:

```go
type Span struct {
	TraceID, SpanID, ParentID string // ParentID is "" for the root span
	Service, Name             string
	Start, End                time.Time
}
type Node struct {
	Span     Span
	Children []*Node // sorted by Start, then SpanID
}
func (n *Node) Duration() time.Duration // End - Start (given)
type Trace struct {
	Root    *Node
	Orphans []*Node // nodes whose parent is not in the trace, sorted by Start, then SpanID
}

func Build(spans []Span) (*Trace, error)             // the flat list as a tree
func SelfTime(n *Node) time.Duration                 // n's time not covered by its children
func ServiceTimes(t *Trace) map[string]time.Duration // self time summed per service
func CriticalPath(t *Trace) []*Node                  // the chain of spans that set the latency
func Render(t *Trace) string                         // the tree as text, for a terminal

var ErrNoSpans, ErrMixedTraces, ErrDuplicateSpanID, // given; Build wraps them
	ErrNegativeDuration, ErrNoRoot, ErrMultipleRoots error
```

Work in the order of the steps in "Run the tests". The tests of SelfTime, ServiceTimes and
CriticalPath build their trees by hand, so they do not depend on your Build.

## Rules

### Build

- The spans come in any order. The result must not depend on it.
- The root is the span with an empty `ParentID`.
- A span whose `ParentID` is the `SpanID` of another input span becomes a child of that span.
  Build links by ID only: a child that starts before its parent (clock skew) is still its child.
- A span whose `ParentID` is not empty but matches no span is an **orphan**: it goes into
  `Orphans` as the top of its own subtree. Two orphans with the same missing parent are two
  separate orphans.
- `Children` of every node (also below an orphan), and `Orphans`, are sorted by `Start`
  (earliest first), then by `SpanID` (string order). A leaf's `Children` and an empty `Orphans`
  may be nil or empty.
- Every span is in the result exactly once, and `Node.Span` is the input span, every field
  unchanged.
- Build does not modify the input slice: not its elements, not their order.
- A span with `End == Start` is valid.
- You may assume that span IDs are never empty and that parent links have no cycles.

On invalid input Build returns a **nil** `*Trace` and an error that wraps one of the given errors
(`errors.Is` works). With several problems, report any one. The wording is yours, but the message
must contain the span IDs in the last column.

| Problem | Wraps | Message contains |
|---|---|---|
| `spans` is nil or empty | `ErrNoSpans` | |
| a span's `TraceID` differs from `spans[0].TraceID` | `ErrMixedTraces` | that span's `SpanID` |
| two spans have the same `SpanID` | `ErrDuplicateSpanID` | the `SpanID` |
| a span's `End` is before its `Start` | `ErrNegativeDuration` | the `SpanID` |
| no span has an empty `ParentID` | `ErrNoRoot` | |
| more than one span has an empty `ParentID` | `ErrMultipleRoots` | the `SpanID` of every root |

### Everything after Build

- SelfTime, ServiceTimes, CriticalPath and Render take a valid tree: what Build returns, or a tree
  built by hand with the same properties (non-nil `Root`, `End` never before `Start`, `Children`
  sorted as above). They may rely on that.
- They never modify the tree: no field of any `Span`, no `Children` slice.

### SelfTime

- `SelfTime(n)` is `n.Duration()` minus the time during which at least one **direct** child of
  `n` was running.
- A child counts only inside `n`'s own `Start`..`End`. The part of a child before `n` starts or
  after `n` ends (clock skew, or async work that outlives its caller) is ignored, and a child
  completely outside `n` counts for nothing.
- Time covered by several children at once (parallel calls) counts once.
- Grandchildren do not count, even when they stick out of their own parent.
- So the result is between 0 and `n.Duration()`, and a leaf's self time is its duration.

### ServiceTimes

- For every node, in the root tree and in every orphan tree, add `SelfTime(node)` to the entry of
  `node.Span.Service`.
- Every service that appears in the trace is a key, also when its sum is 0 (a gateway whose child
  covers all of its time).

### CriticalPath

The critical path answers "what was the request waiting for?". Walk back in time from the root's
end: the root was waiting for the child that ended last; before that child started, for the child
that ended last before that moment; and so on. Inside each picked child, ask the same. Exactly:

```text
CriticalPath(t) = path(t.Root, t.Root.Span.End)

path(n, cursor):                     // n is on the critical path up to cursor
    result = [n]
    picks = []
    repeat:
        candidates = the children c of n with c.Start < cursor   (strictly before)
        if there are none: stop repeating
        end(c) = c.End, or cursor if c.End is after cursor
        best = the candidate with the latest end(c);
               on a tie, the one with the smallest SpanID
        add (best, end(best)) to picks
        cursor = best.Start
    for each (c, e) in picks, from the last pick back to the first:
        append path(c, e) to result
    return result
```

- The result is a node, then the path inside each of its picked children, in the order those
  children ran: the reverse of the order you picked them.
- The path inside a picked child ends at `end(best)`: a child that ends after the cursor is cut
  there, also for the recursion. Work that a child started but did not wait for (a grandchild that
  starts after the child's End) is therefore not on the path.
- The SpanID tie-break is arbitrary. It only makes the result exact, whatever order your code
  scans the children in.
- Orphans are never on the critical path: nobody knows whether the root waited for them.
- The result holds the tree's own `*Node` pointers, not copies.

### Render

```text
trace <TraceID of the root>
<one line per node of the root tree>
orphan: parent <ParentID of the orphan> not in trace
<one line per node of that orphan's tree>
```

- The last 2 parts repeat for each orphan, in `Orphans` order. Without orphans the output ends
  after the root tree.
- Node lines come depth first: a node, then its children in `Children` order, each with its whole
  subtree.
- A node line is `<mark> <indent><Service>: <Name> start=<offset> dur=<duration> self=<self time>`
  - `<mark>` is `*` if the node is on `CriticalPath(t)`, otherwise a space.
  - `<indent>` is 2 spaces per level below the top of its tree: none for the root and for each
    orphan, 2 for their children, 4 below those.
  - `<offset>` is the node's `Start` minus the **root's** `Start`, also for orphans. It can be
    negative.
  - Durations are formatted as `time.Duration` prints itself with `%v`: `0s`, `75ms`, `1.25s`,
    `-20ms`.
- Every line ends with `\n`, also the last one. No line has trailing spaces.

## Examples

One checkout, as a tree. Times are ms after the root's start; `*` marks the critical path.

```text
  span ID     service    name              start-end   self time
* checkout    frontend   POST /checkout      0-500      45
*   auth      auth       Authorize           5-25       20
*   cart      cart       GetCart            30-80       50
*   inventory inventory  Reserve            85-200      15
*     db      postgres   UPDATE stock       90-190     100
    pricing   pricing    Quote              85-150      65
*   payment   payment    Charge            210-480      15
*     fraud   fraud      Score             215-300      85
*     psp     payment    POST /v1/capture  300-470     170
```

- `Build` on these 9 spans, in any order, gives this tree. `inventory` comes before `pricing`:
  same Start, and "inventory" < "pricing".
- `SelfTime(checkout)`: the children cover 5-25, 30-80, 85-200 (inventory and pricing overlap) and
  210-480. That is 20 + 50 + 115 + 270 = 455 ms, so the self time is 500 − 455 = 45 ms.
- `ServiceTimes`: frontend 45ms, auth 20ms, cart 50ms, inventory 15ms, postgres 100ms, pricing
  65ms, payment 185ms (15 + 170), fraud 85ms. Together 565 ms, more than the 500 ms the request
  took, because pricing ran in parallel with inventory.
- `CriticalPath`: cursor 500 picks payment (ends 480). Cursor 210 picks inventory (200 beats
  pricing's 150). Cursor 85 picks cart. Cursor 30 picks auth. Cursor 5: no child starts before 5.
  Inside inventory (cursor 200) db; inside payment (cursor 480) psp, then fraud. Result:
  `checkout auth cart inventory db payment fraud psp`.
- `Render`:

```text
trace 4bf92f3577b34da6a3ce929d0e0e4736
* frontend: POST /checkout start=0s dur=500ms self=45ms
*   auth: Authorize start=5ms dur=20ms self=20ms
*   cart: GetCart start=30ms dur=50ms self=50ms
*   inventory: Reserve start=85ms dur=115ms self=15ms
*     postgres: UPDATE stock start=90ms dur=100ms self=100ms
    pricing: Quote start=85ms dur=65ms self=65ms
*   payment: Charge start=210ms dur=270ms self=15ms
*     fraud: Score start=215ms dur=85ms self=85ms
*     payment: POST /v1/capture start=300ms dur=170ms self=170ms
```

Three shapes of the critical path. One character is 10 ms:

```text
sequential             parallel               overlapping
r ==========  0-100    r ==========  0-100    r ==========  0-100
a ===         0-30     a  ====       10-50    a =========   0-90
b    ===      30-60    b  ========   10-90    b  =========  10-100
c       ====  60-100
path: r a b c          path: r b              path: r a b
```

- Parallel: after b the cursor is 10, and a does not start *before* 10, so it is not on the path.
- Overlapping: after b the cursor is 10. a started at 0, so it is a candidate (its End counts as
  10), and it is on the path for 0-10.

## Run the tests

    go test ./exercises/11-tracetree/
    go test -run TestBuild -v ./exercises/11-tracetree/                        # step 1: Build
    go test -run 'TestSelfTime|TestServiceTimes' -v ./exercises/11-tracetree/  # step 2
    go test -run TestCriticalPath -v ./exercises/11-tracetree/                 # step 3
    go test -run TestRender -v ./exercises/11-tracetree/                       # step 4
    go test -run 'TestCriticalPath$/overlaps' -v ./exercises/11-tracetree/     # one case

A stub that panics stops the whole test binary, so run only the step you are on. Build failures
print trees as `root(a b(b1 b2))`, children in order; SelfTime and CriticalPath failures print the
tree with its times in ms.

## Docs you can use

- [`slices`](https://pkg.go.dev/slices): `SortFunc`, `Backward`
- [`cmp`](https://pkg.go.dev/cmp): `Compare`, `Or`
- [`strings.Builder`](https://pkg.go.dev/strings#Builder) and [`fmt.Fprintf`](https://pkg.go.dev/fmt#Fprintf)
- [`time.Time`](https://pkg.go.dev/time#Time): `Sub`, `Before`, `After`, `Compare`, `Equal`;
  [`time.Duration.String`](https://pkg.go.dev/time#Duration.String)
- [Go maps in action](https://go.dev/blog/maps) (Go blog)
- The spec: [Address operators](https://go.dev/ref/spec#Address_operators)
- [OpenTelemetry: Traces](https://opentelemetry.io/docs/concepts/signals/traces/), the span model
  that exporters use

## Stretch goals

- Critical sections: for each node on the path, the time during which it was the deepest node on
  the path. Without clock skew they add up to the root's duration. Print the top 3.
- Read real data: decode an OTLP JSON export (`resourceSpans` → `scopeSpans` → `spans`) with
  `encoding/json` into `[]Span`, and write a command that renders a file.
- Compare two traces, a fast one and a slow one, by `ServiceTimes`, and print the services sorted
  by how much their self time grew.
- Stop assuming clean input: detect cycles in the parent links (see 10-deployorder), and correct
  clock skew by shifting a child that sticks out of its parent, as Jaeger does.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 11 hint 1
    go run ./cmd/peek 11 hint 2
    go run ./cmd/peek 11 hint 3
    go run ./cmd/peek 11 explanation
    go run ./cmd/peek 11 walkthrough
    go run ./cmd/peek 11 solution
