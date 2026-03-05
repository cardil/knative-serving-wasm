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

// Package tools provides helpers for installing pinned tool binaries defined
// in build/tools.yaml using the ghet downloader.
package tools

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/cardil/ghet/pkg/ghet/download"
	"github.com/cardil/ghet/pkg/ghet/install"
	"github.com/cardil/ghet/pkg/github"
	fsutil "github.com/cardil/knative-serving-wasm/build/util/fs"
	"github.com/goyek/goyek/v2"
	"go.yaml.in/yaml/v2"
)

// toolEntry is a single ghet-installable tool from build/tools.yaml.
type toolEntry struct {
	Ref     string `yaml:"ref"`
	Version string `yaml:"version"`
}

// toolsFile is the top-level structure of build/tools.yaml.
type toolsFile struct {
	Ghet map[string]toolEntry `yaml:"ghet"`
}

// loadTools reads and parses build/tools.yaml.
func loadTools() (toolsFile, error) {
	root := fsutil.RootDir()
	data, err := os.ReadFile(path.Join(root, "build", "tools.yaml"))
	if err != nil {
		return toolsFile{}, fmt.Errorf("reading build/tools.yaml: %w", err)
	}

	var tf toolsFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return toolsFile{}, fmt.Errorf("parsing build/tools.yaml: %w", err)
	}

	return tf, nil
}

// Ghet installs the named tool (as defined in build/tools.yaml under the
// ghet: key) if not already present and returns the path to its binary.
// The tool version can be overridden via env var: upper-cased name with
// hyphens replaced by underscores and _VERSION suffix (e.g. KO_VERSION).
func Ghet(a *goyek.A, name string) string {
	a.Helper()

	tf, err := loadTools()
	if err != nil {
		a.Fatal(err)
	}

	entry, ok := tf.Ghet[name]
	if !ok {
		a.Fatalf("tool %q not found in build/tools.yaml", name)
	}

	// Allow env-var override of the version.
	envKey := strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_VERSION"
	if v := os.Getenv(envKey); v != "" {
		entry.Version = v
	}

	plan := makePlan(entry, name)
	bin := plan.Asset.FileName.ToString()
	pth := path.Join(plan.Destination, bin)

	if _, err := os.Stat(pth); err == nil {
		return pth
	}

	if err := download.Action(a.Context(), plan); err != nil {
		a.Fatal(err)
	}

	return pth
}

func makePlan(entry toolEntry, binName string) download.Args {
	binSpec := entry.Ref + "@" + entry.Version
	destination := path.Join("build", "output", "tools", binName, entry.Version)
	p := download.Args{
		Args:        install.Parse(binSpec),
		Destination: destination,
	}
	p.FileName = github.NewFileName(binName)

	return p
}
