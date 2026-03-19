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
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
)

// TestBasicDeployment tests basic WasmModule deployment with reverse-text.
func TestBasicDeployment(t *testing.T) {
	ctx := t.Context()
	namespace := fmt.Sprintf("e2e-basic-%d", time.Now().Unix())

	tc, err := newTestContext(ctx, t, namespace)
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

// TestTerminalFailurePropagation verifies that when the underlying Knative Service
// fails permanently (e.g. RevisionFailed due to a bad image), the WasmModule status
// reflects a terminal failure reason instead of the generic ServiceUnavailable.
// This ensures clients (e.g. func deploy) can distinguish transient from permanent failures.
func TestTerminalFailurePropagation(t *testing.T) {
	ctx := t.Context()
	namespace := fmt.Sprintf("e2e-terminal-%d", time.Now().Unix())

	tc, err := newTestContext(ctx, t, namespace)
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}
	defer tc.Cleanup()

	if err := tc.CreateNamespace(ctx); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Speed up Knative's progress deadline so failures are detected quickly.
	// Restore the original value after the test.
	prev, err := tc.SetProgressDeadline(ctx, "5s")
	if err != nil {
		t.Fatalf("Failed to set progress deadline: %v", err)
	}

	defer func() {
		if prev == "" {
			prev = "600s"
		}

		if _, restoreErr := tc.SetProgressDeadline(context.Background(), prev); restoreErr != nil {
			t.Logf("Warning: failed to restore progress deadline: %v", restoreErr)
		}
	}()

	// Use a deliberately invalid runner image to trigger a terminal revision failure.
	// The runner is the container image (not the WASM image), so using a non-existent
	// runner image causes Knative to set ConfigurationsReady=False/RevisionFailed.
	wasmModule := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-image",
			Namespace: namespace,
		},
		Spec: wasmv1alpha1.WasmModuleSpec{
			Image: "example.com/this-image-does-not-exist:latest",
		},
	}

	if _, err = tc.CreateWasmModule(ctx, wasmModule); err != nil {
		t.Fatalf("Failed to create WasmModule: %v", err)
	}

	// Wait for WasmModule to reach a terminal failure (any reason other than ServiceUnavailable).
	// Knative sets ConfigurationsReady=False with reason=RevisionFailed or ProgressDeadlineExceeded
	// once it determines no revision can become ready.
	gotReason, err := tc.WaitForWasmModuleTerminalFailure(ctx, "bad-image")
	if err != nil {
		t.Fatalf("WasmModule did not reach terminal failure within timeout: %v", err)
	}

	// Verify the condition is set to terminal failure (not ServiceUnavailable)
	wm, err := tc.WasmClient.WasmV1alpha1().WasmModules(
		namespace,
	).Get(ctx, "bad-image", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get WasmModule: %v", err)
	}

	cond := wm.Status.GetCondition(apis.ConditionReady)
	if cond == nil {
		t.Fatal("WasmModule has no Ready condition")
	}

	if cond.Reason == "ServiceUnavailable" {
		t.Errorf("Ready condition reason is ServiceUnavailable (transient); want a terminal reason (e.g. RevisionFailed, ProgressDeadlineExceeded)")
	}

	t.Logf("Terminal failure correctly propagated: Ready=False, Reason=%s (gotReason=%s), Message=%s",
		cond.Reason, gotReason, cond.Message)
}

// reverseString reverses a string.
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}
