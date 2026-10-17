#!/usr/bin/env bash
# Check that every hidden reference solution passes its exercise's tests.
#
# For each exercise, the script copies the module into a temporary directory,
# decodes solutions/<exercise>/*.go.rot13 over the stub files and runs the
# tests with the race detector. Your own work in exercises/ is not touched.
#
# Usage: scripts/verify-solutions.sh [exercise ...]
#   scripts/verify-solutions.sh                  # all exercises
#   scripts/verify-solutions.sh 07-retry         # one exercise
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

if [ "$#" -gt 0 ]; then
  exercises=("$@")
else
  exercises=()
  for d in exercises/*/; do exercises+=("$(basename "$d")"); done
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
  mkdir -p "$work/exercises"
  cp go.mod "$work/"
  cp -R "exercises/$ex" "$work/exercises/"
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
