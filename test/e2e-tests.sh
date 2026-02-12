#!/usr/bin/env bash

# Copyright 2025 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script runs e2e tests for knative-serving-wasm.
# It delegates to the goyek e2e pipeline which handles all setup, build,
# deploy, and test execution.
#
# Usage:
#   ./test/e2e-tests.sh                    # Local mode (uses --run-tests)
#   ./test/e2e-tests.sh --run-tests        # Explicit local mode
#   ./test/e2e-tests.sh <other-flags>      # CI mode with specific flags

set -Eeo pipefail

cd "$(dirname "$0")/.."

# Delegate to goyek e2e pipeline with runner args passed via environment variable
exec env E2E_RUNNER_ARGS="$*" ./goyek --verbose e2e
