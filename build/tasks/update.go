package tasks

import (
	"github.com/goyek/x/cmd"

	"github.com/goyek/goyek/v2"
)

func Update(f *goyek.Flow) {
	f.Define(goyek.Task{
		Name:  "update",
		Usage: "Update project",
		Deps: goyek.Deps{
			f.Define(UpdateDeps()),
			f.Define(UpdateCodegen()),
		},
	})
	f.Define(TidyDeps())
}

func UpdateCodegen() goyek.Task {
	return goyek.Task{
		Name:  "update-codegen",
		Usage: "Update project automatically generated code",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "hack/update-codegen.sh")
		},
	}
}

func UpdateDeps() goyek.Task {
	return goyek.Task{
		Name:  "update-deps",
		Usage: "Update project dependencies",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "hack/update-deps.sh --upgrade")
		},
	}
}

func TidyDeps() goyek.Task {
	return goyek.Task{
		Name:  "tidy",
		Usage: "Tidy up project dependencies",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "hack/update-deps.sh")
		},
	}
}
