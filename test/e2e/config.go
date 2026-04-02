// Copyright 2026 The Knative Authors
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
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	// DevImageBasenameEnv is the environment variable for specifying dev/test image basename.
	DevImageBasenameEnv = "DEV_IMAGE_BASENAME"

	// E2ETestTimeoutEnv is the environment variable for individual test timeout in seconds.
	E2ETestTimeoutEnv = "E2E_TEST_TIMEOUT"

	// DefaultTestTimeout is the default timeout for individual e2e tests.
	DefaultTestTimeout = 1 * time.Minute
)

// Config holds e2e test configuration.
type Config struct {
	// Namespace is the Kubernetes namespace for tests
	Namespace string

	// ImageBasename is the base path for images
	ImageBasename string
}

func GetE2EImageBasename() (string, error) {
	if v := os.Getenv(DevImageBasenameEnv); v != "" {
		return v, nil
	}

	for _, f := range []string{"user.env", ".env"} {
		if envMap, err := godotenv.Read(f); err == nil {
			if v := envMap[DevImageBasenameEnv]; v != "" {
				return v, nil
			}
		}
	}

	return "", fmt.Errorf("%s is not set (check .env or environment)", DevImageBasenameEnv)
}

// GetTestTimeout returns the individual test timeout from environment or default.
func GetTestTimeout() time.Duration {
	if timeoutStr := os.Getenv(E2ETestTimeoutEnv); timeoutStr != "" {
		if seconds, err := strconv.Atoi(timeoutStr); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}

	return DefaultTestTimeout
}

// NewConfig creates a new e2e test configuration.
func NewConfig(_ context.Context, namespace string) (*Config, error) {
	imageBasename, err := GetE2EImageBasename()
	if err != nil {
		return nil, err
	}

	return &Config{
		Namespace:     namespace,
		ImageBasename: imageBasename,
	}, nil
}

// RunnerImage returns the full runner image reference.
func (c *Config) RunnerImage() string {
	return c.ImageBasename + "/runner"
}

// ExampleImage returns the full example module image reference.
func (c *Config) ExampleImage(name string) string {
	return c.ImageBasename + "/example/" + name
}
