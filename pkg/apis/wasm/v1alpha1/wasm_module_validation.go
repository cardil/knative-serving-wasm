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
	"strings"

	"github.com/distribution/reference"
	"knative.dev/pkg/apis"
)

// Validate implements apis.Validatable.
func (as *WasmModule) Validate(ctx context.Context) *apis.FieldError {
	return as.Spec.Validate(ctx).ViaField("spec")
}

// Validate implements apis.Validatable.
func (ass *WasmModuleSpec) Validate(_ context.Context) *apis.FieldError {
	var errs *apis.FieldError

	imageFieldPath := "image"
	// A WasmModule must specify its image. The 'image' field
	// points to the OCI image containing the Wasm module.
	if ass.Image == "" {
		//goland:noinspection GoDfaNilDereference
		errs = errs.Also(apis.ErrMissingField(imageFieldPath))
	} else if _, err := reference.Parse(ass.Image); err != nil {
		// TODO: validate remote registry for accessibility of parsed ref
		//goland:noinspection GoDfaNilDereference
		errs = errs.Also(apis.ErrInvalidValue(ass.Image, imageFieldPath, err.Error()))
	}

	// Validate network configuration if present
	if ass.Network != nil {
		errs = errs.Also(ass.Network.Validate().ViaField("network"))
	}

	return errs
}

// Validate validates the NetworkSpec.
func (ns *NetworkSpec) Validate() *apis.FieldError {
	var errs *apis.FieldError

	// Validate TCP configuration
	if ns.Tcp != nil {
		for i, addr := range ns.Tcp.Bind {
			if err := validateAddressPattern(addr); err != nil {
				errs = errs.Also(apis.ErrInvalidArrayValue(addr, "tcp.bind", i))
			}
		}
		for i, addr := range ns.Tcp.Connect {
			if err := validateAddressPattern(addr); err != nil {
				errs = errs.Also(apis.ErrInvalidArrayValue(addr, "tcp.connect", i))
			}
		}
	}

	// Validate UDP configuration
	if ns.Udp != nil {
		for i, addr := range ns.Udp.Bind {
			if err := validateAddressPattern(addr); err != nil {
				errs = errs.Also(apis.ErrInvalidArrayValue(addr, "udp.bind", i))
			}
		}
		for i, addr := range ns.Udp.Connect {
			if err := validateAddressPattern(addr); err != nil {
				errs = errs.Also(apis.ErrInvalidArrayValue(addr, "udp.connect", i))
			}
		}
		for i, addr := range ns.Udp.Outgoing {
			if err := validateAddressPattern(addr); err != nil {
				errs = errs.Also(apis.ErrInvalidArrayValue(addr, "udp.outgoing", i))
			}
		}
	}

	return errs
}

// validateAddressPattern validates address patterns like "host:port", "*:port", "host:*", "*:*"
func validateAddressPattern(pattern string) error {
	if pattern == "" {
		return apis.ErrInvalidValue(pattern, "", "address pattern cannot be empty")
	}
	// All patterns must contain a colon separator between host and port
	if !strings.Contains(pattern, ":") {
		return apis.ErrInvalidValue(pattern, "", "address pattern must be in format 'host:port'")
	}
	// Validate minimum length (e.g., "*:*" is 3 chars)
	if len(pattern) < 3 {
		return apis.ErrInvalidValue(pattern, "", "address pattern must be in format 'host:port'")
	}
	return nil
}
