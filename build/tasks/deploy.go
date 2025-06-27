package tasks

import (
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/cardil/ghet/pkg/ghet/download"
	"github.com/cardil/ghet/pkg/ghet/install"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

const koDockerRepo = "KO_DOCKER_REPO"

func Deploy(f *goyek.Flow) {
	f.Define(goyek.Task{
		Name:  "deploy",
		Usage: "Deploys the controller onto Kubernetes",
		Action: func(a *goyek.A) {
			setupKoEnv(a)
			cmd.Exec(a,
				"go run github.com/google/ko@latest apply -B -f config/",
			)
		},
		Deps: goyek.Deps{
			f.Define(Publish(f)),
		},
	})
	f.Define(Undeploy())
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
	installWasmToOci(a)
	tag := path.Join(os.Getenv(koDockerRepo), "example", "reverse-text")
	wasm := path.Join("examples", "modules", "reverse-text",
		"target", "wasm32-wasip2", "release", "reverse_text.wasm")
	w2o := wasm2ociPath()
	cmd.Exec(a, spaceJoin(w2o, "push", wasm, tag))
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

func installWasmToOci(a *goyek.A) {
	a.Helper()

	plan := wasm2ociPlan()
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

func wasm2ociPlan() download.Args {
	binSpec := "engineerd/wasm-to-oci"
	destination := path.Join("build", "output", "tools")
	p := download.Args{
		Args:        install.Parse(binSpec),
		Destination: destination,
	}
	p.Tag = "v0.1.2"

	return p
}

func wasm2ociPath() string {
	plan := wasm2ociPlan()
	bin := plan.Asset.FileName.ToString()
	return path.Join(plan.Destination, bin)
}

func spaceJoin(parts ...string) string {
	return strings.Join(parts, " ")
}
