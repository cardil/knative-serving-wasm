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

package wasmmodule

import (
	"context"
	"encoding/json"
	"fmt"

	api "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	apireconciler "github.com/cardil/knative-serving-wasm/pkg/client/injection/reconciler/wasm/v1alpha1/wasmmodule"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
	servingv1listers "knative.dev/serving/pkg/client/listers/serving/v1"
)

var (
	// DefaultRunnerImage is the default image for the WASM runner.
	// Can be overridden at build time with -ldflags "-X pkg/reconciler/wasmmodule.DefaultRunnerImage=..."
	DefaultRunnerImage = "ghcr.io/cardil/knative-serving-wasm/runner"

	// DefaultImagePullPolicy is the default image pull policy for the WASM runner.
	// Can be overridden at build time with -ldflags "-X pkg/reconciler/wasmmodule.DefaultImagePullPolicy=Always"
	// Empty string means use Kubernetes default (IfNotPresent)
	DefaultImagePullPolicy = ""
)

// Reconciler implements apireconciler.Interface for
// WasmModule resources.
type Reconciler struct {
	// Tracker builds an index of what resources are watching other resources
	// so that we can immediately react to changes tracked resources.
	Tracker tracker.Interface

	// Listers index properties about resources
	ServiceLister servingv1listers.ServiceLister

	// Client is used to create the service
	Client servingv1client.ServingV1Interface
}

// Check that our Reconciler implements Interface.
var _ apireconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, module *api.WasmModule) reconciler.Event {
	log := logging.FromContext(ctx)

	// Use metadata.name as service name
	serviceName := module.Name

	if err := r.Tracker.TrackReference(tracker.Reference{
		APIVersion: servingv1.SchemeGroupVersion.String(),
		Kind:       "Service",
		Name:       serviceName,
		Namespace:  module.Namespace,
	}, module); err != nil {
		log.Errorf("Error tracking service %s: %v", serviceName, err)

		return err
	}

	srv, err := r.ServiceLister.Services(module.Namespace).Get(serviceName)

	if apierrs.IsNotFound(err) {
		log.Info("Service does not exist. Creating: ", serviceName)

		if srv, err = r.createService(ctx, module); err != nil {
			module.Status.MarkServiceUnavailable(serviceName)

			return err
		}
	} else if err != nil {
		log.Errorf("Error reconciling service %s: %v", serviceName, err)

		return err
	}

	// Only mark ready when the underlying Service is ready
	// This ensures pods are running and Kourier has the route before tests can access it
	readyCondition := srv.Status.GetCondition(apis.ConditionReady)
	if readyCondition != nil && readyCondition.IsTrue() {
		module.Status.MarkServiceAvailable()

		// Use the external URL (srv.Status.URL) instead of internal address (srv.Status.Address)
		// The external URL works with the Knative ingress (e.g., Kourier) and follows
		// the configured domain (e.g., example.com for "No DNS" mode on Kind clusters)
		if srv.Status.URL != nil {
			module.Status.Address = &duckv1.Addressable{
				URL: srv.Status.URL,
			}
		}
	} else {
		module.Status.MarkServiceUnavailable(serviceName)
	}

	return nil
}

func (r *Reconciler) createService(ctx context.Context, module *api.WasmModule) (*servingv1.Service, error) {
	log := logging.FromContext(ctx)

	// Use metadata.name as service name
	serviceName := module.Name

	// Build WASI config for the runner
	wasiConfig, err := buildRunnerConfig(module)
	if err != nil {
		return nil, fmt.Errorf("failed to build runner config: %w", err)
	}

	// Prepare environment variables
	envVars := []corev1.EnvVar{
		{
			Name:  "IMAGE",
			Value: module.Spec.Image,
		},
		{
			Name:  "WASI_CONFIG",
			Value: wasiConfig,
		},
	}

	// Build container spec
	container := corev1.Container{
		Image:        DefaultRunnerImage,
		Env:          envVars,
		VolumeMounts: module.Spec.VolumeMounts,
		Resources:    module.Spec.Resources,
	}

	// Set ImagePullPolicy if configured (e.g., "Always" for e2e tests)
	if DefaultImagePullPolicy != "" {
		container.ImagePullPolicy = corev1.PullPolicy(DefaultImagePullPolicy)
	}

	srv, err := r.Client.Services(module.Namespace).Create(ctx, &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceName,
			Namespace:   module.Namespace,
			Labels:      module.Labels,
			Annotations: module.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(module, module.GetGroupVersionKind()),
			},
		},
		Spec: servingv1.ServiceSpec{
			ConfigurationSpec: servingv1.ConfigurationSpec{
				Template: servingv1.RevisionTemplateSpec{
					Spec: servingv1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{container},
							Volumes:    module.Spec.Volumes,
						},
					},
				},
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		log.Errorf("Error creating kservice %s: %v", serviceName, err)
	}

	return srv, err
}

