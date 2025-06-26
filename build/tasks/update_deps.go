package tasks

import (
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func UpdateDeps() goyek.Task {
	return goyek.Task{
		Name:   "update-deps",
		Usage:  "Update project dependencies",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "hack/update-deps.sh --upgrade")
		},
	}
}
