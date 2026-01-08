#!/usr/bin/env bash

# Copyright 2024 The Knative Authors
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

set -Eeo pipefail

# shellcheck disable=SC1090
source "$(go run knative.dev/hack/cmd/script e2e-tests.sh)"

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
}

function knative_setup() {
  start_latest_knative_serving
}

initialize "$@"

set -Eeuo pipefail

# Run e2e tests (goyek e2e task handles build, publish, deploy, and test)
go_test_e2e -timeout 30m -tags=e2e ./test/e2e/... || fail_test 'knative-serving-wasm e2e tests'

success
