package tasks

import (
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func UpdateCodegen() goyek.Task {
	return goyek.Task{
		Name:   "update-codegen",
		Usage:  "Update project automatically generated code",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "hack/update-codegen.sh")
		},
	}
}
