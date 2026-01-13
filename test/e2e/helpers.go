// Copyright 2025 The Knative Authors
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
	"net/url"
	"os"
	"strings"
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
	// Dump diagnostic info if test failed
	if tc.T.Failed() {
		tc.DumpDiagnostics()
	}

	for i := len(tc.CleanupFuncs) - 1; i >= 0; i-- {
		tc.CleanupFuncs[i]()
	}
}

// DumpDiagnostics logs important debugging information for failed tests
func (tc *TestContext) DumpDiagnostics() {
	ctx := context.Background()
	tc.T.Logf("=== DIAGNOSTIC DUMP for namespace %s ===", tc.Namespace)

	// Dump pods and their images
	pods, err := tc.KubeClient.CoreV1().Pods(tc.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		tc.T.Logf("Failed to list pods: %v", err)
	} else {
		tc.T.Logf("Pods in namespace %s:", tc.Namespace)
		for _, pod := range pods.Items {
			tc.T.Logf("  Pod: %s (Phase: %s)", pod.Name, pod.Status.Phase)
			for _, cs := range pod.Status.ContainerStatuses {
				tc.T.Logf("    Container: %s (Ready: %v, Image: %s, ImageID: %s)",
					cs.Name, cs.Ready, cs.Image, cs.ImageID)
				if cs.State.Waiting != nil {
					tc.T.Logf("      Waiting: %s - %s", cs.State.Waiting.Reason, cs.State.Waiting.Message)
				}
				if cs.State.Terminated != nil {
					tc.T.Logf("      Terminated: %s - %s (Exit: %d)",
						cs.State.Terminated.Reason, cs.State.Terminated.Message, cs.State.Terminated.ExitCode)
				}
			}
		}
	}

	// Dump events
	events, err := tc.KubeClient.CoreV1().Events(tc.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		tc.T.Logf("Failed to list events: %v", err)
	} else {
		tc.T.Logf("Recent events in namespace %s:", tc.Namespace)
		for _, event := range events.Items {
			tc.T.Logf("  %s: %s %s - %s",
				event.InvolvedObject.Name, event.Type, event.Reason, event.Message)
		}
	}

	// Dump pod logs for user-container
	if pods != nil {
		for _, pod := range pods.Items {
			for _, container := range []string{"user-container", "queue-proxy"} {
				logs, err := tc.KubeClient.CoreV1().Pods(tc.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
					Container: container,
					TailLines: int64Ptr(50),
				}).Do(ctx).Raw()
				if err != nil {
					tc.T.Logf("Failed to get %s logs for pod %s: %v", container, pod.Name, err)
				} else {
					tc.T.Logf("=== Logs from %s/%s (last 50 lines) ===\n%s", pod.Name, container, string(logs))
				}
			}
		}
	}

	tc.T.Logf("=== END DIAGNOSTIC DUMP ===")
}

