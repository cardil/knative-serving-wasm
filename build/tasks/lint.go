// Copyright 2026 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tasks

import (
	executil "github.com/cardil/knative-serving-wasm/build/util/exec"
	"github.com/cardil/knative-serving-wasm/build/util/tools"
	"github.com/goyek/goyek/v2"
)

func Lint() goyek.Task {
	return goyek.Task{
		Name:  "lint",
		Usage: "Runs linters on the project",
		Action: func(a *goyek.A) {
			linter := tools.Ghet(a, "golangci-lint")
			executil.ExecOrDie(a, spaceJoin(linter, "run", "./..."))
		},
	}
}

func AutoFix() goyek.Task {
	return goyek.Task{
		Name:  "autofix",
		Usage: "Runs linters on the project and fixes what it can",
		Action: func(a *goyek.A) {
			linter := tools.Ghet(a, "golangci-lint")
			executil.ExecOrDie(a, spaceJoin(linter, "run", "--fix", "./..."))
		},
	}
}
