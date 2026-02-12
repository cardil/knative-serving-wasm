package tasks

import (
	"net"
	"os"
	"time"

	executil "github.com/cardil/knative-serving-wasm/build/util/exec"
	"github.com/cardil/knative-serving-wasm/test/util/k8s"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
	"github.com/joho/godotenv"
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

			// Set up IMAGE_BASENAME and KO_DOCKER_REPO for e2e tests
			// This must be done before building images so they get tagged correctly
			setupE2EImageBasename(a)
			setupKoEnv(a)

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

// setupE2EImageBasename ensures IMAGE_BASENAME is set to a test registry for e2e tests
func setupE2EImageBasename(a *goyek.A) {
	a.Helper()

	// Load production IMAGE_BASENAME from .env
	productionBasename := ""
	if envMap, err := godotenv.Read(".env"); err == nil {
		productionBasename = envMap["IMAGE_BASENAME"]
	}

	// Check for explicit E2E_IMAGE_BASENAME
	if e2eBasename := os.Getenv("E2E_IMAGE_BASENAME"); e2eBasename != "" {
		a.Setenv("IMAGE_BASENAME", e2eBasename)
		a.Logf("Using E2E_IMAGE_BASENAME: %s", e2eBasename)
		return
	}

	// Check if IMAGE_BASENAME is already set and differs from production
	if currentBasename := os.Getenv("IMAGE_BASENAME"); currentBasename != "" && currentBasename != productionBasename {
		a.Logf("Using existing IMAGE_BASENAME: %s", currentBasename)
		return
	}

	// Try to detect and use local registry
	localRegistry := "localhost:5001"
	if isLocalRegistryAvailable(localRegistry) {
		imageBasename := localRegistry + "/knative-serving-wasm"
		a.Setenv("IMAGE_BASENAME", imageBasename)
		a.Logf("Using local registry: %s", imageBasename)
		return
	}

	// No test registry available - fail to protect production
	a.Fatalf(
		"E2E tests require a test registry. Options:\n"+
			"  1. Set E2E_IMAGE_BASENAME environment variable\n"+
			"  2. Set IMAGE_BASENAME to a non-production registry\n"+
			"  3. Start local registry on localhost:5001\n"+
			"Production registry (%s) cannot be used for e2e tests",
		productionBasename,
	)
}

// isLocalRegistryAvailable checks if local registry is reachable
func isLocalRegistryAvailable(registry string) bool {
	conn, err := net.DialTimeout("tcp", registry, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func kubeAvailable(a *goyek.A) bool {
	a.Helper()

	if err := k8s.CheckConnection(); err != nil {
		a.Log("Kube client failed: ", err)
		return false
	}

	return true
}
