#!/usr/bin/env bash

# Copyright 2021 The Knative Authors
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


# shellcheck disable=SC1090
source "$(go run knative.dev/hack/cmd/script release.sh)"

function build_release() {
  # Delegate to goyek release:build which handles:
  #   - runner multi-arch image (buildah, local only)
  #   - controller image (ko resolve --push=false, OCI layout)
  #   - YAML generation and checksums
  # TAG and KO_DOCKER_REPO are already set by the Knative release framework.
  # RELEASE_VERSION is derived from TAG (strip the "v" prefix).
  RELEASE_VERSION="${TAG#v}" ./goyek --no-deps release-build

  # Read the artifact list produced by release:build
  ARTIFACTS_TO_PUBLISH="$(tr '\n' ' ' < build/output/release/artifacts-to-publish.list)"
}

main "$@"
