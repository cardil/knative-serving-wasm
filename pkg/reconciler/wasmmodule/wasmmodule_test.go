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

package wasmmodule

import (
	"encoding/json"
	"os"
	"testing"

	api "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestBuildRunnerConfigMatchesGolden verifies that the JSON produced by
// buildRunnerConfig matches the documented wire format that the runner parses.
// If this test fails, both the golden file and the runner must be updated together.
func TestBuildRunnerConfigMatchesGolden(t *testing.T) {
	trueVal := true
	module := &api.WasmModule{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: api.WasmModuleSpec{
			Image: "example.com/img:latest",
			Args:  []string{"--verbose"},
			Env: []corev1.EnvVar{
				{Name: "GREETING", Value: "hello"},
				{Name: "PORT", Value: "8080"},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "data", MountPath: "/mnt/data", ReadOnly: false},
				{Name: "ro", MountPath: "/mnt/ro", ReadOnly: true},
			},
			Volumes: []corev1.Volume{
				{Name: "data"},
				{Name: "ro"},
			},
			Network: &api.NetworkSpec{
				Inherit:           false,
				AllowIpNameLookup: &trueVal,
				Tcp: &api.TcpSpec{
					Connect: []string{"example.com:443"},
				},
			},
		},
	}

	got, err := buildRunnerConfig(module)
	if err != nil {
		t.Fatalf("buildRunnerConfig() error: %v", err)
	}

	goldenBytes, err := os.ReadFile("testdata/wasi_config.golden.json")
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	var gotMap, wantMap any
	if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal(goldenBytes, &wantMap); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	gotJSON, err := json.MarshalIndent(gotMap, "", "  ")
	if err != nil {
		t.Fatalf("marshal got normalized JSON: %v", err)
	}
	wantJSON, err := json.MarshalIndent(wantMap, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden normalized JSON: %v", err)
	}

	if string(gotJSON) != string(wantJSON) {
		t.Errorf("buildRunnerConfig output does not match golden.\ngot:\n%s\n\nwant:\n%s", gotJSON, wantJSON)
	}
}
