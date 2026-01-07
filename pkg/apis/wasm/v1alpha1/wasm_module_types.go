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
	corev1 "k8s.io/api/core/v1"
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
// Mirrors corev1.Container structure where applicable.
// Stdio is always inherited from the host process.
type WasmModuleSpec struct {
	// Image is the OCI artifact containing the WASM module.
	// Required field.
	Image string `json:"image"`

	// Args are command line arguments passed to the WASM module.
	// +optional
	Args []string `json:"args,omitempty"`

	// Env is a list of environment variables to set in the WASM module.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// VolumeMounts describes volumes to mount as WASI preopened directories.
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Volumes defines the volumes that can be mounted.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Resources specifies compute resource requirements.
	// requests.memory and limits.memory map to WASM memory limits.
	// limits.cpu is converted to fuel units for execution limiting.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Network specifies network access configuration for WASI sockets.
	// By default, network access is disabled.
	// +optional
	Network *NetworkSpec `json:"network,omitempty"`
}

// NetworkSpec specifies network access configuration for the WASM module.
// Address patterns support both IP addresses and hostnames.
// Format: "host:port" - host can be IP or hostname.
// Wildcards: "*:port", "host:*", "*:*"
type NetworkSpec struct {
	// Inherit indicates whether to inherit the host's full network stack.
	// When true, all network operations are allowed.
	// Maps to WasiCtxBuilder::inherit_network().
	// Defaults to false.
	// +optional
	Inherit bool `json:"inherit,omitempty"`

	// AllowIpNameLookup enables DNS resolution.
	// Maps to WasiCtxBuilder::allow_ip_name_lookup().
	// Defaults to true when Network is specified.
	// +optional
	AllowIpNameLookup *bool `json:"allowIpNameLookup,omitempty"`

	// Tcp specifies TCP socket permissions.
	// +optional
	Tcp *TcpSpec `json:"tcp,omitempty"`

	// Udp specifies UDP socket permissions.
	// +optional
	Udp *UdpSpec `json:"udp,omitempty"`
}

// TcpSpec specifies TCP socket permissions.
type TcpSpec struct {
	// Bind is a list of address patterns allowed for TCP bind.
	// +optional
	Bind []string `json:"bind,omitempty"`

	// Connect is a list of address patterns allowed for TCP connect.
	// +optional
	Connect []string `json:"connect,omitempty"`
}

// UdpSpec specifies UDP socket permissions.
type UdpSpec struct {
	// Bind is a list of address patterns allowed for UDP bind.
	// +optional
	Bind []string `json:"bind,omitempty"`

	// Connect is a list of address patterns allowed for UDP connect.
	// +optional
	Connect []string `json:"connect,omitempty"`

	// Outgoing is a list of address patterns allowed for UDP outgoing datagrams.
	// +optional
	Outgoing []string `json:"outgoing,omitempty"`
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
