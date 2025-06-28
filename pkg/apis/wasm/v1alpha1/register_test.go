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

package v1alpha1_test

import (
	"testing"

	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestRegisterHelpers(t *testing.T) {
	t.Parallel()

	if got, want := wasmv1alpha1.Kind("Foo"), "Foo.wasm.serving.knative.dev"; got.String() != want {
		t.Errorf("Kind(Foo) = %v, want %v", got.String(), want)
	}

	if got, want := wasmv1alpha1.Resource("Foo"), "Foo.wasm.serving.knative.dev"; got.String() != want {
		t.Errorf("Resource(Foo) = %v, want %v", got.String(), want)
	}

	if got, want := wasmv1alpha1.SchemeGroupVersion.String(), "wasm.serving.knative.dev/v1alpha1"; got != want {
		t.Errorf("SchemeGroupVersion() = %v, want %v", got, want)
	}

	scheme := runtime.NewScheme()

	err := wasmv1alpha1.AddKnownTypes(scheme)
	if err != nil {
		t.Errorf("addKnownTypes() = %v", err)
	}
}
