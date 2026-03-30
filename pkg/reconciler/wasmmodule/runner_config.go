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
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/configmap"
)

const (
	// RunnerConfigName is the name of the ConfigMap containing runner configuration.
	RunnerConfigName = "config-runner"
)

// RunnerConfig holds the runner-level configuration read from the config-runner ConfigMap.
type RunnerConfig struct {
	// InsecureRegistries is the list of registry host[:port] entries that the
	// runner should access over plain HTTP instead of HTTPS.
	InsecureRegistries []string
}

// NewRunnerConfigFromConfigMap creates a RunnerConfig from the given ConfigMap.
func NewRunnerConfigFromConfigMap(cm *corev1.ConfigMap) (*RunnerConfig, error) {
	cfg := &RunnerConfig{}

	raw, ok := cm.Data["insecure-registries"]
	if !ok || raw == "" {
		return cfg, nil
	}

	var registries []string
	if err := yaml.Unmarshal([]byte(raw), &registries); err != nil {
		return nil, fmt.Errorf("failed to parse insecure-registries from %s ConfigMap: %w", RunnerConfigName, err)
	}

	cfg.InsecureRegistries = registries

	return cfg, nil
}

// RunnerConfigStore is a store for the runner configuration.
type RunnerConfigStore struct {
	*configmap.UntypedStore
}

// NewRunnerConfigStore creates a new RunnerConfigStore.
func NewRunnerConfigStore(logger configmap.Logger) *RunnerConfigStore {
	return &RunnerConfigStore{
		UntypedStore: configmap.NewUntypedStore(
			"runner",
			logger,
			configmap.Constructors{
				RunnerConfigName: NewRunnerConfigFromConfigMap,
			},
		),
	}
}

// GetRunnerConfig fetches the current RunnerConfig from the store.
func (s *RunnerConfigStore) GetRunnerConfig() *RunnerConfig {
	cfg, ok := s.UntypedLoad(RunnerConfigName).(*RunnerConfig)
	if !ok || cfg == nil {
		return &RunnerConfig{}
	}

	return cfg
}
