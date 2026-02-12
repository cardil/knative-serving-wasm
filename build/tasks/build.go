package tasks

import (
	executil "github.com/cardil/knative-serving-wasm/build/util/exec"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func Build() goyek.Task {
	return goyek.Task{
		Name:  "build",
		Usage: "Builds the project",
		Action: func(a *goyek.A) {
			executil.ExecOrDie(a, "go test -v -run '^$' -tags 'e2e' ./...")
			executil.ExecOrDie(a, "go build -v -o build/output/controller ./cmd/controller")
			executil.ExecOrDie(a, "cargo build", cmd.Dir("runner"))
			buildExamples(a)
		},
	}
}

func buildExamples(a *goyek.A) {
	a.Helper()

	executil.ExecOrDie(a, "cargo build --target wasm32-wasip2 --release",
		cmd.Dir("examples/modules/reverse-text"))
	executil.ExecOrDie(a, "cargo build --target wasm32-wasip2 --release",
		cmd.Dir("examples/modules/http-fetch"))
}
