// Copyright 2025 The Knative Authors
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
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
)

// TestNetworkInherit tests WasmModule with network.inherit=true
func TestNetworkInherit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), GetTestTimeout())
	defer cancel()

	namespace := fmt.Sprintf("e2e-net-inherit-%d", time.Now().Unix())
	tc, err := newTestContext(t, namespace)
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}
	defer tc.Cleanup()

	// Create namespace
	if err := tc.CreateNamespace(ctx); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Deploy echo server
	if err := tc.DeployEchoServer(ctx); err != nil {
		t.Fatalf("Failed to deploy echo server: %v", err)
	}

	// Create WasmModule with network.inherit=true
	wasmModule := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-fetch-inherit",
			Namespace: namespace,
		},
		Spec: wasmv1alpha1.WasmModuleSpec{
			Image: tc.Config.ExampleImage("http-fetch"),
			Network: &wasmv1alpha1.NetworkSpec{
				Inherit: true,
			},
		},
	}

	_, err = tc.CreateWasmModule(ctx, wasmModule)
	if err != nil {
		t.Fatalf("Failed to create WasmModule: %v", err)
	}

	// Wait for WasmModule to be ready
	if err := tc.WaitForWasmModuleReady(ctx, "http-fetch-inherit"); err != nil {
		t.Fatalf("WasmModule did not become ready: %v", err)
	}

	// Get WasmModule URL
	url, err := tc.GetWasmModuleURL(ctx, "http-fetch-inherit")
	if err != nil {
		t.Fatalf("Failed to get WasmModule URL: %v", err)
	}

	// Test HTTP request to echo server
	// Use full FQDN for DNS resolution (required on GKE, works on all clusters)
	// Explicitly specify port 80 for HTTP
	targetURL := fmt.Sprintf("http://echo-server.%s.svc.cluster.local:80", namespace)
	requestURL := fmt.Sprintf("%s?url=%s", url, targetURL)

	response, err := tc.HTTPGet(ctx, requestURL)
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}

	// Verify response contains echo server response
	if !strings.Contains(response, "echo-response") {
		t.Errorf("Expected response to contain 'echo-response', got: %s", response)
	}

	t.Logf("Successfully verified network.inherit allows outbound connection: %s", response)
}

// TestNetworkTcpConnectSpecific tests WasmModule with specific tcp.connect permissions
func TestNetworkTcpConnectSpecific(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), GetTestTimeout())
	defer cancel()

	namespace := fmt.Sprintf("e2e-net-tcp-%d", time.Now().Unix())
	tc, err := newTestContext(t, namespace)
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}
	defer tc.Cleanup()

	// Create namespace
	if err := tc.CreateNamespace(ctx); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Deploy echo server
	if err := tc.DeployEchoServer(ctx); err != nil {
		t.Fatalf("Failed to deploy echo server: %v", err)
	}

	// Create WasmModule with specific tcp.connect permission
	// Use full FQDN for DNS resolution (required on GKE, works on all clusters)
	echoServerHost := fmt.Sprintf("echo-server.%s.svc.cluster.local:80", namespace)
	wasmModule := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-fetch-tcp",
			Namespace: namespace,
		},
		Spec: wasmv1alpha1.WasmModuleSpec{
			Image: tc.Config.ExampleImage("http-fetch"),
			Network: &wasmv1alpha1.NetworkSpec{
				Tcp: &wasmv1alpha1.TcpSpec{
					Connect: []string{echoServerHost},
				},
			},
		},
	}

	_, err = tc.CreateWasmModule(ctx, wasmModule)
	if err != nil {
		t.Fatalf("Failed to create WasmModule: %v", err)
	}

	// Wait for WasmModule to be ready
	if err := tc.WaitForWasmModuleReady(ctx, "http-fetch-tcp"); err != nil {
		t.Fatalf("WasmModule did not become ready: %v", err)
	}

	// Get WasmModule URL
	url, err := tc.GetWasmModuleURL(ctx, "http-fetch-tcp")
	if err != nil {
		t.Fatalf("Failed to get WasmModule URL: %v", err)
	}

	// Test HTTP request to echo server (should succeed)
	targetURL := fmt.Sprintf("http://%s", echoServerHost)
	requestURL := fmt.Sprintf("%s?url=%s", url, targetURL)

	response, err := tc.HTTPGet(ctx, requestURL)
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}

	// Verify response contains echo server response
	if !strings.Contains(response, "echo-response") {
		t.Errorf("Expected response to contain 'echo-response', got: %s", response)
	}

	t.Logf("Successfully verified tcp.connect allows connection to specified host: %s", response)
}

