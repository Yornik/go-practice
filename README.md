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
6. Run 1 test at a time: `go test -run 'TestParse/quoted' -v ./exercises/01-logfmt/`.
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

The exercise table is added together with the exercises.

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
