# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repository is

A Go practice course: 25 exercises for a reliability engineer (SRE / CRE), ordered by difficulty in 5 tiers. The owner solves them **without an LLM** (rule 1 in `README.md`). So do not implement stubs in `exercises/`, do not decode or summarise anything in `solutions/`, and do not give hints unless the user explicitly asks for that in the current conversation. Work on the course itself (tooling, specs, tests, docs) is fine.

## Commands

Go 1.24 or later, standard library only. `-race` needs cgo and a C compiler on Linux and Windows.

```bash
go vet ./...                                   # stubs and tests compile
gofmt -l .
go test ./cmd/...                              # tests of the peek tool
scripts/verify-solutions.sh [NN-name ...]      # each reference solution passes its tests under -race
scripts/check-stubs.sh [NN-name ...]           # each test fails fast (< 60 s) against the stub
go run ./cmd/peek 07 hint 1                    # decode 1 hidden part: hint 1|2|3, explanation, walkthrough, solution
```

CI (`.github/workflows/ci.yml`) runs gofmt, go vet, `go test ./cmd/...`, `check-stubs.sh` and `verify-solutions.sh`.

Do not run `go test ./...` or `go test ./exercises/...`: every stub panics with `TODO: implement`, so those tests fail by design. Test 1 exercise at a time. Each exercise README lists `-run` filters per step, for example `go test -race -run TestParseQuotedValues -v ./exercises/01-logfmt/`.

`check-stubs.sh` must run on a checkout where the stubs are not implemented. On an implemented exercise it reports the passing tests as errors.

## Architecture

### The three parts of an exercise

| Part | Path | Notes |
|---|---|---|
| Spec | `exercises/NN-slug/README.md` | Scenario, API, Rules, Examples, Run the tests (per-step `-run` filters), Docs you can use, Stretch goals, Stuck?. Line 3 is the tier and time line. |
| Stub and tests | `exercises/NN-slug/*.go`, `*_test.go`, `testdata/` | Package name is the slug without dashes (`01-logfmt` is `logfmt`). Every function body is `panic("TODO: implement")`. Tests are black-box (`package logfmt_test`) and import the package with an explicit alias. |
| Hidden material | `solutions/NN-slug/*.rot13` | `hints.md`, `explanation.md`, `walkthrough.md` and the reference solution. All ROT13. |

How the pieces depend on each other:

- Each reference solution file has **the same name as a stub file**. `verify-solutions.sh` builds a temporary module from `go.mod`, the exercise's `*_test.go` and `testdata/`, and the decoded `solutions/NN-slug/*.go.rot13`. The decoded files replace the stubs by name.
- `peek` finds the exercise by number prefix in `solutions/` and walks up from the working directory to the repository root. It splits `hints.md` on the exact headings `## Hint 1`, `## Hint 2` and `## Hint 3`.
- The root `README.md` table repeats each exercise's title, tier and time. Keep it equal to line 3 of each exercise README.

### Invariants of the course

- The tests are the executable spec. Every behaviour a test checks is stated in the exercise README. Nothing is hidden, and no test checks trivia or a trick.
- Every test fails fast against the stub, and the reference solution passes under `-race -count=3`. Tests stay deterministic: they inject clocks, sleep and randomness, and put a timeout around every blocking call. Do not assert tight wall-clock bounds.
- Explanations are written for a medior engineer. They do not explain basics such as loops or maps. They teach how to approach the problem and how production systems solve it. Later explanations point back to the exercise that first taught a concept.
- The headers and the root table name Go topics, not the approach. Naming the approach would give away what the hints are meant to reveal.

## Changing hidden material

ROT13 is its own inverse:

```bash
tr 'A-Za-z' 'N-ZA-Mn-za-m' < solutions/07-retry/retry.go.rot13 > /tmp/retry.go   # decode
tr 'A-Za-z' 'N-ZA-Mn-za-m' < /tmp/retry.go > solutions/07-retry/retry.go.rot13   # encode
```

Never commit a plaintext hint, explanation or solution. `.staging/` and `.blind/` are gitignored work areas for plaintext. After any change, run `verify-solutions.sh` for that exercise. If you changed a stub or a test, also run `check-stubs.sh` for it. If you changed a walkthrough or the solution, check that the code quoted in the walkthrough still matches the solution.

## Writing style

The READMEs use short sentences, the active voice, numbers as digits and no contractions. Comments in code give the reason, not the description.
