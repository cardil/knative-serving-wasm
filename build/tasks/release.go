// Copyright 2026 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tasks

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	executil "github.com/cardil/knative-serving-wasm/build/util/exec"
	"github.com/cardil/knative-serving-wasm/build/util/tools"
	"github.com/goyek/goyek/v2"
	"go.yaml.in/yaml/v2"
)

const (
	releaseOutputDir  = "build/output/release"
	ociControllerDir  = "build/output/release/oci-controller"
	releaseYAML       = "build/output/release/serving-wasm.yaml"
	checksumsFile     = "build/output/release/checksums.txt"
	artifactListFile  = "build/output/release/artifacts-to-publish.list"
	versionFile       = "build/output/release/version.txt"
	releaseLabelDevel = "wasm.serving.knative.dev/release: devel"
)

// Release registers the release, release-build, release-perform, and release-clean tasks.
func Release(f *goyek.Flow) {
	clean := f.Define(ReleaseClean())
	build := f.Define(ReleaseBuild())
	perform := f.Define(ReleasePerform())
	f.Define(goyek.Task{
		Name:  "release",
		Usage: "Builds and publishes a tagged release",
		Deps:  goyek.Deps{build, perform},
	})
	_ = clean
}

// ReleaseClean removes all release build artifacts.
func ReleaseClean() goyek.Task {
	return goyek.Task{
		Name:  "release-clean",
		Usage: "Removes release build artifacts from build/output/release/",
		Action: func(a *goyek.A) {
			if err := os.RemoveAll(releaseOutputDir); err != nil {
				a.Errorf("Failed to remove release output dir: %v", err)
			} else {
				a.Log("Removed: ", releaseOutputDir)
			}
		},
	}
}

// ReleaseBuild builds all release artifacts locally — no registry push.
// It saves the resolved version to build/output/release/version.txt.
func ReleaseBuild() goyek.Task {
	return goyek.Task{
		Name:  "release-build",
		Usage: "Builds release artifacts locally (no registry push), saves version to build/output/release/version.txt",
		Action: func(a *goyek.A) {
			setupKoEnv(a)

			version := detectReleaseVersion(a)
			a.Log("Release version: ", version)

			repo := os.Getenv(koDockerRepo)

			if err := os.MkdirAll(releaseOutputDir, 0755); err != nil {
				a.Fatalf("Failed to create release output dir: %v", err)
			}
			if err := os.MkdirAll(ociControllerDir, 0755); err != nil {
				a.Fatalf("Failed to create OCI controller dir: %v", err)
			}

			// Save version for release:perform to consume
			if err := os.WriteFile(versionFile, []byte(version+"\n"), 0644); err != nil {
				a.Fatalf("Failed to write version file: %v", err)
			}
			a.Log("Version saved to: ", versionFile)

			runnerImage := path.Join(repo, "runner") + ":v" + version

			// Build multi-arch runner image locally (no push)
			buildRunnerImageMultiArch(a, runnerImage)

			// Build controller locally via ko resolve --push=false --oci-layout-path
			rawYAML := buildControllerLocal(a, repo, runnerImage, version)

			// Replace OCI local path with real registry ref + update release label
			finalYAML := replaceImageRefs(rawYAML, ociControllerDir, repo, version)

			if err := os.WriteFile(releaseYAML, []byte(finalYAML), 0644); err != nil {
				a.Fatalf("Failed to write release YAML: %v", err)
			}
			a.Log("Release YAML written to: ", releaseYAML)

			// Generate checksums
			generateChecksums(a)

			// Write artifacts list
			artifacts := releaseYAML + "\n" + checksumsFile + "\n"
			if err := os.WriteFile(artifactListFile, []byte(artifacts), 0644); err != nil {
				a.Fatalf("Failed to write artifact list: %v", err)
			}
			a.Log("Artifact list written to: ", artifactListFile)
		},
	}
}

