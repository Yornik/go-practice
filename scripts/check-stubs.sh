#!/usr/bin/env bash
# Check that every test fails against the stubs, and fails fast.
#
# A test that passes against a stub checks nothing, and a test that hangs
# against a stub leaves a learner staring at a frozen terminal. The script
# compiles each exercise's test binary once and runs every test, example
# and fuzz seed corpus on its own with a 60 s limit.
#
# Run it on a clean checkout: once you implement an exercise, its tests
# pass, and this script reports them.
#
# Usage: scripts/check-stubs.sh [exercise ...]
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

bad=()
total=0
for ex in "${exercises[@]}"; do
  bin="$tmp/$ex.test"
  go test -c -o "$bin" "./exercises/$ex/"
  # The binary runs in the package directory, so testdata/ paths resolve.
  pushd "exercises/$ex" >/dev/null
  before=${#bad[@]}
  # Listing runs package init. A panic there would kill every test, so it
  # counts as a failure of the exercise, as does a binary with no tests.
  if ! names=$("$bin" -test.list '.*' 2>&1) || [ -z "$names" ]; then
    echo "FAIL $ex: the test binary crashed or lists no tests"
    echo "$names" | tail -5
    bad+=("$ex")
    popd >/dev/null
    continue
  fi
  for name in $names; do
    case $name in Benchmark*) continue ;; esac
    total=$((total + 1))
    if out=$("$bin" -test.run "^${name}\$" -test.count 1 -test.timeout 60s 2>&1); then
      echo "PASS against the stub: $ex $name"
      bad+=("$ex/$name")
    elif grep -q "test timed out" <<<"$out"; then
      echo "HANG against the stub: $ex $name"
      bad+=("$ex/$name")
    fi
  done
  popd >/dev/null
  if [ "${#bad[@]}" -eq "$before" ]; then
    echo "ok   $ex"
  else
    echo "FAIL $ex"
  fi
done

if [ "${#bad[@]}" -gt 0 ]; then
  echo "Not failing fast against the stubs: ${bad[*]}"
  exit 1
fi
echo "All $total tests in ${#exercises[@]} exercises fail fast against the stubs."
