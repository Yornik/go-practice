# go-practice

50 Go exercises of increasing difficulty. Each one is a task from the daily work of a reliability engineer (SRE / CRE): count log levels, compare versions, find the feeders that went quiet, parse logs, compute SLO burn rates, retry with backoff, rate-limit clients, check TLS certificates, run a reconcile loop, balance traffic. You solve them without an LLM. Each exercise has a spec, a stub and tests. Hints, concept explanations and reference solutions are hidden in `solutions/` with ROT13, so you see them only when you decide to.

## Documents

| Document | Read it when |
|---|---|
| `exercises/NN-name/README.md` | You start an exercise. It is the spec. |
| `exercises/NN-name/*_test.go` | You want the exact expected behaviour. The tests are the executable spec. |
| `solutions/NN-name/` | You are stuck, or you are done and want to compare. Read it with `peek` (see below). |

## The story

You are the new junior SRE at Peckish, a small startup that sells the Peckish Feeder: a bird feeder with a camera, a seed sensor and Wi-Fi. 40 000 feeders report in. A squirrel called Kevin lives in the tree next to the demo feeder outside the office window, and whoever is on call keeps a ceramic owl on their desk. Ines, the senior SRE, writes the runbooks. Every runbook starts with "Step 0: breathe."

Each exercise opens with 1 moment from your time there: a dashboard that showed 0 errors since March, a rollout script that thinks `v2.9.3` is newer than `v2.10.0`, a syslog line from the future on 1 January. Tiers 1 to 5 are your first 4 months. Tiers 6 to 10 are year 2, when Peckish is 10 times bigger. The story is only in the opening paragraph. The rules, the tests and the hidden material stay exact and plain.

Peckish, its feeders and its people are fictional.

## Why this exists

An LLM can write the code for each of these tasks in seconds. If it always does, a set of skills fades: breaking a vague task into steps, choosing a data structure, reading the docs until you understand a function, and debugging from a failing test to the cause. These exercises train exactly those skills. The tasks are real: you meet each of them in production work, and none of them is a puzzle with a trick. The difficulty comes from the problem itself.

