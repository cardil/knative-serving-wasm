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
	"context"
)

// SetDefaults implements apis.Defaultable.
func (as *WasmModule) SetDefaults(_ context.Context) {
	as.Spec.SetDefaults()
}

// SetDefaults sets default values for WasmModuleSpec.
func (ass *WasmModuleSpec) SetDefaults() {
	// Set defaults for network configuration
	if ass.Network != nil {
		ass.Network.SetDefaults()
	}
}

// SetDefaults sets default values for NetworkSpec.
func (ns *NetworkSpec) SetDefaults() {
	// Default allowIpNameLookup to true when network is configured but field is nil
	if ns.AllowIpNameLookup == nil {
		trueVal := true
		ns.AllowIpNameLookup = &trueVal
	}
}
