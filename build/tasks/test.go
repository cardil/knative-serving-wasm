package tasks

import (
	"os"

	executil "github.com/cardil/knative-serving-wasm/build/util/exec"
	"github.com/cardil/knative-serving-wasm/test/util/k8s"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

const (
	// E2ERunnerArgsEnv is the environment variable for passing args to the e2e runner
	E2ERunnerArgsEnv = "E2E_RUNNER_ARGS"
)

func Test(f *goyek.Flow) {
	f.Define(goyek.Task{
		Name:  "test",
		Usage: "Run tests",
		Deps: goyek.Deps{
			f.Define(Unit()),
			f.Define(Lint()),
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
			executil.ExecOrDie(a, "test/presubmit-tests.sh --unit-tests")
			executil.ExecOrDie(a, "cargo test", cmd.Dir("runner"))
			executil.ExecOrDie(a, "cargo test", cmd.Dir("examples/modules/reverse-text"))
			executil.ExecOrDie(a, "cargo test", cmd.Dir("examples/modules/http-fetch"))
		},
	}
}

func BuildTest() goyek.Task {
	return goyek.Task{
		Name:  "build-test",
		Usage: "Check if the project build properly, without code-gen changes",
		Action: func(a *goyek.A) {
			executil.ExecOrDie(a, "test/presubmit-tests.sh --build-tests")
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

			setupKoEnvDev(a)

			// Build images
			a.Log("Building images for e2e tests...")
			buildExamples(a)
			buildRunnerImage(a)

			// Push images to test registry
			a.Log("Pushing images to test registry...")
			pushExamples(a)
			pushRunnerImage(a)

			// Deploy controller
			a.Log("Deploying controller...")
			koApplyE2E(a)

			// Run e2e tests using the Knative test runner
			// E2E_RUNNER_ARGS controls behavior: --run-tests for local, other flags for CI
			runnerArgs := os.Getenv(E2ERunnerArgsEnv)
			if runnerArgs == "" {
				// Default to --run-tests for local development (skips Boskos/GCP initialization)
				runnerArgs = "--run-tests"
			}
			a.Logf("Running e2e tests with args: %s", runnerArgs)
			executil.ExecOrDie(a, "test/e2e/runner.sh "+runnerArgs)
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
