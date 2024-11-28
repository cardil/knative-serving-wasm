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

set -Eeuo pipefail

# shellcheck disable=SC1090
source "$(go run knative.dev/hack/cmd/script codegen-library.sh)"

function run_yq() {
	go_run github.com/mikefarah/yq/v4@v4.23.1 "$@"
}

echo "=== Update Codegen for ${MODULE_NAME}"

group "Kubernetes Codegen"

source "${CODEGEN_PKG}/kube_codegen.sh"

kube::codegen::gen_helpers \
  --boilerplate "$(boilerplate)" \
  "${REPO_ROOT_DIR}/pkg/apis"

group "Knative Codegen"

# Knative Injection
"${KNATIVE_CODEGEN_PKG}/hack/generate-knative.sh" "injection" \
  github.com/cardil/knative-serving-wasm/pkg/client github.com/cardil/knative-serving-wasm/pkg/apis \
  "wasm:v1alpha1" \
  --go-header-file "$(boilerplate)"

group "Update CRD Schema"

go run "${REPO_ROOT_DIR}/cmd/schema" dump WasmModule \
  | run_yq eval-all --header-preprocess=false --inplace 'select(fileIndex == 0).spec.versions[0].schema.openAPIV3Schema = select(fileIndex == 1) | select(fileIndex == 0)' \
  "${REPO_ROOT_DIR}/config/300-wasmmodule.yaml" -

group "Update deps post-codegen"

# Make sure our dependencies are up-to-date
"${REPO_ROOT_DIR}/hack/update-deps.sh"
