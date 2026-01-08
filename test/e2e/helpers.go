// Copyright 2024 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	wasmclientset "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned"
)

const (
	// DefaultTimeout for waiting operations
	DefaultTimeout = 5 * time.Minute

	// DefaultPollInterval for polling operations
	DefaultPollInterval = 250 * time.Millisecond
)

// TestContext holds common test resources
type TestContext struct {
	T            *testing.T
	Namespace    string
	Config       *Config
	KubeClient   kubernetes.Interface
	WasmClient   wasmclientset.Interface
	CleanupFuncs []func()
}

// Cleanup runs all registered cleanup functions
func (tc *TestContext) Cleanup() {
	for i := len(tc.CleanupFuncs) - 1; i >= 0; i-- {
		tc.CleanupFuncs[i]()
	}
}

// AddCleanup registers a cleanup function
func (tc *TestContext) AddCleanup(f func()) {
	tc.CleanupFuncs = append(tc.CleanupFuncs, f)
}

// CreateNamespace creates a test namespace
func (tc *TestContext) CreateNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tc.Namespace,
			Labels: map[string]string{
				"test": "e2e",
			},
		},
	}

	_, err := tc.KubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", tc.Namespace, err)
	}

	tc.T.Logf("Created namespace: %s", tc.Namespace)

	tc.AddCleanup(func() {
		tc.T.Logf("Deleting namespace: %s", tc.Namespace)
		err := tc.KubeClient.CoreV1().Namespaces().Delete(context.Background(), tc.Namespace, metav1.DeleteOptions{})
		if err != nil {
			tc.T.Logf("Failed to delete namespace %s: %v", tc.Namespace, err)
		}
	})

	return nil
}

// WaitForWasmModuleReady waits for a WasmModule to become ready
func (tc *TestContext) WaitForWasmModuleReady(ctx context.Context, name string) error {
	tc.T.Logf("Waiting for WasmModule %s to be ready...", name)

	return wait.PollUntilContextTimeout(ctx, DefaultPollInterval, DefaultTimeout, true, func(ctx context.Context) (bool, error) {
		wm, err := tc.WasmClient.WasmV1alpha1().WasmModules(tc.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// Check if Ready condition is True
		for _, cond := range wm.Status.Conditions {
			if cond.Type == "Ready" {
				if cond.Status == "True" {
					tc.T.Logf("WasmModule %s is ready", name)
					return true, nil
				}
				tc.T.Logf("WasmModule %s not ready yet: %s - %s", name, cond.Reason, cond.Message)
			}
		}

		return false, nil
	})
}

// GetWasmModuleURL returns the URL for accessing a WasmModule
func (tc *TestContext) GetWasmModuleURL(ctx context.Context, name string) (string, error) {
	wm, err := tc.WasmClient.WasmV1alpha1().WasmModules(tc.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get WasmModule %s: %w", name, err)
	}

	if wm.Status.Address == nil || wm.Status.Address.URL == nil {
		return "", fmt.Errorf("WasmModule %s has no URL in status", name)
	}

	return wm.Status.Address.URL.String(), nil
}

// DeployEchoServer deploys a simple echo server for network tests
func (tc *TestContext) DeployEchoServer(ctx context.Context) error {
	tc.T.Log("Deploying echo server...")

	// Create deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-server",
			Namespace: tc.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "echo-server",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "echo-server",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "echo",
						Image: "docker.io/hashicorp/http-echo:latest",
						Args:  []string{"-text=echo-response"},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 5678,
							Name:          "http",
						}},
					}},
				},
			},
		},
	}

	_, err := tc.KubeClient.AppsV1().Deployments(tc.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create echo server deployment: %w", err)
	}

	// Create service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-server",
			Namespace: tc.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "echo-server",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(5678),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	_, err = tc.KubeClient.CoreV1().Services(tc.Namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create echo server service: %w", err)
	}

	// Wait for deployment to be ready
	tc.T.Log("Waiting for echo server to be ready...")
	return wait.PollUntilContextTimeout(ctx, DefaultPollInterval, DefaultTimeout, true, func(ctx context.Context) (bool, error) {
		dep, err := tc.KubeClient.AppsV1().Deployments(tc.Namespace).Get(ctx, "echo-server", metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if dep.Status.ReadyReplicas > 0 {
			tc.T.Log("Echo server is ready")
			return true, nil
		}

		return false, nil
	})
}

// HTTPGet performs an HTTP GET request and returns the response body
func (tc *TestContext) HTTPGet(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

// CreateWasmModule creates a WasmModule resource
func (tc *TestContext) CreateWasmModule(ctx context.Context, wm *wasmv1alpha1.WasmModule) (*wasmv1alpha1.WasmModule, error) {
	created, err := tc.WasmClient.WasmV1alpha1().WasmModules(tc.Namespace).Create(ctx, wm, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create WasmModule: %w", err)
	}

	tc.T.Logf("Created WasmModule: %s", created.Name)
	return created, nil
}

// int32Ptr returns a pointer to an int32
func int32Ptr(i int32) *int32 {
	return &i
}
