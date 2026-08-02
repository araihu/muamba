#!/usr/bin/env bash

set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_output="$(mktemp -d)"
trap 'rm -r "$test_output"' EXIT

for invalid_minimum in not-a-number 69.9 -1 100.1; do
  if output="$(MIN_COVERAGE="$invalid_minimum" COVERAGE_DIR="$test_output" \
    "$project_root/scripts/check-coverage.sh" 2>&1)"; then
    echo "expected MIN_COVERAGE=$invalid_minimum to be rejected" >&2
    exit 1
  fi
  if [[ "$output" != *"MIN_COVERAGE must be a number between 70 and 100"* ]]; then
    echo "unexpected error for MIN_COVERAGE=$invalid_minimum: $output" >&2
    exit 1
  fi
done
