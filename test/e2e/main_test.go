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
	"os"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	wasmclientset "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned"
)

var (
	// globalKubeClient is the Kubernetes client for all tests
	globalKubeClient kubernetes.Interface

	// globalWasmClient is the WasmModule client for all tests
	globalWasmClient wasmclientset.Interface
)

// TestMain is the entry point for e2e tests
func TestMain(m *testing.M) {
	// Verify image basename configuration
	imageBasename, err := GetE2EImageBasename()
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E image basename check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Using e2e image basename: %s\n", imageBasename)

	// Initialize Kubernetes clients
	if err := initClients(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize clients: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	os.Exit(code)
}

// initClients initializes Kubernetes and WasmModule clients
func initClients() error {
	// Load kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create Kubernetes client
	globalKubeClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create WasmModule client
	globalWasmClient, err = wasmclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create wasm client: %w", err)
	}

	// Verify cluster connectivity
	_, err = globalKubeClient.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to connect to kubernetes cluster: %w", err)
	}

	fmt.Println("Successfully connected to Kubernetes cluster")
	return nil
}

// newTestContext creates a new test context for a test
func newTestContext(t *testing.T, namespace string) (*TestContext, error) {
	config, err := NewConfig(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
	}

	tc := &TestContext{
		T:          t,
		Namespace:  namespace,
		Config:     config,
		KubeClient: globalKubeClient,
		WasmClient: globalWasmClient,
	}

	return tc, nil
}
