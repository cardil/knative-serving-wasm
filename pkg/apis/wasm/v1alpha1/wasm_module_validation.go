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

	if ass.ServiceName == "" {
		//goland:noinspection GoDfaNilDereference
		errs = errs.Also(apis.ErrMissingField("serviceName"))
	}

	imageFieldPath := "source.image"
	// A WasmModule must specify its source. The 'source.image' is the field
	// that points to the OCI image containing the Wasm module.
	if ass.Source.Image == "" {
		//goland:noinspection GoDfaNilDereference
		errs = errs.Also(apis.ErrMissingField(imageFieldPath))
	}

	// TODO: validate remote registry for accessibility of parsed ref
	if _, err := reference.Parse(ass.Source.Image); err != nil {
		//goland:noinspection GoDfaNilDereference
		errs = errs.Also(apis.ErrInvalidValue(ass.Source.Image, imageFieldPath, err.Error()))
	}

	return errs
}
