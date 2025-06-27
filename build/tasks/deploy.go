package tasks

import (
	"os"

	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func Deploy() goyek.Task {
	return goyek.Task{
		Name:  "deploy",
		Usage: "Deploys the controller onto Kubernetes",
		Action: func(a *goyek.A) {
			setupKoEnv(a)
			cmd.Exec(a,
				"go run github.com/google/ko@latest apply -B -f config/",
			)
		},
	}
}

func Undeploy() goyek.Task {
	return goyek.Task{
		Name:  "undeploy",
		Usage: "Removes the controller from Kubernetes",
		Action: func(a *goyek.A) {
			setupKoEnv(a)
			cmd.Exec(a,
				"go run github.com/google/ko@latest delete -f config/",
			)
		},
	}
}

func setupKoEnv(a *goyek.A) {
	a.Helper()

	if _, ok := os.LookupEnv("KO_DOCKER_REPO"); !ok {
		a.Setenv("KO_DOCKER_REPO", os.Getenv("IMAGE_BASENAME"))
	}
}
