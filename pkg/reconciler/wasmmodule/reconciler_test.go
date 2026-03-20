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

package wasmmodule_test

import (
	"context"
	"fmt"
	"testing"

	api "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	"github.com/cardil/knative-serving-wasm/pkg/reconciler/wasmmodule"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/tracker"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingv1listers "knative.dev/serving/pkg/client/listers/serving/v1"
)

// fakeTracker is a no-op tracker for unit tests.
type fakeTracker struct{}

func (fakeTracker) Track(_ corev1.ObjectReference, _ interface{}) error     { return nil }
func (fakeTracker) TrackReference(_ tracker.Reference, _ interface{}) error { return nil }
func (fakeTracker) OnChanged(_ interface{})                                 {}
func (fakeTracker) GetObservers(_ interface{}) []types.NamespacedName       { return nil }
func (fakeTracker) OnDeletedObserver(_ interface{})                         {}

// buildServiceLister creates a ServiceLister seeded with the given services.
func buildServiceLister(svcs ...*servingv1.Service) servingv1listers.ServiceLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	for _, svc := range svcs {
		_ = indexer.Add(svc)
	}

	return servingv1listers.NewServiceLister(indexer)
}

// ksvcWithConfigFailed returns a Knative Service with ConfigurationsReady=False.
func ksvcWithConfigFailed(namespace, name, reason, message string) *servingv1.Service {
	return &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: servingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:    apis.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  reason,
						Message: message,
					},
					{
						Type:    servingv1.ConfigurationConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  reason,
						Message: message,
					},
				},
			},
		},
	}
}

// ksvcWithReadyUnknown returns a Knative Service that is still reconciling
// (Ready=Unknown, no ConfigurationsReady=False).
func ksvcWithReadyUnknown(namespace, name string) *servingv1.Service {
	return &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: servingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   apis.ConditionReady,
						Status: corev1.ConditionUnknown,
					},
				},
			},
		},
	}
}

// assertCondition runs ReconcileKind and validates the Ready condition.
func assertCondition(
	t *testing.T,
	r *wasmmodule.Reconciler,
	module *api.WasmModule,
	wantReason, wantMessage string,
) {
	t.Helper()

	if err := r.ReconcileKind(context.Background(), module); err != nil {
		t.Fatalf("ReconcileKind() error: %v", err)
	}

	cond := module.Status.GetCondition(apis.ConditionReady)
	if cond == nil {
		t.Fatal("expected Ready condition, got nil")
	}

	if !cond.IsFalse() {
		t.Errorf("expected condition to be False, got %v", cond.Status)
	}

	if cond.Reason != wantReason {
		t.Errorf("reason: got %q, want %q", cond.Reason, wantReason)
	}

	if cond.Message != wantMessage {
		t.Errorf("message: got %q, want %q", cond.Message, wantMessage)
	}
}

func TestReconcileKind_TerminalConfigFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		svcReason   string
		svcMessage  string
		wantReason  string
		wantMessage string
	}{
		{
			name:        "RevisionFailed propagated to WasmModule",
			svcReason:   "RevisionFailed",
			svcMessage:  "Revision 'foo-00001' failed with message: OCI pull failed.",
			wantReason:  "RevisionFailed",
			wantMessage: "Revision 'foo-00001' failed with message: OCI pull failed.",
		},
		{
			name:        "ContainerMissing propagated to WasmModule",
			svcReason:   "ContainerMissing",
			svcMessage:  "Image not found.",
			wantReason:  "ContainerMissing",
			wantMessage: "Image not found.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const ns = "default"

			const moduleName = "my-wasm"

			svc := ksvcWithConfigFailed(ns, moduleName, tt.svcReason, tt.svcMessage)

			r := &wasmmodule.Reconciler{
				Tracker:       fakeTracker{},
				ServiceLister: buildServiceLister(svc),
			}

			module := &api.WasmModule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: ns,
				},
				Spec: api.WasmModuleSpec{
					Image: "example.com/img:latest",
				},
			}
			module.Status.InitializeConditions()

			assertCondition(t, r, module, tt.wantReason, tt.wantMessage)
		})
	}
}

func TestReconcileKind_TransientNotReady(t *testing.T) {
	t.Parallel()

	const ns = "default"

	const moduleName = "my-wasm"

	svc := ksvcWithReadyUnknown(ns, moduleName)

	r := &wasmmodule.Reconciler{
		Tracker:       fakeTracker{},
		ServiceLister: buildServiceLister(svc),
	}

	module := &api.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      moduleName,
			Namespace: ns,
		},
		Spec: api.WasmModuleSpec{
			Image: "example.com/img:latest",
		},
	}
	module.Status.InitializeConditions()

	// Transient: should be ServiceUnavailable, NOT RevisionFailed.
	// MarkServiceUnavailable formats the message as: Service %q wasn't found.
	wantMsg := fmt.Sprintf("Service %q wasn't found.", moduleName)
	assertCondition(t, r, module, "ServiceUnavailable", wantMsg)
}
