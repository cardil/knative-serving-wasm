package tasks

import (
	"os"

	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func Clean() goyek.Task {

	return goyek.Task{
		Name:  "clean",
		Usage: "Clean the project",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "cargo clean", cmd.Dir("runner"))
			cmd.Exec(a, "cargo clean", cmd.Dir("examples/modules/reverse-text"))
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
