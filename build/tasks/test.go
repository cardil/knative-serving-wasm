package tasks

import (
	"github.com/cardil/knative-serving-wasm/test/util/k8s"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

func Test(f *goyek.Flow) {
	f.Define(goyek.Task{
		Name:  "test",
		Usage: "Run tests",
		Deps: goyek.Deps{
			f.Define(Unit()),
			f.Define(BuildTest()),
			f.Define(E2e()),
		},
	})
}

func Unit() goyek.Task {
	return goyek.Task{
		Name:  "unit",
		Usage: "Runs unit tests for the project",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "test/presubmit-tests.sh --unit-tests")
			cmd.Exec(a, "cargo test", cmd.Dir("runner"))
			cmd.Exec(a, "cargo test", cmd.Dir("examples/modules/reverse-text"))
			cmd.Exec(a, "cargo test", cmd.Dir("examples/modules/http-fetch"))
		},
	}
}

func BuildTest() goyek.Task {
	return goyek.Task{
		Name:  "build-test",
		Usage: "Check if the project build properly, without code-gen changes",
		Action: func(a *goyek.A) {
			cmd.Exec(a, "test/presubmit-tests.sh --build-tests")
		},
	}
}

func E2e() goyek.Task {
	return goyek.Task{
		Name:  "e2e",
		Usage: "Runs e2e tests on Kubernetes",
		Action: func(a *goyek.A) {
			if !kubeAvailable(a) {
				a.Log("Kube unavailable. Skipping e2e tests.")
				return
			}
			cmd.Exec(a, "test/presubmit-tests.sh --integration-tests")
		},
	}
}

func kubeAvailable(a *goyek.A) bool {
	a.Helper()

	if err := k8s.CheckConnection(); err != nil {
		a.Log("Kube client failed: ", err)
		return false
	}

	return true
}
