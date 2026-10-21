# 35 · Deploy order, waves and blast radius

**Tier 8 · about 2.5 h** · Go: maps as adjacency lists and sets, `slices.Sorted(maps.Keys(m))`, recursive closures, custom error types with `errors.As`, copying slices you hand out · Problem solving: model the task as a graph; pick BFS or DFS by the question you ask; make the output deterministic

30 services must run in the `cleanroom` cluster, free of HUSH, and order matters: cert-manager
before the ingress controller, postgres before the services that run schema migrations at startup,
the API gateway before the web frontend. Argo CD sync waves and Helm hook weights encode this order
as numbers that someone typed by hand, and six months later nobody knows why `orders` is wave 3, or
whether it could be wave 1. Each service already declares what it needs, so you compute the plan
instead: deploy everything that is ready in parallel, wave after wave. Hand-maintained dependency
lists also grow cycles ("auth needs config, config needs auth"), and then the tool must name the
exact loop, the same loop on every run. Adaeze Nwosu, the CEO, asks "Who is not in this room?"
Marisol Quispe Mamani, the support lead: no list names her tools. If HUSH goes dark, what falls (the
blast radius)? Why does the web frontend need Kafka at all? In this exercise you build the tool.

## Task

Implement the stubs in `deployorder.go`:

```go
func New(deps map[string][]string) (*Graph, error)            // deps: service → the services it depends on

func (g *Graph) Waves() ([][]string, error)                   // services grouped into deployment waves
func (g *Graph) Order() ([]string, error)                     // the waves concatenated
func (g *Graph) Dependents(service string) ([]string, error)  // everything that depends on service, directly or not
func (g *Graph) Why(from, to string) ([]string, bool)         // the shortest chain from → ... → to

type UnknownDependencyError struct{ Service, Dependency string }
func (e *UnknownDependencyError) Error() string               // service "api" depends on unknown service "postgress"

type CycleError struct{ Path []string }                       // [a b c a]: a depends on b, b on c, c on a
func (e *CycleError) Error() string                           // dependency cycle: a -> b -> c -> a

var ErrUnknownService = errors.New("unknown service")         // given
```

You design the fields of `Graph`. "a depends on b" means b must be running before a starts.
Suggested order: New and the two `Error` methods, then Waves and Order, then the cycle report, then
Dependents, then Why.

## Rules

"Sorted" always means Go string order, as `slices.Sort` sorts.

### New

- Every key of deps is a service; its slice lists the services it depends on. A nil or empty slice
  means no dependencies. A nil or empty map is a valid, empty graph.
- Every dependency must also be a key. If one is not, New returns a nil Graph and an
  `*UnknownDependencyError` (`errors.As` works). With several unknown dependencies it reports the
  first one: services in sorted order, then the dependencies of each service in sorted order.
- Duplicate dependencies count once: `api: [db, db]` is `api: [db]`.
- New accepts cycles, including a service that depends on itself. Waves and Order report them;
  Dependents and Why work on graphs with cycles. An unknown dependency is reported by New even when
  the graph also has a cycle.
- New does not change deps, not even the order inside its slices. The Graph shares no memory with
  deps: changing the map or its slices after New does not change the Graph.
- `UnknownDependencyError.Error()` returns `service "<Service>" depends on unknown service "<Dependency>"`.

### Waves and Order

- Wave 0 holds the services without dependencies. Every other service goes into the earliest wave
  after all its dependencies: its wave is 1 + the highest wave among its dependencies. A service
  that depends on something in wave 0 and on something in wave 2 goes into wave 3.
- Every service is in exactly one wave. No wave is empty. Each wave is sorted.
- An empty graph has no waves: Waves returns a slice of length 0 (nil is fine) and a nil error.
- Order returns the waves concatenated: wave 0 first, each wave in its sorted order. For an empty
  graph it returns a slice of length 0 and a nil error.
- If the graph has a cycle, Waves and Order return nil and a `*CycleError`, never a partial plan.

