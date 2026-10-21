#!/usr/bin/env bash
# Check that every hidden reference solution passes its exercise's tests.
#
# For each exercise, the script builds a temporary module from go.mod, the
# exercise's tests and testdata/, and the decoded solutions/<exercise>/*.go.rot13
# files, then runs the tests with the race detector. It copies none of the
# other files in exercises/, so your code there is not touched and does not
# get in the way. A test file you added or changed is copied, though.
#
# Usage: scripts/verify-solutions.sh [exercise ...]
#   scripts/verify-solutions.sh                  # all exercises
#   scripts/verify-solutions.sh 32-retry         # one exercise
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

if [ "$#" -gt 0 ]; then
  exercises=("$@")
else
  # nullglob: an empty exercises/ gives an empty list, not a literal "*".
  shopt -s nullglob
  exercises=()
  for d in exercises/*/; do exercises+=("$(basename "$d")"); done
  shopt -u nullglob
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

failed=()
for ex in "${exercises[@]}"; do
  sol="solutions/$ex"
  if [ ! -d "exercises/$ex" ] || [ ! -d "$sol" ]; then
    echo "FAIL $ex: exercises/$ex or $sol is missing"
    failed+=("$ex")
    continue
  fi
  for part in hints explanation walkthrough; do
    if [ ! -s "$sol/$part.md.rot13" ]; then
      echo "FAIL $ex: $sol/$part.md.rot13 is missing"
      failed+=("$ex")
      continue 2
    fi
  done

  work="$tmp/$ex"
  mkdir -p "$work/exercises/$ex"
  cp go.mod "$work/"
  cp "exercises/$ex"/*_test.go "$work/exercises/$ex/"
  if [ -d "exercises/$ex/testdata" ]; then
    cp -R "exercises/$ex/testdata" "$work/exercises/$ex/"
  fi
  for f in "$sol"/*.go.rot13; do
    name=$(basename "$f" .rot13)
    # ROT13 is its own inverse, so the same tr command encodes and decodes.
    tr 'A-Za-z' 'N-ZA-Mn-za-m' < "$f" > "$work/exercises/$ex/$name"
  done

  if (cd "$work" && go vet ./... && go test -race -count=1 ./...); then
    echo "ok   $ex"
  else
    echo "FAIL $ex"
    failed+=("$ex")
  fi
done

if [ "${#failed[@]}" -gt 0 ]; then
  echo "Failed: ${failed[*]}"
  exit 1
fi
echo "All ${#exercises[@]} reference solutions pass."
