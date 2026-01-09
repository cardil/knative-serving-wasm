package tasks

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/cardil/ghet/pkg/ghet/download"
	"github.com/cardil/ghet/pkg/ghet/install"
	"github.com/cardil/ghet/pkg/github"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
	"go.yaml.in/yaml/v2"
)

const koDockerRepo = "KO_DOCKER_REPO"

// wasmModule represents a WASM module example
type wasmModule struct {
	name     string // module name (e.g., "reverse-text")
	wasmFile string // wasm file name without extension (e.g., "reverse_text")
}

// wasmModules lists all example modules to build and push
var wasmModules = []wasmModule{{
	name: "reverse-text", wasmFile: "reverse_text",
}, {
	name: "http-fetch", wasmFile: "http_fetch",
}}

func Deploy(f *goyek.Flow) {
	f.Define(goyek.Task{
		Name:  "deploy",
		Usage: "Deploys the controller onto Kubernetes",
		Action: func(a *goyek.A) {
			setupKoEnv(a)
			koApply(a)
		},
		Deps: goyek.Deps{
			f.Define(Publish(f)),
		},
	})
	f.Define(Undeploy())
}

// koApply applies Kubernetes manifests using ko
func koApply(a *goyek.A) {
	koApplyWithFlags(a, false)
}

// koApplyE2E applies Kubernetes manifests using ko with e2e-specific settings
func koApplyE2E(a *goyek.A) {
	koApplyWithFlags(a, true)
}

// koApplyWithFlags applies Kubernetes manifests using ko with optional e2e flags
func koApplyWithFlags(a *goyek.A, e2eMode bool) {
	a.Helper()

	runnerImage := path.Join(os.Getenv(koDockerRepo), "runner")

	// Create ko config with ldflags
	// GOFLAGS doesn't support multiple -X flags, so we use .ko.yaml instead
	// See: https://ko.build/advanced/faq/#how-can-i-set-ldflags
	type koBuild struct {
		ID      string   `yaml:"id"`
		Dir     string   `yaml:"dir"`
		Ldflags []string `yaml:"ldflags"`
	}
	type koConfig struct {
		Builds []koBuild `yaml:"builds"`
	}

	ldflags := []string{
		fmt.Sprintf("-X github.com/cardil/knative-serving-wasm/pkg/reconciler/wasmmodule.DefaultRunnerImage=%s", runnerImage),
	}

	// For e2e tests, also set ImagePullPolicy=Always to ensure fresh images
	if e2eMode {
		ldflags = append(ldflags, "-X github.com/cardil/knative-serving-wasm/pkg/reconciler/wasmmodule.DefaultImagePullPolicy=Always")
	}

	config := koConfig{
		Builds: []koBuild{{
			ID:      "controller",
			Dir:     "./cmd/controller",
			Ldflags: ldflags,
		}},
	}

	configYAML, err := yaml.Marshal(config)
	if err != nil {
		a.Fatalf("Failed to marshal ko config: %v", err)
	}

	koConfigPath := path.Join("build", "output", ".ko.yaml")
	if err := os.WriteFile(koConfigPath, configYAML, 0644); err != nil {
		a.Fatalf("Failed to write ko config: %v", err)
	}

	cmd.Exec(a, spaceJoin(
		"go", "run", "github.com/google/ko@latest", "apply",
		"-B", "-f", "config/",
	), cmd.Env("KO_CONFIG_PATH", koConfigPath))
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

func Publish(f *goyek.Flow) goyek.Task {
	return goyek.Task{
		Name:  "publish",
		Usage: "Publish artifacts onto registry",
		Action: func(a *goyek.A) {
			setupKoEnv(a)
			pushExamples(a)
			pushRunnerImage(a)
		},
		Deps: goyek.Deps{
			f.Define(Images()),
		},
	}
}

func Images() goyek.Task {
	return goyek.Task{
		Name:  "images",
		Usage: "Builds OCI images",
		Action: func(a *goyek.A) {
			setupKoEnv(a)
			buildExamples(a)
			buildRunnerImage(a)
		},
	}
}

func pushExamples(a *goyek.A) {
	installWkg(a)
	wkg := wkgPath()
	repo := os.Getenv(koDockerRepo)

	for _, mod := range wasmModules {
		tag := path.Join(repo, "example", mod.name)
		wasm := path.Join("examples", "modules", mod.name,
			"target", "wasm32-wasip2", "release", mod.wasmFile+".wasm")
		cmd.Exec(a, spaceJoin(wkg, "oci", "push", tag, wasm))
	}
}

func pushRunnerImage(a *goyek.A) {
	e := resolveContainerEngine()
	tag := path.Join(os.Getenv(koDockerRepo), "runner")
	cmd.Exec(a, spaceJoin(e, "push", tag))
}

func buildRunnerImage(a *goyek.A) {
	e := resolveContainerEngine()
	tag := path.Join(os.Getenv(koDockerRepo), "runner")
	cmd.Exec(a, spaceJoin(e, "build", ".", "--layers", "-t", tag),
		cmd.Dir("runner"))
}

func resolveContainerEngine() string {
	e := "podman"
	if _, err := exec.LookPath(e); err != nil {
		e = "docker"
	}
	return e
}

func setupKoEnv(a *goyek.A) {
	a.Helper()

	if _, ok := os.LookupEnv(koDockerRepo); !ok {
		a.Setenv(koDockerRepo, os.Getenv("IMAGE_BASENAME"))
	}
}

func installWkg(a *goyek.A) {
	a.Helper()

	plan := wkgPlan()
	bin := plan.Asset.FileName.ToString()
	pth := path.Join(plan.Destination, bin)
	if _, err := os.Stat(pth); err == nil {
		a.Log("Already installed: ", bin)

		return
	}

	if err := download.Action(a.Context(), plan); err != nil {
		a.Fatal(err)
	}
}

func wkgPlan() download.Args {
	binSpec := "bytecodealliance/wasm-pkg-tools@v0.11.0"
	destination := path.Join("build", "output", "tools")
	p := download.Args{
		Args:        install.Parse(binSpec),
		Destination: destination,
	}
	p.FileName = github.NewFileName("wkg")

	return p
}

func wkgPath() string {
	plan := wkgPlan()
	bin := plan.Asset.FileName.ToString()

	return path.Join(plan.Destination, bin)
}

func spaceJoin(parts ...string) string {
	return strings.Join(parts, " ")
}