### Cycles

`CycleError.Path` lists the services of one cycle, each depending on the next. It starts and ends
with the same service, and no other service appears twice. A self-dependency gives `[a a]`.
`Error()` returns `dependency cycle: ` followed by Path joined with ` -> `.

A graph can have many cycles. The tool must report the same one on every run, so the rule is fixed:
it is the first cycle found by this depth-first search.

1. Start a search from each service in sorted order. Skip services that an earlier search finished.
2. To visit a service: add it to the end of the current path, then go through its dependencies in
   sorted order, then remove it from the path. It is now finished.
3. A dependency that is finished is skipped. A dependency that was not visited yet is visited.
4. A dependency that is on the current path closes a cycle. Stop: Path is the current path from
   that dependency to the end, followed by that dependency again.

For `{a: [b], b: [c, d], c: [], d: [b]}`:

```text
visit a          path [a]
  visit b        path [a b]
    visit c      path [a b c]   no dependencies: c is finished, path [a b]
    visit d      path [a b d]   d depends on b, and b is on the path
Path = [b d b]                  (a is on the path but not in the cycle)
```

- Path is not rotated: it starts where the search entered the cycle, not at the smallest name.
- The cycle found first is not necessarily the shortest or the longest one.
- Order returns the same cycle as Waves.

### Dependents

- `Dependents(s)` returns every service that depends on s, directly or through other services:
  what breaks if s goes down. Sorted, no duplicates.
- s itself is never in the result, also when s is part of a cycle.
- No dependents: a slice of length 0 (nil is fine) and a nil error.
- Unknown s: nil and an error that wraps `ErrUnknownService` (`errors.Is` works) and contains s.
  A typo must not look like "nothing breaks".

### Why

- `Why(from, to)` returns a chain `[from, ..., to]` in which each service depends on the next one,
  and true. The chain has the fewest services possible.
- If several chains are equally short, Why returns the lexicographically first one: compare the
  chains service by service from the start; at the first difference, the smaller name (Go string
  order) wins.
- `Why(s, s)` for a known s returns `[s]` and true.
- If from or to is unknown, or from does not depend on to (directly or not), Why returns nil and
  false. Direction matters: web depends on postgres, postgres does not depend on web.

### Results, size and concurrency

- Large graphs work: a chain of 1000 services, and 30 stacked diamonds (91 services, but 2^30
  different paths from top to bottom). A search must visit each service once; one that walks every
  path never finishes.
- Every slice that a method returns, including `CycleError.Path`, is new. The caller may change it,
  and later results stay the same.
- A Graph does not change after New. All methods are safe to call from several goroutines at once.
  `TestConcurrentUse` checks this; it reliably catches a data race only when you run it with
  `-race`.

## Examples

The graph that most tests use:

```go
g, err := deployorder.New(map[string][]string{
	"cert-manager":  nil,
	"ingress-nginx": {"cert-manager"},
	"postgres":      nil,
	"redis":         nil,
	"kafka":         nil,
	"auth":          {"postgres", "redis"},
	"orders":        {"postgres", "kafka", "auth"},
	"payments":      {"postgres", "auth"},
	"api-gateway":   {"auth", "orders", "payments", "ingress-nginx"},
	"web":           {"api-gateway"},
})
```

```text
g.Waves()  → [[cert-manager kafka postgres redis] [auth ingress-nginx] [orders payments] [api-gateway] [web]], nil
g.Order()  → [cert-manager kafka postgres redis auth ingress-nginx orders payments api-gateway web], nil

g.Dependents("postgres")  → [api-gateway auth orders payments web], nil
g.Dependents("web")       → [], nil
g.Dependents("postgress") → nil, `unknown service: "postgress"` (wraps ErrUnknownService)

g.Why("web", "postgres")  → [web api-gateway auth postgres], true
                            (through orders or payments is as short; auth comes first)
g.Why("web", "kafka")     → [web api-gateway orders kafka], true
g.Why("postgres", "web")  → nil, false
g.Why("auth", "auth")     → [auth], true
```

