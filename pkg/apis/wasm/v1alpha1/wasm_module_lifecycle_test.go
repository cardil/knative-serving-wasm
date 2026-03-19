/*
Copyright 2026 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1_test

import (
	"testing"

	v1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	"knative.dev/pkg/apis"
)

func TestMarkServiceFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reason      string
		message     string
		wantReason  string
		wantMessage string
	}{
		{
			name:        "RevisionFailed propagated",
			reason:      "RevisionFailed",
			message:     "Revision 'foo-00001' failed with message: OCI pull failed.",
			wantReason:  "RevisionFailed",
			wantMessage: "Revision 'foo-00001' failed with message: OCI pull failed.",
		},
		{
			name:        "ContainerMissing propagated",
			reason:      "ContainerMissing",
			message:     "Image 'ghcr.io/bad/image' not found.",
			wantReason:  "ContainerMissing",
			wantMessage: "Image 'ghcr.io/bad/image' not found.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status := &v1alpha1.WasmModuleStatus{}
			status.InitializeConditions()
			status.MarkServiceFailed(tt.reason, tt.message)

			cond := status.GetCondition(apis.ConditionReady)
			if cond == nil {
				t.Fatal("expected Ready condition, got nil")
			}

			if !cond.IsFalse() {
				t.Errorf("expected condition to be False, got %v", cond.Status)
			}

			if cond.Reason != tt.wantReason {
				t.Errorf("reason: got %q, want %q", cond.Reason, tt.wantReason)
			}

			if cond.Message != tt.wantMessage {
				t.Errorf("message: got %q, want %q", cond.Message, tt.wantMessage)
			}
		})
	}
}

func TestMarkServiceFailedDiffersFromUnavailable(t *testing.T) {
	t.Parallel()

	unavailable := &v1alpha1.WasmModuleStatus{}
	unavailable.InitializeConditions()
	unavailable.MarkServiceUnavailable("my-svc")

	failed := &v1alpha1.WasmModuleStatus{}
	failed.InitializeConditions()
	failed.MarkServiceFailed("RevisionFailed", "crashed")

	unavailableCond := unavailable.GetCondition(apis.ConditionReady)
	failedCond := failed.GetCondition(apis.ConditionReady)

	if unavailableCond.Reason != "ServiceUnavailable" {
		t.Errorf("MarkServiceUnavailable reason: got %q, want %q", unavailableCond.Reason, "ServiceUnavailable")
	}

	if failedCond.Reason != "RevisionFailed" {
		t.Errorf("MarkServiceFailed reason: got %q, want %q", failedCond.Reason, "RevisionFailed")
	}
}
