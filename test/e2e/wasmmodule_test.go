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

// TestBasicDeployment tests basic WasmModule deployment with reverse-text
func TestBasicDeployment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	namespace := fmt.Sprintf("e2e-basic-%d", time.Now().Unix())
	tc, err := newTestContext(t, namespace)
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}
	defer tc.Cleanup()

	// Create namespace
	if err := tc.CreateNamespace(ctx); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Create WasmModule
	wasmModule := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "reverse-text",
			Namespace: namespace,
		},
		Spec: wasmv1alpha1.WasmModuleSpec{
			Image: tc.Config.ExampleImage("reverse-text"),
		},
	}

	_, err = tc.CreateWasmModule(ctx, wasmModule)
	if err != nil {
		t.Fatalf("Failed to create WasmModule: %v", err)
	}

	// Wait for WasmModule to be ready
	if err := tc.WaitForWasmModuleReady(ctx, "reverse-text"); err != nil {
		t.Fatalf("WasmModule did not become ready: %v", err)
	}

	// Get WasmModule URL
	url, err := tc.GetWasmModuleURL(ctx, "reverse-text")
	if err != nil {
		t.Fatalf("Failed to get WasmModule URL: %v", err)
	}

	t.Logf("WasmModule URL: %s", url)

	// Test HTTP request
	testText := "hello"
	requestURL := fmt.Sprintf("%s?text=%s", url, testText)

	response, err := tc.HTTPGet(ctx, requestURL)
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}

	// Verify response is reversed
	expected := reverseString(testText)
	if !strings.Contains(response, expected) {
		t.Errorf("Expected response to contain %q, got: %s", expected, response)
	}

	t.Logf("Successfully verified reverse-text response: %s", response)
}

// reverseString reverses a string
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
