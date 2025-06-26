package tasks

import (
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func Deploy() goyek.Task {
	return goyek.Task{
		Name:  "deploy",
		Usage: "Deploys the controller onto Kuberentes",
		Action: func(a *goyek.A) {
			cmd.Exec(a,
				"go run github.com/google/ko@latest apply -f config/",
			)
		},
	}
}