// RunnerWasiConfig represents the WASI configuration passed to the runner.
type RunnerWasiConfig struct {
	Args    []string             `json:"args,omitempty"`
	Env     map[string]string    `json:"env,omitempty"`
	Dirs    []RunnerDirConfig    `json:"dirs,omitempty"`
	Network *RunnerNetworkConfig `json:"network,omitempty"`
}

// RunnerDirConfig represents a directory mount configuration.
type RunnerDirConfig struct {
	HostPath  string `json:"hostPath"`
	GuestPath string `json:"guestPath"`
	ReadOnly  bool   `json:"readOnly"`
}

// RunnerNetworkConfig represents network configuration for the runner.
type RunnerNetworkConfig struct {
	Inherit           bool     `json:"inherit,omitempty"`
	AllowIpNameLookup bool     `json:"allowIpNameLookup,omitempty"`
	TcpBind           []string `json:"tcpBind,omitempty"`
	TcpConnect        []string `json:"tcpConnect,omitempty"`
	UdpBind           []string `json:"udpBind,omitempty"`
	UdpConnect        []string `json:"udpConnect,omitempty"`
	UdpOutgoing       []string `json:"udpOutgoing,omitempty"`
}

// buildRunnerConfig builds the WASI configuration JSON for the runner.
func buildRunnerConfig(wm *api.WasmModule) (string, error) {
	config := RunnerWasiConfig{
		Args: wm.Spec.Args,
	}

	// Convert environment variables
	if len(wm.Spec.Env) > 0 {
		config.Env = make(map[string]string)
		for _, env := range wm.Spec.Env {
			if env.ValueFrom != nil {
				// ValueFrom is not supported in this implementation
				return "", fmt.Errorf("env var %s uses valueFrom which is not yet supported", env.Name)
			}
			config.Env[env.Name] = env.Value
		}
	}

	// Convert volume mounts to directory configs
	// The runner sees volumes at the mount path specified by the VolumeMount,
	// not at any hardcoded location. Kubernetes mounts the volume at vm.MountPath.
	if len(wm.Spec.VolumeMounts) > 0 {
		config.Dirs = make([]RunnerDirConfig, 0, len(wm.Spec.VolumeMounts))

		// Build a set of valid volume names for validation
		validVolumes := make(map[string]bool)
		for _, vol := range wm.Spec.Volumes {
			validVolumes[vol.Name] = true
		}

		for _, vm := range wm.Spec.VolumeMounts {
			if !validVolumes[vm.Name] {
				return "", fmt.Errorf("volume mount %s references undefined volume %s", vm.MountPath, vm.Name)
			}

			// With Kubernetes subPath, the subdirectory content is mounted directly
			// at vm.MountPath, so hostPath is always just vm.MountPath.
			config.Dirs = append(config.Dirs, RunnerDirConfig{
				HostPath:  vm.MountPath,
				GuestPath: vm.MountPath,
				ReadOnly:  vm.ReadOnly,
			})
		}
	}

	// Convert network configuration
	if wm.Spec.Network != nil {
		config.Network = &RunnerNetworkConfig{
			Inherit: wm.Spec.Network.Inherit,
		}

		// Default allowIpNameLookup to true if not specified
		if wm.Spec.Network.AllowIpNameLookup != nil {
			config.Network.AllowIpNameLookup = *wm.Spec.Network.AllowIpNameLookup
		} else {
			config.Network.AllowIpNameLookup = true
		}

		// Convert TCP configuration
		if wm.Spec.Network.Tcp != nil {
			config.Network.TcpBind = wm.Spec.Network.Tcp.Bind
			config.Network.TcpConnect = wm.Spec.Network.Tcp.Connect
		}

		// Convert UDP configuration
		if wm.Spec.Network.Udp != nil {
			config.Network.UdpBind = wm.Spec.Network.Udp.Bind
			config.Network.UdpConnect = wm.Spec.Network.Udp.Connect
			config.Network.UdpOutgoing = wm.Spec.Network.Udp.Outgoing
		}
	}

	// Serialize to JSON
	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal WASI config: %w", err)
	}

	return string(configJSON), nil
}