// int64Ptr returns a pointer to an int64
func int64Ptr(i int64) *int64 {
	return &i
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

// WaitForWasmModuleReady waits for a WasmModule to become ready with a URL
func (tc *TestContext) WaitForWasmModuleReady(ctx context.Context, name string) error {
	tc.T.Logf("Waiting for WasmModule %s to be ready...", name)

	return wait.PollUntilContextTimeout(ctx, DefaultPollInterval, DefaultTimeout, true, func(ctx context.Context) (bool, error) {
		wm, err := tc.WasmClient.WasmV1alpha1().WasmModules(tc.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// Check if Ready condition is True
		ready := false
		for _, cond := range wm.Status.Conditions {
			if cond.Type == "Ready" {
				if cond.Status == "True" {
					ready = true
				} else {
					tc.T.Logf("WasmModule %s not ready yet: %s - %s", name, cond.Reason, cond.Message)
					return false, nil
				}
			}
		}

		// Also wait for the URL to be available
		if ready && wm.Status.Address != nil && wm.Status.Address.URL != nil {
			tc.T.Logf("WasmModule %s is ready with URL: %s", name, wm.Status.Address.URL.String())
			return true, nil
		}

		if ready {
			tc.T.Logf("WasmModule %s Ready condition is True but URL not yet available", name)
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

// GetIngressAddress returns the address of the Kourier ingress gateway.
// Uses LOCAL_GATEWAY_ADDRESS env var if set (for port-forwarded local access).
// Uses GATEWAY_OVERRIDE and GATEWAY_NAMESPACE_OVERRIDE env vars if set.
// For Kind/local clusters without LoadBalancer, uses NodePort access.
// For cloud clusters (GKE, etc.), waits for LoadBalancer IP.
func (tc *TestContext) GetIngressAddress(ctx context.Context) (string, error) {
	// Check for local gateway address (set by runner.sh for Kind port-forwarding)
	if localAddr := os.Getenv("LOCAL_GATEWAY_ADDRESS"); localAddr != "" {
		tc.T.Logf("Using LOCAL_GATEWAY_ADDRESS: %s", localAddr)
		return localAddr, nil
	}

	// Determine gateway namespace and name from environment or defaults
	gatewayNamespace := "kourier-system"
	if ns := os.Getenv("GATEWAY_NAMESPACE_OVERRIDE"); ns != "" {
		gatewayNamespace = ns
	}
	gatewayName := "kourier"
	if name := os.Getenv("GATEWAY_OVERRIDE"); name != "" {
		gatewayName = name
	}

	// Get the gateway service
	svc, err := tc.KubeClient.CoreV1().Services(gatewayNamespace).Get(ctx, gatewayName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get gateway service %s/%s: %w", gatewayNamespace, gatewayName, err)
	}

	// Check if this is a LoadBalancer service (cloud cluster)
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		// Wait for LoadBalancer to be ready (up to 2 minutes)
		tc.T.Log("Waiting for LoadBalancer to be ready...")
		var lbAddr string
		err := wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			svc, err := tc.KubeClient.CoreV1().Services(gatewayNamespace).Get(ctx, gatewayName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if len(svc.Status.LoadBalancer.Ingress) > 0 {
				ingress := svc.Status.LoadBalancer.Ingress[0]
				if ingress.IP != "" {
					lbAddr = ingress.IP
					return true, nil
				}
				if ingress.Hostname != "" {
					lbAddr = ingress.Hostname
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return "", fmt.Errorf("LoadBalancer not ready after 2 minutes: %w", err)
		}
		tc.T.Logf("Using LoadBalancer IP: %s", lbAddr)
		return lbAddr, nil
	}

	// For local clusters (Kind, k3d, etc.) use NodePort approach
	// Get the first node's internal IP
	nodes, err := tc.KubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodes.Items) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}

	// Get node internal IP
	var nodeIP string
	for _, addr := range nodes.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			nodeIP = addr.Address
			break
		}
	}

	if nodeIP == "" {
		return "", fmt.Errorf("no internal IP found for node")
	}

	// Get NodePort for http port
	for _, port := range svc.Spec.Ports {
		if port.Name == "http2" || port.Port == 80 {
			if port.NodePort > 0 {
				tc.T.Logf("Using NodePort access: %s:%d", nodeIP, port.NodePort)
				return fmt.Sprintf("%s:%d", nodeIP, port.NodePort), nil
			}
		}
	}

	return "", fmt.Errorf("no suitable port found in gateway service %s/%s", gatewayNamespace, gatewayName)
}

// HTTPGet performs an HTTP GET request to a WasmModule URL.
// For local Kind clusters, it routes through the ingress with Host header.
// Retries on 404 errors to handle ingress propagation delays.
func (tc *TestContext) HTTPGet(ctx context.Context, targetURL string) (string, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Get the ingress address
	ingressAddr, err := tc.GetIngressAddress(ctx)
	if err != nil {
		tc.T.Logf("Warning: Could not get ingress address, trying direct: %v", err)
		// Fallback to direct request
		return tc.directHTTPGet(ctx, targetURL)
	}

	// Build request URL using ingress address
	requestURL := fmt.Sprintf("http://%s%s", ingressAddr, parsed.RequestURI())
	tc.T.Logf("Making request to %s with Host: %s", requestURL, parsed.Host)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Retry logic for 404 errors (ingress propagation delay)
	var lastErr error
	var lastBody string
	retryInterval := 250 * time.Millisecond
	retryTimeout := 4 * time.Second
	deadline := time.Now().Add(retryTimeout)

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		// Set Host header to route to the correct Knative service
		req.Host = parsed.Host

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to perform request: %w", err)
			time.Sleep(retryInterval)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			return string(body), nil
		}

		// Retry on 404 (ingress not ready yet)
		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			lastBody = string(body)
			time.Sleep(retryInterval)
			continue
		}

		// For other errors, fail immediately
		return string(body), fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Timeout reached
	return lastBody, fmt.Errorf("request timed out after %v, last error: %w", retryTimeout, lastErr)
}

// directHTTPGet performs a direct HTTP GET request without routing through ingress
func (tc *TestContext) directHTTPGet(ctx context.Context, targetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
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