// ReleasePerform pushes images and creates the GitHub release.
// It reads the version from build/output/release/version.txt (written by release-build).
// On success, it removes the release output directory.
func ReleasePerform() goyek.Task {
	return goyek.Task{
		Name:  "release-perform",
		Usage: "Pushes images and publishes GitHub release (reads version from build/output/release/version.txt)",
		Action: func(a *goyek.A) {
			setupKoEnv(a)

			// Read version written by release:build
			versionBytes, err := os.ReadFile(versionFile)
			if err != nil {
				a.Fatalf("Failed to read version file %q (run release:build first): %v", versionFile, err)
			}
			version := strings.TrimSpace(string(versionBytes))
			a.Log("Release version (from file): ", version)

			repo := os.Getenv(koDockerRepo)
			runnerImage := path.Join(repo, "runner")
			versionedRunner := runnerImage + ":v" + version

			// Validate artifact list before any remote mutations.
			// This fails fast if release:build was not run first.
			artifactBytes, err := os.ReadFile(artifactListFile)
			if err != nil {
				a.Fatalf("Failed to read artifact list (run release:build first): %v", err)
			}
			var artifacts []string
			for _, line := range strings.Split(strings.TrimSpace(string(artifactBytes)), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					artifacts = append(artifacts, line)
				}
			}

			// Push runner image from local buildah storage
			pushRunnerImageMultiArch(a, versionedRunner, runnerImage+":latest")

			// Push controller image from OCI layout via skopeo
			controllerImage := path.Join(repo, "controller")
			pushControllerImage(a, controllerImage, version)

			// Derive release branch from version (e.g. 0.1.2 -> release-0.1)
			tag := "v" + version
			parts := strings.SplitN(version, ".", 3)
			releaseBranch := "release-" + parts[0] + "." + parts[1]

			// Create GitHub release
			ghArgs := []string{
				"gh", "release", "create", tag,
				"--title", tag,
				"--generate-notes",
				"--target", releaseBranch,
			}
			ghArgs = append(ghArgs, artifacts...)
			executil.ExecOrDie(a, strings.Join(ghArgs, " "))

			// Clean up release output on success
			a.Log("Cleaning up release artifacts...")
			if err := os.RemoveAll(releaseOutputDir); err != nil {
				// Non-fatal — release was published successfully
				a.Logf("Warning: failed to clean release output dir: %v", err)
			}
		},
	}
}

// detectReleaseVersion resolves the release version (without "v" prefix) from
// environment variables or git tags. Used only by release:build.
func detectReleaseVersion(a *goyek.A) string {
	a.Helper()

	normalize := func(v string) string {
		v = strings.TrimPrefix(v, "v")
		parts := strings.SplitN(v, ".", 3)
		if len(parts) < 2 {
			a.Fatalf("Invalid release version %q: expected vX.Y or vX.Y.Z", v)
		}
		return v
	}

	// 1. RELEASE_VERSION env var (e.g. set by hack/release.sh from Prow TAG)
	if v := os.Getenv("RELEASE_VERSION"); v != "" {
		return normalize(v)
	}

	// 2. GITHUB_REF_NAME (GH Actions tag push, e.g. "v0.1.0") — prefer the
	//    exact tag that triggered the workflow over a local git lookup.
	if v := os.Getenv("GITHUB_REF_NAME"); strings.HasPrefix(v, "v") {
		return normalize(v)
	}

	// 3. Git tag on HEAD matching v* (local / non-GH-Actions invocations)
	if v := gitTagOnHead(); v != "" {
		return normalize(v)
	}

	a.Fatal("Cannot determine release version: set RELEASE_VERSION env var, " +
		"ensure HEAD has a v* git tag, or run from a GH Actions tag push (GITHUB_REF_NAME=v...)")
	return ""
}

