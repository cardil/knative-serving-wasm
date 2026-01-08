// Copyright 2024 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build e2e

package e2e

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/joho/godotenv"
)

const (
	// DefaultLocalRegistry is the default local registry for e2e tests
	DefaultLocalRegistry = "localhost:5001"

	// E2EImageBasenameEnv is the environment variable for specifying test image basename
	E2EImageBasenameEnv = "E2E_IMAGE_BASENAME"

	// ImageBasenameEnv is the environment variable for image basename
	ImageBasenameEnv = "IMAGE_BASENAME"
)

// Config holds e2e test configuration
type Config struct {
	// Namespace is the Kubernetes namespace for tests
	Namespace string

	// ImageBasename is the base path for images
	ImageBasename string
}

// GetE2EImageBasename returns the image basename to use for e2e tests with safety checks
func GetE2EImageBasename() (string, error) {
	// Load .env file to get production IMAGE_BASENAME
	productionBasename := ""
	if envMap, err := godotenv.Read(".env"); err == nil {
		productionBasename = envMap[ImageBasenameEnv]
	}

	// Check for explicit E2E_IMAGE_BASENAME
	if e2eBasename := os.Getenv(E2EImageBasenameEnv); e2eBasename != "" {
		return e2eBasename, nil
	}

	// Check if IMAGE_BASENAME has been overridden from production value
	currentBasename := os.Getenv(ImageBasenameEnv)
	if currentBasename != "" && currentBasename != productionBasename {
		// IMAGE_BASENAME is set and differs from production, use it
		return currentBasename, nil
	}

	// Try to detect local registry and construct basename
	if isLocalRegistryAvailable() {
		return DefaultLocalRegistry + "/knative-serving-wasm", nil
	}

	return "", fmt.Errorf(
		"e2e tests require %s environment variable or local registry on %s; "+
			"alternatively, set %s to a non-production value; "+
			"production value is %q",
		E2EImageBasenameEnv, DefaultLocalRegistry, ImageBasenameEnv, productionBasename,
	)
}

// isLocalRegistryAvailable checks if local registry is reachable
func isLocalRegistryAvailable() bool {
	conn, err := net.DialTimeout("tcp", DefaultLocalRegistry, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

// NewConfig creates a new e2e test configuration
func NewConfig(namespace string) (*Config, error) {
	imageBasename, err := GetE2EImageBasename()
	if err != nil {
		return nil, err
	}

	return &Config{
		Namespace:     namespace,
		ImageBasename: imageBasename,
	}, nil
}

// RunnerImage returns the full runner image reference
func (c *Config) RunnerImage() string {
	return c.ImageBasename + "/runner"
}

// ExampleImage returns the full example module image reference
func (c *Config) ExampleImage(name string) string {
	return c.ImageBasename + "/example/" + name
}
