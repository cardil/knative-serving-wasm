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

# This script provides the e2e test runner logic, including Knative Serving setup
# and test execution with proper reporting.
#
# It sources the Knative hack e2e-tests.sh script to reuse:
# - start_knative_serving, get_latest_knative_yaml_source
# - go_test_e2e for proper test reporting
# - initialize (for CI mode only)
#
# Usage:
#   ./test/e2e/runner.sh                # CI mode - runs initialize, full setup
#   ./test/e2e/runner.sh --run-tests    # Local mode - skips initialize, uses existing cluster

set -Eeo pipefail

# shellcheck disable=SC1090
source "$(go run knative.dev/hack/cmd/script e2e-tests.sh)"

# Detect if we're running on a Kind cluster
# Kind nodes have providerID in format "kind://<provider>/<cluster-name>/<node-name>"
function is_kind_cluster() {
  kubectl get nodes -o jsonpath='{.items[0].spec.providerID}' 2>/dev/null | grep -q '^kind://'
}

# Detect if we're running on a minikube cluster
function is_minikube_cluster() {
  kubectl config current-context 2>/dev/null | grep -q '^minikube$'
}

# Detect if we need port-forwarding (Kind or minikube)
function needs_port_forward() {
  is_kind_cluster || is_minikube_cluster
}

function start_latest_knative_serving() {
  local KNATIVE_NET_KOURIER_RELEASE
  KNATIVE_NET_KOURIER_RELEASE="$(get_latest_knative_yaml_source "net-kourier" "kourier")"
  start_knative_serving "${KNATIVE_SERVING_RELEASE_CRDS}" \
    "${KNATIVE_SERVING_RELEASE_CORE}" \
    "${KNATIVE_NET_KOURIER_RELEASE}"

  kubectl patch configmap/config-network \
    --namespace knative-serving \
    --type merge \
    --patch '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'

  # Configure "No DNS" mode for e2e tests
  # This uses Host header routing with example.com domain instead of real DNS
  # This is needed for all clusters (local and cloud) to ensure consistent behavior
  # See: https://knative.dev/docs/install/yaml-install/serving/install-serving-with-yaml/#configure-dns
  echo "Configuring 'No DNS' mode with example.com domain for e2e tests"
  kubectl patch configmap/config-domain \
    --namespace knative-serving \
    --type merge \
    --patch '{"data":{"example.com":""}}'
}

# Start port-forward to Kourier for local clusters (Kind, minikube)
# This allows the test runner on the host to access the ingress gateway
function start_local_port_forward() {
  if needs_port_forward; then
    echo "Starting port-forward to Kourier gateway (localhost:31080 -> kourier:80)"
    kubectl port-forward -n kourier-system service/kourier 31080:80 &
    PORT_FORWARD_PID=$!
    export PORT_FORWARD_PID
    # Set environment variable for Go tests to use the forwarded address
    export LOCAL_GATEWAY_ADDRESS="localhost:31080"
    # Wait for port-forward to be ready
    sleep 3
    echo "Port-forward started (PID: $PORT_FORWARD_PID)"
    echo "Go tests will use LOCAL_GATEWAY_ADDRESS=$LOCAL_GATEWAY_ADDRESS"
  fi
}

# Stop port-forward when done
function stop_local_port_forward() {
  if [[ -n "${PORT_FORWARD_PID:-}" ]]; then
    echo "Stopping port-forward (PID: $PORT_FORWARD_PID)"
    kill "$PORT_FORWARD_PID" 2>/dev/null || true
  fi
}

function knative_setup() {
  start_latest_knative_serving
}

# Initialize test infrastructure (creates cluster, downloads releases, etc.)
# This is called by the Knative hack script via initialize "$@"
# The --run-tests flag skips Boskos/GCP initialization for local development
initialize "$@"

set -Eeuo pipefail

# Start port-forward for local clusters (Kind, minikube)
start_local_port_forward
trap stop_local_port_forward EXIT

# Run e2e tests with proper test reporting
go_test_e2e -timeout 30m -tags=e2e ./test/e2e/... || fail_test 'knative-serving-wasm e2e tests'

success
