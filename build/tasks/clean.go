package tasks

import (
	"os"

	executil "github.com/cardil/knative-serving-wasm/build/util/exec"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func Clean() goyek.Task {
	return goyek.Task{
		Name:  "clean",
		Usage: "Cleans the project",
		Action: func(a *goyek.A) {
			executil.ExecOrDie(a, "cargo clean", cmd.Dir("runner"))
			executil.ExecOrDie(a, "cargo clean", cmd.Dir("examples/modules/reverse-text"))
			executil.ExecOrDie(a, "cargo clean", cmd.Dir("examples/modules/http-fetch"))
			deleteDir(a, "build/output")
		},
	}
}

func deleteDir(a *goyek.A, dir string) {
	a.Helper()

	if err := os.RemoveAll(dir); err != nil {
		a.Error(err)
	}
	a.Log("Dir removed: ", dir)
}