// gitTagOnHead returns the first v* tag pointing at HEAD, or empty string.
func gitTagOnHead() string {
	out, err := exec.Command("git", "tag", "--points-at", "HEAD", "--list", "v*").Output()
	if err != nil {
		return ""
	}
	tags := strings.Fields(strings.TrimSpace(string(out)))
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}

// manageBranchAndTag creates the release branch and tag if they don't exist.
func manageBranchAndTag(a *goyek.A, version string) {
	a.Helper()

	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		a.Fatalf("Invalid version format (expected X.Y.Z): %s", version)
	}
	releaseBranch := "release-" + parts[0] + "." + parts[1]
	tag := "v" + version

	// Check if release branch exists on remote
	checkBranch := exec.Command("git", "ls-remote", "--exit-code", "--heads", "origin", releaseBranch)
	if err := checkBranch.Run(); err != nil {
		a.Log("Creating release branch: ", releaseBranch)
		executil.ExecOrDie(a, spaceJoin("git", "checkout", "-b", releaseBranch))
		executil.ExecOrDie(a, spaceJoin("git", "push", "origin", releaseBranch))
	} else {
		a.Log("Release branch already exists: ", releaseBranch)
	}

	// Check if tag exists on remote
	checkTag := exec.Command("git", "ls-remote", "--exit-code", "--tags", "origin", tag)
	if err := checkTag.Run(); err != nil {
		a.Log("Creating tag: ", tag)
		executil.ExecOrDie(a, spaceJoin("git", "tag", tag))
		executil.ExecOrDie(a, spaceJoin("git", "push", "origin", tag))
	} else {
		a.Log("Tag already exists: ", tag)
	}
}

// buildRunnerImageMultiArch builds the runner image for linux/amd64,linux/arm64
// into local buildah manifest storage (no push).
func buildRunnerImageMultiArch(a *goyek.A, manifestName string) {
	a.Helper()
	a.Log("Building multi-arch runner image locally: ", manifestName)
	executil.ExecOrDie(a, spaceJoin(
		"buildah", "build",
		"--platform", "linux/amd64,linux/arm64",
		"--manifest", manifestName,
		"--layers",
		"runner/",
	))
}

// pushRunnerImageMultiArch pushes the locally-stored runner manifest to the registry.
func pushRunnerImageMultiArch(a *goyek.A, versionedTag, latestTag string) {
	a.Helper()
	a.Log("Pushing runner image: ", versionedTag)
	executil.ExecOrDie(a, spaceJoin(
		"buildah", "manifest", "push", "--all",
		versionedTag, "docker://"+versionedTag,
	))
	a.Log("Pushing runner image: ", latestTag)
	executil.ExecOrDie(a, spaceJoin(
		"buildah", "manifest", "push", "--all",
		versionedTag, "docker://"+latestTag,
	))
}

