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
	"testing"

	"github.com/cardil/knative-serving-wasm/pkg/reconciler/wasmmodule"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewRunnerConfigFromConfigMap_Empty(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: wasmmodule.RunnerConfigName},
		Data:       map[string]string{},
	}

	cfg, err := wasmmodule.NewRunnerConfigFromConfigMap(cm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.InsecureRegistries) != 0 {
		t.Errorf("expected empty InsecureRegistries, got %v", cfg.InsecureRegistries)
	}
}

func TestNewRunnerConfigFromConfigMap_EmptyValue(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: wasmmodule.RunnerConfigName},
		Data: map[string]string{
			"insecure-registries": "",
		},
	}

	cfg, err := wasmmodule.NewRunnerConfigFromConfigMap(cm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.InsecureRegistries) != 0 {
		t.Errorf("expected empty InsecureRegistries, got %v", cfg.InsecureRegistries)
	}
}

func TestNewRunnerConfigFromConfigMap_SingleEntry(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: wasmmodule.RunnerConfigName},
		Data: map[string]string{
			"insecure-registries": "- registry.local:5000\n",
		},
	}

	cfg, err := wasmmodule.NewRunnerConfigFromConfigMap(cm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.InsecureRegistries) != 1 {
		t.Fatalf("expected 1 InsecureRegistry, got %d", len(cfg.InsecureRegistries))
	}

	if cfg.InsecureRegistries[0] != "registry.local:5000" {
		t.Errorf("expected registry.local:5000, got %s", cfg.InsecureRegistries[0])
	}
}

func TestNewRunnerConfigFromConfigMap_MultipleEntries(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: wasmmodule.RunnerConfigName},
		Data: map[string]string{
			"insecure-registries": "- registry.local:5000\n- my-registry.internal:5000\n",
		},
	}

	cfg, err := wasmmodule.NewRunnerConfigFromConfigMap(cm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.InsecureRegistries) != 2 {
		t.Fatalf("expected 2 InsecureRegistries, got %d", len(cfg.InsecureRegistries))
	}

	if cfg.InsecureRegistries[0] != "registry.local:5000" {
		t.Errorf("expected registry.local:5000, got %s", cfg.InsecureRegistries[0])
	}

	if cfg.InsecureRegistries[1] != "my-registry.internal:5000" {
		t.Errorf("expected my-registry.internal:5000, got %s", cfg.InsecureRegistries[1])
	}
}

func TestNewRunnerConfigFromConfigMap_MalformedYAML(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: wasmmodule.RunnerConfigName},
		Data: map[string]string{
			"insecure-registries": "key: {broken: yaml: [",
		},
	}

	_, err := wasmmodule.NewRunnerConfigFromConfigMap(cm)
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

func TestMatchesInsecureRegistry_Matching(t *testing.T) {
	t.Parallel()

	registries := []string{"registry.local:5000", "my-registry.internal:5000", "localhost"}

	tests := []struct {
		image string
		want  bool
	}{
		{"registry.local:5000/mymodule:latest", true},
		{"my-registry.internal:5000/img:v1", true},
		{"localhost/my-module:latest", true},
		{"oci://localhost/my-module:v2", true},
		{"ghcr.io/example/module:latest", false},
		{"docker.io/library/nginx:latest", false},
	}

	for _, tt := range tests {
		got := wasmmodule.MatchesInsecureRegistry(tt.image, registries)
		if got != tt.want {
			t.Errorf("MatchesInsecureRegistry(%q, %v) = %v, want %v", tt.image, registries, got, tt.want)
		}
	}
}
