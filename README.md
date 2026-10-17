# go-practice

25 Go exercises of increasing difficulty. Each one is a task from the daily work of a reliability engineer (SRE / CRE): parse logs, compute SLO burn rates, retry with backoff, rate-limit clients, shed load, check TLS certificates, run a reconcile loop, balance traffic. You solve them without an LLM. Each exercise has a spec, a stub and tests. Hints, concept explanations and reference solutions are hidden in `solutions/` with ROT13, so you see them only when you decide to.

## Documents

| Document | Read it when |
|---|---|
| `exercises/NN-name/README.md` | You start an exercise. It is the spec. |
| `exercises/NN-name/*_test.go` | You want the exact expected behaviour. The tests are the executable spec. |
| `solutions/NN-name/` | You are stuck, or you are done and want to compare. Read it with `peek` (see below). |

## The rules

The goal is to train the part of your brain that solves problems. Prompting is a different skill.

1. Do not use an LLM. That includes chat assistants and AI autocomplete (Copilot, Cursor Tab, Codeium). Turn them off in your editor. `gopls` is fine.
2. Use documentation freely. `go doc`, [pkg.go.dev](https://pkg.go.dev/std), [the Go spec](https://go.dev/ref/spec), [Effective Go](https://go.dev/doc/effective_go), [the Go blog](https://go.dev/blog/), man pages and RFCs are all allowed. Finding the answer in the docs is part of the skill.
3. Try for at least 25 minutes before you reveal hint 1. Wait at least 10 minutes between hints.
4. If you read the reference solution, close it, wait, and write your own version from memory. Do not copy it.
5. Read the explanation and the walkthrough after you pass, too. They show the trade-offs and how production systems do it.

## How to work an exercise

1. Read the exercise README twice.
2. Read the tests. Find the case names and the helper functions. They tell you what "done" means.
3. Work 2 small examples by hand, on paper.
4. Write down the state your code must keep and what must always be true about it (the invariants).
5. Write the simplest version that can pass 1 test.
6. Run 1 test at a time: `go test -run TestParseQuotedValues -v ./exercises/01-logfmt/`.
7. Make it correct. Then make it clean.
8. If the exercise has goroutines, run the tests with `-race`.
9. Read the explanation and the walkthrough. Compare them with your solution.

## A problem-solving checklist

Ask these questions before you write code. They replace "ask the LLM".

- What are the inputs and the outputs, exactly? What are their types?
- What does the smallest interesting example look like? What is the answer by hand?
- What state must I keep between steps? What changes it?
- What must always be true about that state?
- What are the edges? Empty input, 1 element, duplicates, huge input, malformed input, a cancelled context, a clock that moves backwards, 2 goroutines at the same time.
- Which known shape is this? A parser (state machine), an aggregation (map, then sort), a scheduler (heap), dependencies (graph), a lifecycle (state machine), a fan-out (goroutines, `sync.WaitGroup`, `context`).
- Can I split it into pure functions plus a thin layer that does the effects (time, I/O, goroutines)? The pure part is easy to test.
- How do I know I am done? The tests, plus the edge cases from my own list.

If you are stuck:

- Make the failing case smaller until you understand it.
- Print the state at each step with `t.Logf` or `fmt.Printf("%+v\n", x)`.
- Explain the problem out loud, step by step. This is what you used the LLM for. Do it yourself.
- Read the docs of the function you call: `go doc bufio.Scanner.Buffer`.
- Walk away for 10 minutes.

## The exercises

Work them in order. Each tier assumes the concepts of the tiers before it. The times are for a medior engineer who works without an LLM.

| Tier | Exercises | Theme |
|---|---|---|
| 1 | 01–04 | Warm-up: Go idioms on real data |
| 2 | 05–09 | State, time, types and errors |
| 3 | 10–14 | Algorithms on real data, first concurrency |
| 4 | 15–19 | Network services and protocols |
| 5 | 20–25 | Distributed-systems building blocks |

| # | Exercise | Tier | Time | Go you practice | The real-life skill |
|---|---|---|---|---|---|
| 01 | [Parse logfmt log lines](exercises/01-logfmt/) | 1 | 1 h | a byte scanner, custom error types, `errors.Is`/`As`, `iota` enums | parse logs that `strings.Split` breaks |
| 02 | [Find the endpoints that hurt](exercises/02-accesslog/) | 1 | 1 h | maps of structs, `slices.SortFunc`, `cmp` | incident triage, label cardinality |
| 03 | [Percentiles and histogram quantiles](exercises/03-latency/) | 1 | 1 h | slice aliasing, sorting, float math, binary search | p99 versus `histogram_quantile` |
| 04 | [Streaming log filter with context](exercises/04-grep/) | 1 | 2 h | `io.Reader`/`io.Writer`, `bufio.Scanner`, a ring buffer | `grep -C` on huge logs in constant memory |
| 05 | [Error budgets and burn-rate alerts](exercises/05-slo/) | 2 | 1.5 h | float precision, time windows, pure functions | SLO burn-rate alerting |
| 06 | [Typed config from the environment](exercises/06-envconfig/) | 2 | 2.5 h | `reflect`, struct tags, `errors.Join` | twelve-factor config, masked secrets |
| 07 | [Retries with backoff, jitter and context](exercises/07-retry/) | 2 | 2.5 h | `context`, timers, error wrapping, generics | safe retries, `Retry-After` |
| 08 | [Token bucket and per-client limits](exercises/08-ratelimit/) | 2 | 2 h | `sync.Mutex`, an injected clock, map eviction | API rate limiting |
| 09 | [Cache: generic LRU with TTL](exercises/09-cache/) | 2 | 2 h | generics, a linked list plus a map, callbacks and locks | bounded caches with expiry |
| 10 | [Deploy order, waves and blast radius](exercises/10-deployorder/) | 3 | 2.5 h | graphs, Kahn's algorithm, DFS, BFS | sync waves, impact analysis |
| 11 | [Make sense of a distributed trace](exercises/11-tracetree/) | 3 | 2.5 h | trees from flat data, recursion, interval union | trace analysis, critical path |
| 12 | [Worker pool: bounded parallelism, ordered results, fail fast](exercises/12-workerpool/) | 3 | 2.5 h | goroutines, `sync.WaitGroup`, semaphores, cancellation | fan-out without goroutine leaks |
| 13 | [Merge logs from many pods in time order](exercises/13-logmerge/) | 3 | 2 h | `container/heap`, `iter.Seq`, `iter.Pull` | merge pod logs like `stern` |
| 14 | [Circuit breaker: stop hammering a dead dependency](exercises/14-circuitbreaker/) | 3 | 3 h | state machines, lock scope, generation counters | stop cascading failures |
| 15 | [Health checks: /livez, /readyz and graceful shutdown](exercises/15-healthcheck/) | 4 | 3 h | `net/http`, `httptest`, JSON, `Server.Shutdown` | Kubernetes probes, zero-downtime rollouts |
| 16 | [HTTP middleware that operators need](exercises/16-middleware/) | 4 | 3 h | decorators, wrapping `http.ResponseWriter`, `log/slog` | request IDs, access logs, load shedding |
| 17 | [Parse Prometheus metrics and compute rates](exercises/17-promtext/) | 4 | 3.5 h | a line lexer, `strconv.ParseFloat`, canonical keys | debug exporters, `rate()` with counter resets |
| 18 | [TLS certificate expiry checker](exercises/18-certcheck/) | 4 | 3 h | `crypto/tls`, `crypto/x509`, `errors.As`, deadlines | catch expiring certificates, intermediates too |
| 19 | [Alert dedup, grouping and inhibition](exercises/19-alertgroup/) | 4 | 4 h | canonical keys, hashing, simulated time | how Alertmanager groups and mutes alerts |
| 20 | [Singleflight: stop the cache stampede](exercises/20-singleflight/) | 5 | 3 h | per-key calls, panic propagation, reference counting | protect a database from a stampede |
| 21 | [Broadcaster: watch streams with slow consumers](exercises/21-broadcaster/) | 5 | 3.5 h | channel ownership, close protocols, ring buffers | watch streams, slow consumers, 410 Gone |
| 22 | [A Kubernetes-style controller](exercises/22-reconciler/) | 5 | 4 h | `sync.Cond`, a work queue, fakes for failure injection | reconcile loops, optimistic concurrency |
| 23 | [Leader election: leases and fencing tokens](exercises/23-leaderelection/) | 5 | 4 h | functional core, compare-and-swap, context trees | 1 active replica, safe failover |
| 24 | [Crash-safe disk buffer for a log shipper](exercises/24-diskbuffer/) | 5 | 4.5 h | `encoding/binary`, CRC-32C, `fsync`, atomic rename | durable buffering, crash recovery |
| 25 | [Capstone: a health-aware load balancer](exercises/25-loadbalancer/) | 5 | 6 h | `httputil.ReverseProxy`, atomics, goroutines bound to a context | what Envoy and NGINX do for you |

## Reveal hints and solutions

Run `peek` from the repository root. Reveal 1 level at a time.

```bash
go run ./cmd/peek 07 hint 1
go run ./cmd/peek 07 hint 2
go run ./cmd/peek 07 hint 3
go run ./cmd/peek 07 explanation
go run ./cmd/peek 07 walkthrough
go run ./cmd/peek 07 solution
```

| Level | What it gives you |
|---|---|
| Hint 1 | A nudge: the question to ask, or the concept that fits. |
| Hint 2 | The key insight or the main pitfall. |
| Hint 3 | An outline of the steps, without code. |
| Explanation | The concepts in depth, how to approach the problem, the pitfalls, and how production systems do it. No solution code. |
| Walkthrough | A walkthrough of the reference solution, the alternatives and the trade-offs. |
| Solution | The reference solution. It passes all tests. |

NOTE: The files are ROT13, not encryption. ROT13 only stops an accidental glance. Without Go, decode a file with `tr`:

```bash
tr 'A-Za-z' 'N-ZA-Mn-za-m' < solutions/07-retry/hints.md.rot13
```

## Run the tests

You need Go 1.24 or later.

```bash
go test ./exercises/01-logfmt/
go test -race ./exercises/12-workerpool/
go test -run TestParse -v ./exercises/01-logfmt/
```

NOTE: Every stub function panics with `TODO: implement`. A test for code you did not write yet fails with that panic. That is expected. Test 1 exercise at a time, not `./...`.

## Repository layout

```text
exercises/NN-name/          The spec (README.md), the stub you edit and the tests.
solutions/NN-name/          Hints, explanation, walkthrough and the reference solution, all ROT13.
cmd/peek/                   Decodes 1 part of solutions/ at a time.
scripts/verify-solutions.sh Runs every reference solution against its tests. CI runs it.
```

## Check the reference solutions

This script copies the module to a temporary directory, decodes each reference solution over its stub and runs the tests with the race detector. It does not touch your work in `exercises/`.

```bash
scripts/verify-solutions.sh            # all exercises
scripts/verify-solutions.sh 07-retry   # 1 exercise
```

## License

MIT. See [`LICENSE`](LICENSE).
