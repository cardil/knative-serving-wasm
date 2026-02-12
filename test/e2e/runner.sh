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
# Minikube nodes have labels containing "minikube.k8s.io"
function is_minikube_cluster() {
  kubectl get nodes -o jsonpath='{.items[0].metadata.labels}' 2>/dev/null | grep -q 'minikube'
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
    # Wait for Kourier gateway pods to be ready before port-forwarding
    echo "DEBUG: Waiting for Kourier gateway pods to be ready..."
    wait_until_pods_running kourier-system || return 1
    
    echo "DEBUG: Starting port-forward to Kourier gateway (localhost:31080 -> kourier:80)"
    kubectl port-forward -n kourier-system service/kourier 31080:80 > /tmp/port-forward.log 2>&1 &
    PORT_FORWARD_PID=$!
    export PORT_FORWARD_PID
    # Set environment variable for Go tests to use the forwarded address
    export LOCAL_GATEWAY_ADDRESS="localhost:31080"
    
    echo -n "DEBUG: Waiting for port-forward to be ready (PID: $PORT_FORWARD_PID)"
    # Wait for port to be accessible (timeout after 120 seconds)
    for i in {1..120}; do
      # Check if process is still running (fail fast if it died)
      if ! ps -p "$PORT_FORWARD_PID" > /dev/null 2>&1; then
        echo -e "\nERROR: Port-forward process (PID: $PORT_FORWARD_PID) died!"
        cat /tmp/port-forward.log
        return 1
      fi
      # Check if port is accessible
      if nc -z localhost 31080 2>/dev/null; then
        echo -e "\nDEBUG: Port-forward verified successfully"
        echo "DEBUG: Go tests will use LOCAL_GATEWAY_ADDRESS=$LOCAL_GATEWAY_ADDRESS"
        return 0
      fi
      echo -n "."
      sleep 1
    done
    
    # Timeout - port-forward failed
    echo -e "\nERROR: Timeout waiting for port 31080 to be accessible!"
    if ps -p "$PORT_FORWARD_PID" > /dev/null 2>&1; then
      echo "Port-forward process is still running (PID: $PORT_FORWARD_PID)"
    else
      echo "Port-forward process is not running"
    fi
    cat /tmp/port-forward.log
    return 1
  else
    echo "DEBUG: Port-forward not needed for this cluster type"
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
  # Start port-forward for local clusters (Kind, minikube) after Knative is set up
  start_local_port_forward
  trap stop_local_port_forward EXIT
}

# Initialize test infrastructure (creates cluster, downloads releases, etc.)
# This is called by the Knative hack script via initialize "$@"
# The --run-tests flag skips Boskos/GCP initialization for local development
initialize "$@"

set -Eeuo pipefail

# Run e2e tests with proper test reporting
# Default timeout: 6m (5 tests * 1m each + 1m buffer)
# Override with E2E_SUITE_TIMEOUT env var (in minutes)
SUITE_TIMEOUT="${E2E_SUITE_TIMEOUT:-6}m"
go_test_e2e -timeout "$SUITE_TIMEOUT" -tags=e2e ./test/e2e/... || fail_test 'knative-serving-wasm e2e tests'

success
