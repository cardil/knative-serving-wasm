package tasks

import (
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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
		},
	}
}

func BuildTest() goyek.Task {
	return goyek.Task{
		Name:  "build-test",
		Usage: "Check if the project build properly, and without code-gen changes",
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
			cmd.Exec(a, "test/presubmit-tests.sh --e2e-tests")
		},
	}
}

func kubeAvailable(a *goyek.A) bool {
	a.Helper()

	loadingRules := &clientcmd.ClientConfigLoadingRules{}
	overrides := &clientcmd.ConfigOverrides{}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	config, err := loader.ClientConfig()
	if err != nil {
		a.Log("Kube client failed: ", err)
		return false
	}

	// create the clientset
	_, err = kubernetes.NewForConfig(config)
	if err != nil {
		a.Log("kube error: ", err)
		return false
	}

	return true
}
