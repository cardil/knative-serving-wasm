package tasks

import (
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func Build() goyek.Task {
	return goyek.Task{
		Name:  "build",
		Usage: "Builds the project",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "go test -v -run '^$' -tags 'e2e' ./...")
			cmd.Exec(a, "go build -v -o build/output/controller ./cmd/controller")
			cmd.Exec(a, "cargo build", cmd.Dir("runner"))
			buildExamples(a)
		},
	}
}

func buildExamples(a *goyek.A) {
	a.Helper()

	cmd.Exec(a, "cargo build --target wasm32-wasip2 --release",
		cmd.Dir("examples/modules/reverse-text"))
}
