/*
Copyright 2021 The Knative Authors

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

package main

import (
	"log"

	"knative.dev/hack/schema/commands"
	"knative.dev/hack/schema/registry"

	"github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
)

// schema is a tool to dump the schema for Eventing resources.
func main() {
	registry.Register(&v1alpha1.WasmModule{})

	if err := commands.New("github.com/cardil/knative-serving-wasm").Execute(); err != nil {
		log.Fatal("Error during command execution: ", err)
	}
}