Another graph:

```text
New({console: [notify, postgres], hush-api: [], notify: [hush-api], postgres: [],
     support-desk: [console]})  → g, nil
g.Dependents("hush-api")            → [console notify support-desk], nil
g.Why("support-desk", "hush-api")   → [support-desk console notify hush-api], true
```

Errors:

```text
New({api: [postgress], postgres: []})
  → nil, &UnknownDependencyError{Service: "api", Dependency: "postgress"}
    service "api" depends on unknown service "postgress"

New({auth: [config], config: [auth], web: [auth]})   → g, nil
g.Waves() → nil, &CycleError{Path: [auth config auth]}
            dependency cycle: auth -> config -> auth
```

## Run the tests

    go test ./exercises/35-deployorder/
    go test -race ./exercises/35-deployorder/
    go test -run TestWhy -v ./exercises/35-deployorder/

A stub that panics stops the whole test binary, so work one step at a time. Steps 1 to 5 do not
overlap:

    go test -run 'TestNew|TestErrorStrings' -v ./exercises/35-deployorder/   # step 1: New, Error()
    go test -run 'TestWaves|TestOrder' -v ./exercises/35-deployorder/        # step 2: Waves, Order
    go test -run TestCycle -v ./exercises/35-deployorder/                    # step 3: the cycle report
    go test -run TestDependents -v ./exercises/35-deployorder/               # step 4: Dependents
    go test -run TestWhy -v ./exercises/35-deployorder/                      # step 5: Why
    go test -race ./exercises/35-deployorder/                                # step 6: all, with Example and TestConcurrentUse

Each method call in the tests has a 5 s timeout. "did not return within 5s" means your search
loops on a cycle or walks the same services again and again. A recursive search that loops on a
cycle can crash the test binary first, with `goroutine stack exceeds 1000000000-byte limit`: same
cause. When a cycle test fails, the message also says whether your Path is a real cycle that the
rule did not pick, or not a cycle at all.

## Docs you can use

- [`slices`](https://pkg.go.dev/slices): `Sorted`, `Sort`, `Compact`, `Clone`, `Index`, `Reverse`, `Concat`
- [`maps.Keys`](https://pkg.go.dev/maps#Keys) and [Go maps in action](https://go.dev/blog/maps)
  (the section on iteration order)
- The spec: [For statements with range clause](https://go.dev/ref/spec#For_range) (map iteration
  order is not specified) and [Function literals](https://go.dev/ref/spec#Function_literals)
- [`errors.As`](https://pkg.go.dev/errors#As), [`errors.Is`](https://pkg.go.dev/errors#Is),
  [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) with `%w`
- [Go Slices: usage and internals](https://go.dev/blog/slices-intro) (why handing out a slice
  shares memory)
- Graph algorithms in general: [Topological sorting](https://en.wikipedia.org/wiki/Topological_sorting),
  [Breadth-first search](https://en.wikipedia.org/wiki/Breadth-first_search),
  [Depth-first search](https://en.wikipedia.org/wiki/Depth-first_search)

## Stretch goals

- Report every unknown dependency at once with `errors.Join`, as in 31-envconfig, so one CI run
  shows every typo.
- Teardown order for deleting an environment: dependents first, the reverse of Waves.
- Limit the parallelism: split each wave into batches of at most n services, and run them with the
  worker pool from 37-workerpool.
- Report every group of services caught in a cycle, not only the first cycle (Tarjan's strongly
  connected components), and print the graph in DOT format for Graphviz, as `terraform graph`
  does.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 35 hint 1
    go run ./cmd/peek 35 hint 2
    go run ./cmd/peek 35 hint 3
    go run ./cmd/peek 35 explanation
    go run ./cmd/peek 35 walkthrough
    go run ./cmd/peek 35 solution