// TestNetworkTcpConnectWildcard tests WasmModule with wildcard tcp.connect permissions
func TestNetworkTcpConnectWildcard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), GetTestTimeout())
	defer cancel()

	namespace := fmt.Sprintf("e2e-net-wildcard-%d", time.Now().Unix())
	tc, err := newTestContext(t, namespace)
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}
	defer tc.Cleanup()

	// Create namespace
	if err := tc.CreateNamespace(ctx); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Deploy echo server
	if err := tc.DeployEchoServer(ctx); err != nil {
		t.Fatalf("Failed to deploy echo server: %v", err)
	}

	// Create WasmModule with wildcard tcp.connect permission
	wasmModule := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-fetch-wildcard",
			Namespace: namespace,
		},
		Spec: wasmv1alpha1.WasmModuleSpec{
			Image: tc.Config.ExampleImage("http-fetch"),
			Network: &wasmv1alpha1.NetworkSpec{
				Tcp: &wasmv1alpha1.TcpSpec{
					Connect: []string{"*:80"},
				},
			},
		},
	}

	_, err = tc.CreateWasmModule(ctx, wasmModule)
	if err != nil {
		t.Fatalf("Failed to create WasmModule: %v", err)
	}

	// Wait for WasmModule to be ready
	if err := tc.WaitForWasmModuleReady(ctx, "http-fetch-wildcard"); err != nil {
		t.Fatalf("WasmModule did not become ready: %v", err)
	}

	// Get WasmModule URL
	url, err := tc.GetWasmModuleURL(ctx, "http-fetch-wildcard")
	if err != nil {
		t.Fatalf("Failed to get WasmModule URL: %v", err)
	}

	// Test HTTP request to echo server
	// Use full FQDN for DNS resolution (required on GKE, works on all clusters)
	// Explicitly specify port 80 for HTTP
	targetURL := fmt.Sprintf("http://echo-server.%s.svc.cluster.local:80", namespace)
	requestURL := fmt.Sprintf("%s?url=%s", url, targetURL)

	response, err := tc.HTTPGet(ctx, requestURL)
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}

	// Verify response contains echo server response
	if !strings.Contains(response, "echo-response") {
		t.Errorf("Expected response to contain 'echo-response', got: %s", response)
	}

	t.Logf("Successfully verified wildcard tcp.connect allows connection: %s", response)
}

// TestNetworkNoPermission tests WasmModule without network permissions (should fail)
func TestNetworkNoPermission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), GetTestTimeout())
	defer cancel()

	namespace := fmt.Sprintf("e2e-net-none-%d", time.Now().Unix())
	tc, err := newTestContext(t, namespace)
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}
	defer tc.Cleanup()

	// Create namespace
	if err := tc.CreateNamespace(ctx); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Deploy echo server
	if err := tc.DeployEchoServer(ctx); err != nil {
		t.Fatalf("Failed to deploy echo server: %v", err)
	}

	// Create WasmModule without network permissions
	wasmModule := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-fetch-no-net",
			Namespace: namespace,
		},
		Spec: wasmv1alpha1.WasmModuleSpec{
			Image: tc.Config.ExampleImage("http-fetch"),
			// No network spec - network access disabled
		},
	}

	_, err = tc.CreateWasmModule(ctx, wasmModule)
	if err != nil {
		t.Fatalf("Failed to create WasmModule: %v", err)
	}

	// Wait for WasmModule to be ready
	if err := tc.WaitForWasmModuleReady(ctx, "http-fetch-no-net"); err != nil {
		t.Fatalf("WasmModule did not become ready: %v", err)
	}

	// Get WasmModule URL
	url, err := tc.GetWasmModuleURL(ctx, "http-fetch-no-net")
	if err != nil {
		t.Fatalf("Failed to get WasmModule URL: %v", err)
	}

	// Test HTTP request to echo server (should fail or return error)
	// Use full FQDN for DNS resolution (required on GKE, works on all clusters)
	// Explicitly specify port 80 for HTTP
	targetURL := fmt.Sprintf("http://echo-server.%s.svc.cluster.local:80", namespace)
	requestURL := fmt.Sprintf("%s?url=%s", url, targetURL)

	response, err := tc.HTTPGet(ctx, requestURL)
	// We expect either an HTTP error or an error response in the body
	if err == nil && !strings.Contains(response, "error") && !strings.Contains(response, "denied") {
		t.Errorf("Expected network request to fail without permissions, but got success: %s", response)
	}

	t.Log("Successfully verified network request fails without permissions")
}
