/*
Copyright 2024 The Knative Authors

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"
)

// WasmModule is a Knative's WASM module that accepts HTTP.
//
// +genclient
// +genreconciler
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type WasmModule struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the WasmModule (from the client).
	// +optional
	Spec WasmModuleSpec `json:"spec,omitempty"`

	// Status communicates the observed state of the WasmModule (from the controller).
	// +optional
	Status WasmModuleStatus `json:"status,omitempty"`
}

var (
	// Check that WasmModule can be validated and defaulted.
	_ apis.Validatable   = (*WasmModule)(nil)
	_ apis.Defaultable   = (*WasmModule)(nil)
	_ kmeta.OwnerRefable = (*WasmModule)(nil)
	// Check that the type conforms to the duck Knative Resource shape.
	_ duckv1.KRShaped = (*WasmModule)(nil)
)


// WasmModuleSpec holds the desired state of the WasmModule (from the client).
type WasmModuleSpec struct {
	// ServiceName holds the name of the Knative Service to expose as an "addressable".
	ServiceName string `json:"serviceName"`

	// Source define the WASM source of the module
	Source ModuleSource `json:"source"`
}

// ModuleSource defines the source of the WASM module.
type ModuleSource struct {
	// Image is the container image where the WASM module is located.
	// This image must contain the WASM module distributed as OCI artifact.
	Image string `json:"image"`
}

const (
	// WasmModuleConditionReady is set when the revision is starting to materialize
	// runtime resources, and becomes true when those resources are ready.
	WasmModuleConditionReady = apis.ConditionReady
)

// WasmModuleStatus communicates the observed state of the WasmModule (from the controller).
type WasmModuleStatus struct {
	duckv1.Status `json:",inline"`

	// Address holds the information needed to connect this Addressable up to receive events.
	// +optional
	Address *duckv1.Addressable `json:"address,omitempty"`
}

// WasmModuleList is a list of WasmModule resources
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type WasmModuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []WasmModule `json:"items"`
}

// GetStatus retrieves the status of the resource. Implements the KRShaped interface.
func (as *WasmModule) GetStatus() *duckv1.Status {
	return &as.Status.Status
}
