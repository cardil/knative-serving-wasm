#!/usr/bin/env bash
# Copyright 2026 The Knative Authors.
# SPDX-License-Identifier: Apache-2.0

set -eEuo pipefail

# Delete all test run-specific images to avoid accumulation
# Package names are like: knative-serving-wasm/e2e-6/runner

RUN_ID="${1:?Usage: $0 <run_id> <owner>}"
OWNER="${2:?Usage: $0 <run_id> <owner>}"
PATTERN="e2e-${RUN_ID}"
FAILED=0

echo "Deleting packages containing: ${PATTERN}"

# List packages matching the pattern and delete them
# Packages are user-scoped but linked to repository, use /users/{owner}/packages
PACKAGES=$(gh api "/users/${OWNER}/packages?package_type=container" \
  --jq ".[] | select(.name | contains(\"${PATTERN}\")) | .name") || {
  echo "::error::Failed to list packages from GitHub API"
  exit 1
}

if [ -z "$PACKAGES" ]; then
  echo "No packages found matching pattern: ${PATTERN}"
  exit 0
fi

echo "Found packages to delete:"
echo "$PACKAGES"

while IFS= read -r package_name; do
  # URL-encode the package name (slashes become %2F)
  encoded_name=$(printf '%s' "$package_name" | jq -sRr @uri)
  echo "Deleting package: ${package_name}"
  if ! gh api --method DELETE "/users/${OWNER}/packages/container/${encoded_name}"; then
    echo "::error::Failed to delete package: ${package_name}"
    FAILED=1
  fi
done <<< "$PACKAGES"

if [ "$FAILED" -eq 1 ]; then
  echo "::error::Some packages failed to delete"
  exit 1
fi

echo "✅ Cleanup complete - deleted $(echo "$PACKAGES" | wc -l) package(s)"
