#!/usr/bin/env bash
# Copyright 2026 The Knative Authors.
# SPDX-License-Identifier: Apache-2.0

set -eEuo pipefail

# Delete all test run-specific images to avoid accumulation
# Package names are like: knative-serving-wasm/e2e-6/runner
# Known package suffixes from the e2e tests

RUN_ID="${1:?Usage: $0 <run_id> <owner> <repo>}"
OWNER="${2:?Usage: $0 <run_id> <owner> <repo>}"
REPO="${3:?Usage: $0 <run_id> <owner> <repo>}"
PATTERN="e2e-${RUN_ID}"
FAILED=0
DELETED=0

echo "Deleting packages containing: ${PATTERN}"

# Known package names from e2e tests
# These are the packages created during e2e test runs
PACKAGE_SUFFIXES=(
  "controller"
  "runner"
  "example/reverse-text"
  "example/http-fetch"
)

for suffix in "${PACKAGE_SUFFIXES[@]}"; do
  package_name="${REPO}/${PATTERN}/${suffix}"
  encoded_name=$(printf '%s' "$package_name" | jq -sRr @uri)
  
  echo "Attempting to delete package: ${package_name}"
  
  # Try to delete the package
  output=$(gh api --method DELETE "/users/${OWNER}/packages/container/${encoded_name}" 2>&1) && deleted=true || deleted=false
  
  if [ "$deleted" = "true" ]; then
    echo "✅ Deleted: ${package_name}"
    DELETED=$((DELETED + 1))
  else
    # Check if it's a 404 (package doesn't exist) or a real error
    if echo "$output" | grep -qi "not found.*404\|package not found"; then
      echo "::warning::Package not found (may not have been created): ${package_name}"
    else
      echo "::error::Failed to delete package: ${package_name}"
      echo "::error::Error: ${output}"
      FAILED=1
    fi
  fi
done

if [ "$FAILED" -eq 1 ]; then
  echo "::error::Some packages failed to delete"
  exit 1
fi

if [ "$DELETED" -eq 0 ]; then
  echo "::warning::No packages were deleted (they may not have been created yet)"
fi

echo "✅ Cleanup complete - deleted ${DELETED} package(s)"
