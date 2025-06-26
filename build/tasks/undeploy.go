package tasks

import (
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func Undeploy() goyek.Task {
	return goyek.Task{
		Name:   "undeploy",
		Usage:  "Removes the controller from Kuberentes",
		Action: func(a *goyek.A) {
			cmd.Exec(a,
				"go run github.com/google/ko@latest delete -f config/",
			)
		},
	}
}
