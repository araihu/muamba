#!/usr/bin/env bash

set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"

coverage_dir="${COVERAGE_DIR:-.coverage}"
minimum_coverage="${MIN_COVERAGE:-70.0}"

if [[ ! "$minimum_coverage" =~ ^([0-9]+([.][0-9]+)?|[.][0-9]+)$ ]] ||
  ! awk -v minimum="$minimum_coverage" \
    'BEGIN { value = minimum + 0; exit !(value >= 70 && value <= 100) }'; then
  echo "MIN_COVERAGE must be a number between 70 and 100" >&2
  exit 1
fi

mkdir -p "$coverage_dir"
go test -count=1 -covermode=atomic -coverprofile="$coverage_dir/coverage.out" ./...
go tool cover -func="$coverage_dir/coverage.out" | tee "$coverage_dir/coverage.txt"
go tool cover -html="$coverage_dir/coverage.out" -o "$coverage_dir/coverage.html"

total_coverage="$(awk '/^total:/ { gsub(/%/, "", $3); print $3 }' "$coverage_dir/coverage.txt")"
if [[ -z "$total_coverage" ]]; then
  echo "unable to read total coverage" >&2
  exit 1
fi

if ! awk -v total="$total_coverage" -v minimum="$minimum_coverage" \
  'BEGIN { exit !((total + 0) >= (minimum + 0)) }'; then
  echo "coverage ${total_coverage}% is below required ${minimum_coverage}%" >&2
  exit 1
fi

echo "coverage ${total_coverage}% meets required ${minimum_coverage}%"

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "### Go coverage"
    echo
    echo "Total: **${total_coverage}%** (minimum: ${minimum_coverage}%)"
  } >> "$GITHUB_STEP_SUMMARY"
fi