Tiers 1 to 5 assume that you can program and did [A Tour of Go](https://go.dev/tour/) once. Their explanations also cover Go basics where a bug hides in them, such as map order, slices that share memory and pointer receivers. Tiers 6 to 10 assume that you write Go fluently. Their explanations do not explain loops, structs or maps. All explanations cover the Go semantics that cause bugs, the design choices, and how production systems solve the same problem.

## Start here

1. Make sure you have Go 1.24 or later: `go version`.
2. Make sure the race detector works. On Linux and Windows `-race` needs cgo and a C compiler (`sudo apt install gcc`, `sudo dnf install gcc`, or MinGW-w64 on Windows). On Linux and Windows, `go env CGO_ENABLED` must print `1`. macOS needs nothing extra.
3. If basic Go syntax is new to you, do [A Tour of Go](https://go.dev/tour/) first. Tier 1 starts right after it.
4. Create your own branch for your work: `git switch -c my-solutions`. To get fixes to the course later, run `git pull origin main`.
5. Run the first tests: `go test ./exercises/01-loglevels/`. They fail with `panic: TODO: implement`. That is expected.
6. Open `exercises/01-loglevels/README.md` and edit `exercises/01-loglevels/loglevels.go`.

If you write Go fluently, you can start at tier 6 (`exercises/26-logfmt/`). Skim the openings of 01 to 25 for the story.

An exercise is done when `go test -race -count=1 ./exercises/NN-name/` prints `ok`. To start an exercise again, run `git restore --source=origin/main exercises/NN-name/`. A plain `git restore exercises/NN-name/` only drops changes that you did not commit.

NOTE: Expect the first 2 or 3 exercises to take longer than the time in the table while your Go comes back.

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
6. Run 1 test at a time: `go test -run TestParseQuotedValues -v ./exercises/26-logfmt/`.
7. Make it correct. Then make it clean.
8. If the exercise has goroutines, run the tests with `-race`.
9. Read the explanation and the walkthrough. Compare them with your solution.
10. Write down the time you spent, the hints you used and where you got stuck. Over 50 exercises this log shows your progress.

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

Work them in order. Each tier assumes the concepts of the tiers before it. The times in tiers 1 to 5 are for an engineer who did A Tour of Go once. The times in tiers 6 to 10 are for a medior engineer. Both work without an LLM. Together they are about 102 hours, which is about 4 months at 2 sessions a week.

| Tier | Exercises | Name | Theme |
|---|---|---|---|
| 1 | 01–05 | First week | Go basics on small real tasks: strings, maps, slices and sorting |
| 2 | 06–10 | Carrying the Owl | Types with methods, errors you can inspect, enums, time values, text as bytes |
| 3 | 11–15 | Your own tools | Readers and writers, closures, float math, binary search, a testable command |
| 4 | 16–20 | Black Friday | Interfaces the standard library calls, generics, `reflect`, goroutines and a mutex |
| 5 | 21–25 | The long nights | `context`, an injected clock, eviction, and hand-written scanners with typed errors |
| 6 | 26–28 | The morning after | Warm-up: Go idioms on real data |
| 7 | 29–34 | Growing pains | Streams, state, time, types, errors and locks |
| 8 | 35–39 | Spring launch | Algorithms on real data, your own goroutines |
| 9 | 40–44 | Dashboards that lie | Services, protocols and alerting logic |
| 10 | 45–50 | Passing the Owl | Distributed-systems building blocks |

The "Go you practice" column names the topics, not the approach. Finding the approach is part of each exercise.

| # | Exercise | Tier | Time | Go you practice | The real-life skill |
|---|---|---|---|---|---|
| 01 | [Count the errors the dashboard missed](exercises/01-loglevels/) | 1 | 1 h | `strings`, maps, `switch` | count log levels that every service spells differently |
| 02 | [Last message repeated 8599 times](exercises/02-repeated/) | 1 | 1 h | slices, `append`, loops that remember | collapse log noise before you read it |
| 03 | [Was it Kevin? Count the pages](exercises/03-pagenoise/) | 1 | 1 h | maps as counters and sets, struct keys, `slices.SortFunc`, `cmp` | alert-fatigue review |
| 04 | [Wave 2 got the firmware twice](exercises/04-waves/) | 1 | 1 h | slices share backing arrays, `copy`, `slices.Clone` | rollouts in waves with a canary |
| 05 | [Read a .env file without bash](exercises/05-envfile/) | 1 | 1.5 h | string rules, maps, errors with line numbers | read config as data, not as code |
| 06 | [Firmware v2.10 is newer than v2.9](exercises/06-semver/) | 2 | 1 h | methods, `String()`, compare functions, sentinel errors | compare versions in rollout scripts |
| 07 | [Which feeders went quiet?](exercises/07-rollcall/) | 2 | 1 h | `time.Parse`, comparing `time.Time`, sets, passing `now` in | dead man's switch, `absent()` alerts |
| 08 | [No deploys on Friday after 15:00](exercises/08-deploynote/) | 2 | 1 h | custom error types, `errors.As`, `errors.Join` | deploy gates that report every problem |
| 09 | [All systems operational (they were not)](exercises/09-statuspage/) | 2 | 1.5 h | `iota`, `String()`, pointer receivers, the zero value | status pages and worst-of aggregation |
| 10 | [The pigeon emoji broke Android](exercises/10-pushtext/) | 2 | 1 h | bytes vs runes, `unicode/utf8`, `strings.Builder` | byte limits on text, error positions |
| 11 | [What happened last night?](exercises/11-nightlog/) | 3 | 1 h | `io.Reader`, `bufio.Scanner`, `io.Writer`, `testing/iotest` | summarise a log you cannot open; find the silences |
| 12 | [Replace the grep pipeline in the runbook](exercises/12-linefilter/) | 3 | 1.5 h | function types, closures, variadic functions | composable log filters and sampling |
| 13 | [How many minutes is 99.95%?](exercises/13-nines/) | 3 | 1.5 h | float64, `NaN` and `Inf`, `time.Duration`, intervals | SLA arithmetic and real downtime |
| 14 | [Bisect the firmware](exercises/14-bisect/) | 3 | 1 h | binary search, `sort.Search`, functions as parameters | `git bisect`, lookups by time |
| 15 | [A disk check that can fail](exercises/15-checkdisk/) | 3 | 1.5 h | `flag.FlagSet`, a testable `Run`, exit codes | Nagios-style check plugins |
| 16 | [Secrets that refuse to print](exercises/16-redact/) | 4 | 1 h | interfaces that `fmt`, `json` and `slog` look for, `net/url` | keep secrets out of logs and JSON |
| 17 | [Keep the last 500 events](exercises/17-flightrec/) | 4 | 1 h | generics, `comparable`, the zero value of a type parameter | in-memory flight recorders |
| 18 | [kubectl get, for anything](exercises/18-tableprint/) | 4 | 1.5 h | `reflect`, struct tags, `fmt.Stringer` | table output for any type |
| 19 | [Check every service at once](exercises/19-fanout/) | 4 | 1 h | goroutines, `sync.WaitGroup`, `recover`, closing a channel | parallel health checks without leaks |
| 20 | [Count photos without crashing](exercises/20-counters/) | 4 | 1 h | `sync.Mutex`, `-race`, lock scope | safe counters, scrape and reset |
| 21 | [Wait for the rollout, not for 5 minutes](exercises/21-rollout/) | 5 | 1.5 h | channels, `select`, `context` | `kubectl rollout status`, progress deadlines |
| 22 | [Do not page me for a squirrel](exercises/22-alertstate/) | 5 | 1 h | an `iota` state type, an injected clock | Prometheus `for` and `keep_firing_for` |
| 23 | [Every silence ends](exercises/23-silences/) | 5 | 1 h | a mutex with a clock, map eviction, callbacks outside a lock | alert silences that expire |
| 24 | [Silence it for 2 days, not 2 seconds](exercises/24-durations/) | 5 | 1.5 h | a byte scanner, overflow checks, an error type with `Unwrap` | duration flags with days and weeks |
| 25 | [Read the feeders' syslog](exercises/25-syslog/) | 5 | 1.5 h | byte positions, `time.Parse` layouts, an `iota` enum, `errors.As` | RFC 3164 syslog, the missing year |
| 26 | [Parse logfmt log lines](exercises/26-logfmt/) | 6 | 1.5 h | a byte scanner, custom error types, `errors.Is`/`As`, `iota` enums | parse logs that `strings.Split` breaks |
| 27 | [Find the endpoints that hurt](exercises/27-accesslog/) | 6 | 1 h | maps of structs, `slices.SortFunc`, `cmp` | incident triage, label cardinality |
| 28 | [Percentiles and histogram quantiles](exercises/28-latency/) | 6 | 1.5 h | slice aliasing, sorting, float math, binary search | p99 versus `histogram_quantile` |
| 29 | [Streaming log filter with context](exercises/29-grep/) | 7 | 2.5 h | `io.Reader`/`io.Writer`, `bufio.Scanner`, error paths | `grep -C` on huge logs in constant memory |
| 30 | [Error budgets and burn-rate alerts](exercises/30-slo/) | 7 | 1.5 h | float precision, time windows, pure functions | SLO burn-rate alerting |
| 31 | [Typed config from the environment](exercises/31-envconfig/) | 7 | 2.5 h | `reflect`, struct tags, `errors.Join` | twelve-factor config, masked secrets |
| 32 | [Retries with backoff, jitter and context](exercises/32-retry/) | 7 | 2.5 h | `context`, timers, error wrapping, generics | safe retries, `Retry-After` |
| 33 | [Token bucket and per-client limits](exercises/33-ratelimit/) | 7 | 2 h | `sync.Mutex`, an injected clock, map eviction | API rate limiting |
| 34 | [Cache: generic LRU with TTL](exercises/34-cache/) | 7 | 2 h | generics, O(1) data structures, callbacks and locks | bounded caches with expiry |
| 35 | [Deploy order, waves and blast radius](exercises/35-deployorder/) | 8 | 2.5 h | graphs: ordering, cycles, reachability | sync waves, impact analysis |
| 36 | [Make sense of a distributed trace](exercises/36-tracetree/) | 8 | 2.5 h | trees from flat data, recursion, time intervals | trace analysis, critical path |
| 37 | [Worker pool: bounded parallelism, ordered results, fail fast](exercises/37-workerpool/) | 8 | 2.5 h | goroutines, `sync.WaitGroup`, semaphores, cancellation | fan-out without goroutine leaks |
| 38 | [Merge logs from many pods in time order](exercises/38-logmerge/) | 8 | 2.5 h | `iter.Seq`, range-over-func, cleanup on early exit | merge pod logs like `stern` |
| 39 | [Circuit breaker: stop hammering a dead dependency](exercises/39-circuitbreaker/) | 8 | 3.5 h | state machines, lock scope, late results | stop cascading failures |
| 40 | [Health checks: /livez, /readyz and graceful shutdown](exercises/40-healthcheck/) | 9 | 3 h | `net/http`, `httptest`, JSON, `Server.Shutdown` | Kubernetes probes, zero-downtime rollouts |
| 41 | [HTTP middleware that operators need](exercises/41-middleware/) | 9 | 3 h | decorators, wrapping `http.ResponseWriter`, `log/slog` | request IDs, access logs, load shedding |
| 42 | [Parse Prometheus metrics and compute rates](exercises/42-promtext/) | 9 | 4.5 h | a line lexer, `strconv.ParseFloat`, canonical keys | debug exporters, `rate()` with counter resets |
| 43 | [TLS certificate expiry checker](exercises/43-certcheck/) | 9 | 3 h | `crypto/tls`, `crypto/x509`, `errors.As`, deadlines | catch expiring certificates, intermediates too |
| 44 | [Alert dedup, grouping and inhibition](exercises/44-alertgroup/) | 9 | 4 h | canonical keys, hashing, simulated time | how Alertmanager groups and mutes alerts |
| 45 | [Singleflight: stop the cache stampede](exercises/45-singleflight/) | 10 | 3 h | waking many waiters, panics across goroutines, shared lifetimes | protect a database from a stampede |
| 46 | [Broadcaster: watch streams with slow consumers](exercises/46-broadcaster/) | 10 | 3.5 h | channel ownership, close protocols, ring buffers | watch streams, slow consumers, 410 Gone |
| 47 | [A Kubernetes-style controller](exercises/47-reconciler/) | 10 | 4 h | `sync.Cond`, a work queue, fakes for failure injection | reconcile loops, optimistic concurrency |
| 48 | [Leader election: leases and fencing tokens](exercises/48-leaderelection/) | 10 | 4 h | functional core, compare-and-swap, context trees | 1 active replica, safe failover |
| 49 | [Crash-safe disk buffer for a log shipper](exercises/49-diskbuffer/) | 10 | 4.5 h | `encoding/binary`, CRC-32C, `fsync`, atomic rename | durable buffering, crash recovery |
| 50 | [Capstone: a health-aware load balancer](exercises/50-loadbalancer/) | 10 | 6 h | `httputil.ReverseProxy`, atomics, goroutines bound to a context | what Envoy and NGINX do for you |

## Reveal hints and solutions

Run `peek` from the repository root. From an exercise directory, use `go run ../../cmd/peek` instead. Reveal 1 level at a time.

```bash
go run ./cmd/peek 32 hint 1
go run ./cmd/peek 32 hint 2
go run ./cmd/peek 32 hint 3
go run ./cmd/peek 32 explanation | less
go run ./cmd/peek 32 walkthrough | less
go run ./cmd/peek 32 solution
```

| Level | What it gives you |
|---|---|
| Hint 1 | A nudge: the question to ask, or the concept that fits. |
| Hint 2 | The key insight or the main pitfall. |
| Hint 3 | An outline of the steps. It can name the key functions, but it has no solution code. |
| Explanation | The concepts in depth, how to approach the problem, the pitfalls, and how production systems do it. No solution code. |
| Walkthrough | A walkthrough of the reference solution, the alternatives and the trade-offs. |
| Solution | The reference solution. It passes all tests. |

NOTE: The files are ROT13, not encryption. ROT13 only stops an accidental glance. Without Go, decode a file with `tr`:

```bash
tr 'A-Za-z' 'N-ZA-Mn-za-m' < solutions/32-retry/hints.md.rot13
```

## Run the tests

```bash
go test ./exercises/26-logfmt/
go test -race ./exercises/37-workerpool/
go test -run TestParse -v ./exercises/26-logfmt/
```

NOTE: Every stub function panics with `TODO: implement`. A test for code you did not write yet fails with that panic. That is expected. Test 1 exercise at a time, not `./...`. Each exercise README lists the test filters for each step.

## Make it a tool

The exercises are libraries, because libraries are easy to test. An SRE ships small tools. After you pass an exercise, you can wrap it in a command. These follow-ups have no tests:

| Exercise | Tool |
|---|---|
| 02-repeated | A `collapse` filter on stdin that folds repeated lines. |
| 11-nightlog | `nightlog -gap 10m < feeder.log`: a summary of the night and its silences. |
| 12-linefilter | A `-filter` flag for any log command. |
| 13-nines | `nines -objective 99.95 outages.csv`: the budget and the real downtime. |
| 18-tableprint | `tableprint`: aligned columns from JSON lines. |
| 25-syslog | `syslog2logfmt`: each syslog line as a logfmt line, the input of 26-logfmt. |
| 26-logfmt | A filter on stdin: `logfilter -level warn -match db=orders`. |
| 27-accesslog | An incident report from an access log file. |
| 29-grep | Your own `grep -C` with a regular expression. |
| 38-logmerge | A `stern`-like merge of several log files. |
| 42-promtext | A `promtool check metrics`-like linter, plus `rate` between 2 scrapes. |
| 43-certcheck | A cron job that exits with 0 (OK), 1 (warning) or 2 (critical). |
| 50-loadbalancer | A runnable load balancer with flags for the backends. |

Write `main` as a thin shell around a function you can test:

```go
func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Parse flags with flag.NewFlagSet, do the work, return the exit code.
	return 0
}
```

A test calls `run` with a `strings.Reader` and 2 `bytes.Buffer` values. It needs no process and no real files.

## Repository layout

```text
exercises/NN-name/          The spec (README.md), the stub you edit and the tests.
solutions/NN-name/          Hints, explanation, walkthrough and the reference solution, all ROT13.
cmd/peek/                   Decodes 1 part of solutions/ at a time.
scripts/verify-solutions.sh Runs every reference solution against its tests. CI runs it.
scripts/check-stubs.sh      Checks that every test fails fast against the stubs. CI runs it.
```

## Check the reference solutions

This script builds a temporary module from each exercise's tests, its `testdata/` and its decoded reference solution, and runs the tests with the race detector. It ignores your other files in `exercises/`. It does use your copy of the test files, so a test file that you added or changed can make it fail.

```bash
scripts/verify-solutions.sh            # all exercises
scripts/verify-solutions.sh 32-retry   # 1 exercise
```

## Change an exercise

ROT13 is its own inverse, so the same `tr` command decodes and encodes.

1. Decode the file you want to change: `tr 'A-Za-z' 'N-ZA-Mn-za-m' < solutions/32-retry/retry.go.rot13 > /tmp/retry.go`.
2. Edit the plain file.
3. Encode it back: `tr 'A-Za-z' 'N-ZA-Mn-za-m' < /tmp/retry.go > solutions/32-retry/retry.go.rot13`.
4. Run `scripts/verify-solutions.sh 32-retry`.
5. If you changed a stub or a test, run `scripts/check-stubs.sh 32-retry` on a clean checkout.

## License

MIT. See [`LICENSE`](LICENSE).