// buildControllerLocal runs ko resolve --push=false --oci-layout-path and returns
// the raw YAML content (with OCI local path refs).
func buildControllerLocal(a *goyek.A, repo, runnerImage, version string) string {
	a.Helper()

	// Write .ko.yaml with ldflags pointing to the tagged runner image
	type koBuild struct {
		ID      string   `yaml:"id"`
		Dir     string   `yaml:"dir"`
		Ldflags []string `yaml:"ldflags"`
	}
	type koConfig struct {
		Builds []koBuild `yaml:"builds"`
	}
	config := koConfig{
		Builds: []koBuild{{
			ID:  "controller",
			Dir: "./cmd/controller",
			Ldflags: []string{
				fmt.Sprintf("-X github.com/cardil/knative-serving-wasm/pkg/reconciler/wasmmodule.DefaultRunnerImage=%s", runnerImage),
			},
		}},
	}
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		a.Fatalf("Failed to marshal ko config: %v", err)
	}
	koConfigPath := path.Join(releaseOutputDir, ".ko.yaml")
	if err := os.WriteFile(koConfigPath, configYAML, 0644); err != nil {
		a.Fatalf("Failed to write ko config: %v", err)
	}

	ko := tools.Ghet(a, "ko")
	tmpYAML := path.Join(releaseOutputDir, "serving-wasm-raw.yaml")

	// Run ko resolve --push=false --oci-layout-path, capture stdout into file.
	// We use os/exec directly so we can redirect stdout to the file while still
	// streaming stderr (ko logs) to the task output.
	//nolint:gosec
	koCmd := exec.Command(ko, "resolve", //nolint:govet
		"-B",
		"--push=false",
		"--oci-layout-path", ociControllerDir,
		"--platform", "linux/amd64,linux/arm64",
		"--tags", "v"+version+",latest",
		"-f", "config/",
	)
	koCmd.Env = append(os.Environ(),
		"KO_DOCKER_REPO="+repo,
		"KO_CONFIG_PATH="+koConfigPath,
	)
	koCmd.Stderr = os.Stderr // stream ko logs to terminal

	outFile, err := os.Create(tmpYAML)
	if err != nil {
		a.Fatalf("Failed to create raw YAML file: %v", err)
	}
	koCmd.Stdout = outFile
	if err := koCmd.Run(); err != nil {
		_ = outFile.Close()
		a.Fatalf("ko resolve failed: %v", err)
	}
	_ = outFile.Close()

	content, err := os.ReadFile(tmpYAML)
	if err != nil {
		a.Fatalf("Failed to read raw YAML: %v", err)
	}
	return string(content)
}

// replaceImageRefs replaces the OCI local path prefix in the YAML with the real
// registry controller image ref (preserving the sha256 digest), and also
// replaces the devel release label with the versioned label.
func replaceImageRefs(rawYAML, ociLayoutPath, repo, version string) string {
	controllerRef := path.Join(repo, "controller")

	// Replace OCI layout path ref with real registry ref, preserving digest
	// e.g. "build/output/release/oci-controller@sha256:abc" -> "ghcr.io/.../controller@sha256:abc"
	result := strings.ReplaceAll(rawYAML, ociLayoutPath+"@sha256:", controllerRef+"@sha256:")

	// Replace devel release label with versioned label
	versionLabel := "wasm.serving.knative.dev/release: \"v" + version + "\""
	result = strings.ReplaceAll(result, releaseLabelDevel, versionLabel)

	return result
}

// generateChecksums computes sha256 of the release YAML and writes checksums.txt.
func generateChecksums(a *goyek.A) {
	a.Helper()

	f, err := os.Open(releaseYAML)
	if err != nil {
		a.Fatalf("Failed to open release YAML for checksumming: %v", err)
	}
	defer f.Close() //nolint:errcheck

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		a.Fatalf("Failed to compute sha256: %v", err)
	}

	// Format: "<hex>  <filename>" (sha256sum compatible)
	yamlBase := path.Base(releaseYAML)
	checksumLine := fmt.Sprintf("%x  %s\n", h.Sum(nil), yamlBase)

	if err := os.WriteFile(checksumsFile, []byte(checksumLine), 0644); err != nil {
		a.Fatalf("Failed to write checksums file: %v", err)
	}
	a.Log("Checksums written to: ", checksumsFile)
}

// pushControllerImage pushes the controller OCI layout to the registry via skopeo.
func pushControllerImage(a *goyek.A, controllerRef, version string) {
	a.Helper()

	versionedRef := controllerRef + ":v" + version
	latestRef := controllerRef + ":latest"

	a.Log("Pushing controller image: ", versionedRef)
	executil.ExecOrDie(a, spaceJoin(
		"skopeo", "copy", "--all",
		"oci:"+ociControllerDir,
		"docker://"+versionedRef,
	))

	a.Log("Pushing controller image: ", latestRef)
	executil.ExecOrDie(a, spaceJoin(
		"skopeo", "copy", "--all",
		"oci:"+ociControllerDir,
		"docker://"+latestRef,
	))
}
