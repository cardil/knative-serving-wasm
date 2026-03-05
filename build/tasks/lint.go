package tasks

import (
	"os"
	"path"

	"github.com/cardil/ghet/pkg/ghet/download"
	"github.com/cardil/ghet/pkg/ghet/install"
	"github.com/cardil/ghet/pkg/github"
	executil "github.com/cardil/knative-serving-wasm/build/util/exec"
	"github.com/goyek/goyek/v2"
)

func Lint() goyek.Task {
	return goyek.Task{
		Name:  "lint",
		Usage: "Runs linters on the project",
		Action: func(a *goyek.A) {
			installGolangciLint(a)
			linter := golangciLintPath()
			executil.ExecOrDie(a, spaceJoin(linter, "run", "./..."))
		},
	}
}

func AutoFix() goyek.Task {
	return goyek.Task{
		Name:  "autofix",
		Usage: "Runs linters on the project and fixes what it can",
		Action: func(a *goyek.A) {
			installGolangciLint(a)
			linter := golangciLintPath()
			executil.ExecOrDie(a, spaceJoin(linter, "run", "--fix", "./..."))
		},
	}
}

func installGolangciLint(a *goyek.A) {
	a.Helper()

	plan := golangciLintPlan()
	bin := plan.Asset.FileName.ToString()
	pth := path.Join(plan.Destination, bin)
	if _, err := os.Stat(pth); err == nil {
		return
	}

	if err := download.Action(a.Context(), plan); err != nil {
		a.Fatal(err)
	}
}

func golangciLintPlan() download.Args {
	binSpec := "golangci/golangci-lint@v2.5.0"
	destination := path.Join("build", "output", "tools")
	p := download.Args{
		Args:        install.Parse(binSpec),
		Destination: destination,
	}
	p.FileName = github.NewFileName("golangci-lint")

	return p
}

func golangciLintPath() string {
	plan := golangciLintPlan()
	bin := plan.Asset.FileName.ToString()

	return path.Join(plan.Destination, bin)
}
