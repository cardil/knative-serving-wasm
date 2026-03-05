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
	"errors"
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
	// DefaultTimeout for waiting operations.
	DefaultTimeout = 5 * time.Minute

	// DefaultPollInterval for polling operations.
	DefaultPollInterval = 250 * time.Millisecond
)

// TestContext holds common test resources.
type TestContext struct {
	T            *testing.T
	Namespace    string
	Config       *Config
	KubeClient   kubernetes.Interface
	WasmClient   wasmclientset.Interface
	CleanupFuncs []func()
}

// Cleanup runs all registered cleanup functions.
func (tc *TestContext) Cleanup() {
	// Dump diagnostic info if test failed
	if tc.T.Failed() {
		tc.DumpDiagnostics()
	}

	for i := len(tc.CleanupFuncs) - 1; i >= 0; i-- {
		tc.CleanupFuncs[i]()
	}
}

// DumpDiagnostics logs important debugging information for failed tests.
func (tc *TestContext) DumpDiagnostics() {
	ctx := context.Background()

	tc.T.Logf("=== DIAGNOSTIC DUMP for namespace %s ===", tc.Namespace)

	tc.dumpPods(ctx)
	tc.dumpEvents(ctx)
	tc.dumpPodLogs(ctx)

	tc.T.Logf("=== END DIAGNOSTIC DUMP ===")
}

// AddCleanup registers a cleanup function.
func (tc *TestContext) AddCleanup(f func()) {
	tc.CleanupFuncs = append(tc.CleanupFuncs, f)
}

// CreateNamespace creates a test namespace.
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

	//nolint:contextcheck // cleanup runs outside the request context
	tc.AddCleanup(func() {
		tc.T.Logf("Deleting namespace: %s", tc.Namespace)

		err := tc.KubeClient.CoreV1().Namespaces().Delete(
			context.Background(), tc.Namespace, metav1.DeleteOptions{},
		)
		if err != nil {
			tc.T.Logf("Failed to delete namespace %s: %v", tc.Namespace, err)
		}
	})

	return nil
}

// WaitForWasmModuleReady waits for a WasmModule to become ready with a URL.
func (tc *TestContext) WaitForWasmModuleReady(
	ctx context.Context, name string,
) error {
	tc.T.Logf("Waiting for WasmModule %s to be ready...", name)

	pollFn := func(ctx context.Context) (bool, error) {
		return tc.checkWasmModuleReady(ctx, name)
	}

	return wait.PollUntilContextTimeout(
		ctx, DefaultPollInterval, DefaultTimeout, true, pollFn,
	)
}

// GetWasmModuleURL returns the URL for accessing a WasmModule.
func (tc *TestContext) GetWasmModuleURL(
	ctx context.Context, name string,
) (string, error) {
	wm, err := tc.WasmClient.WasmV1alpha1().WasmModules(
		tc.Namespace,
	).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get WasmModule %s: %w", name, err)
	}

	if wm.Status.Address == nil || wm.Status.Address.URL == nil {
		return "", fmt.Errorf("WasmModule %s has no URL in status", name)
	}

	return wm.Status.Address.URL.String(), nil
}

// DeployEchoServer deploys a simple echo server for network tests.
func (tc *TestContext) DeployEchoServer(ctx context.Context) error {
	tc.T.Log("Deploying echo server...")

	if err := tc.createEchoDeployment(ctx); err != nil {
		return err
	}

	if err := tc.createEchoService(ctx); err != nil {
		return err
	}

	if err := tc.waitForEchoDeployment(ctx); err != nil {
		return err
	}

	if err := tc.waitForEchoEndpoints(ctx); err != nil {
		return err
	}

	return tc.verifyEchoNetworking(ctx)
}

// GetIngressAddress returns the address of the Kourier ingress gateway.
// Uses LOCAL_GATEWAY_ADDRESS env var if set (for port-forwarded local access).
// Uses GATEWAY_OVERRIDE and GATEWAY_NAMESPACE_OVERRIDE env vars if set.
// For Kind/local clusters without LoadBalancer, uses NodePort access.
// For cloud clusters (GKE, etc.), waits for LoadBalancer IP.
func (tc *TestContext) GetIngressAddress(
	ctx context.Context,
) (string, error) {
	// Check for local gateway address (set by runner.sh for Kind port-forwarding)
	if localAddr := os.Getenv("LOCAL_GATEWAY_ADDRESS"); localAddr != "" {
		tc.T.Logf("Using LOCAL_GATEWAY_ADDRESS: %s", localAddr)

		return localAddr, nil
	}

	gatewayNamespace, gatewayName := getGatewayInfo()

	svc, err := tc.KubeClient.CoreV1().Services(
		gatewayNamespace,
	).Get(ctx, gatewayName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf(
			"failed to get gateway service %s/%s: %w",
			gatewayNamespace, gatewayName, err,
		)
	}

	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return tc.waitForLoadBalancer(ctx, gatewayNamespace, gatewayName)
	}

	return tc.getNodePortAddress(ctx, svc, gatewayNamespace, gatewayName)
}

// HTTPGet performs an HTTP GET request to a WasmModule URL.
// For local Kind clusters, it routes through the ingress with Host header.
// Retries on 404 errors to handle ingress propagation delays.
func (tc *TestContext) HTTPGet(
	ctx context.Context, targetURL string,
) (string, error) {
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

	return tc.httpGetViaIngress(ctx, parsed, ingressAddr)
}

// CreateWasmModule creates a WasmModule resource.
func (tc *TestContext) CreateWasmModule(
	ctx context.Context,
	wm *wasmv1alpha1.WasmModule,
) (*wasmv1alpha1.WasmModule, error) {
	created, err := tc.WasmClient.WasmV1alpha1().WasmModules(
		tc.Namespace,
	).Create(ctx, wm, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create WasmModule: %w", err)
	}

	tc.T.Logf("Created WasmModule: %s", created.Name)

	return created, nil
}

// --- private helpers below (after all exported methods per funcorder) ---

func (tc *TestContext) directHTTPGet(
	ctx context.Context, targetURL string,
) (string, error) {
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

func (tc *TestContext) dumpPods(ctx context.Context) {
	pods, err := tc.KubeClient.CoreV1().Pods(tc.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		tc.T.Logf("Failed to list pods: %v", err)

		return
	}

	tc.T.Logf("Pods in namespace %s:", tc.Namespace)

	for _, pod := range pods.Items {
		tc.T.Logf("  Pod: %s (Phase: %s)", pod.Name, pod.Status.Phase)

		for _, cs := range pod.Status.ContainerStatuses {
			tc.T.Logf("    Container: %s (Ready: %v, Image: %s, ImageID: %s)",
				cs.Name, cs.Ready, cs.Image, cs.ImageID)

			if cs.State.Waiting != nil {
				tc.T.Logf("      Waiting: %s - %s",
					cs.State.Waiting.Reason, cs.State.Waiting.Message)
			}

			if cs.State.Terminated != nil {
				tc.T.Logf("      Terminated: %s - %s (Exit: %d)",
					cs.State.Terminated.Reason,
					cs.State.Terminated.Message,
					cs.State.Terminated.ExitCode)
			}
		}
	}
}

func (tc *TestContext) dumpEvents(ctx context.Context) {
	events, err := tc.KubeClient.CoreV1().Events(tc.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		tc.T.Logf("Failed to list events: %v", err)

		return
	}

	tc.T.Logf("Recent events in namespace %s:", tc.Namespace)

	for _, event := range events.Items {
		tc.T.Logf("  %s: %s %s - %s",
			event.InvolvedObject.Name, event.Type, event.Reason, event.Message)
	}
}

func (tc *TestContext) dumpPodLogs(ctx context.Context) {
	pods, err := tc.KubeClient.CoreV1().Pods(tc.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, pod := range pods.Items {
		for _, container := range []string{"user-container", "queue-proxy"} {
			logs, err := tc.KubeClient.CoreV1().Pods(tc.Namespace).GetLogs(
				pod.Name, &corev1.PodLogOptions{
					Container: container,
					TailLines: int64Ptr(50),
				},
			).Do(ctx).Raw()
			if err != nil {
				tc.T.Logf("Failed to get %s logs for pod %s: %v", container, pod.Name, err)
			} else {
				tc.T.Logf("=== Logs from %s/%s (last 50 lines) ===\n%s",
					pod.Name, container, string(logs))
			}
		}
	}
}

func (tc *TestContext) checkWasmModuleReady(
	ctx context.Context, name string,
) (bool, error) {
	wm, err := tc.WasmClient.WasmV1alpha1().WasmModules(
		tc.Namespace,
	).Get(ctx, name, metav1.GetOptions{})
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
				tc.T.Logf("WasmModule %s not ready yet: %s - %s",
					name, cond.Reason, cond.Message)

				return false, nil
			}
		}
	}

	// Also wait for the URL to be available
	if ready && wm.Status.Address != nil && wm.Status.Address.URL != nil {
		tc.T.Logf("WasmModule %s is ready with URL: %s",
			name, wm.Status.Address.URL.String())

		return true, nil
	}

	if ready {
		tc.T.Logf("WasmModule %s Ready condition is True but URL not yet available", name)
	}

	return false, nil
}

func (tc *TestContext) createEchoDeployment(ctx context.Context) error {
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
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/",
									Port: intstr.FromInt(5678),
								},
							},
							InitialDelaySeconds: 1,
							PeriodSeconds:       1,
							TimeoutSeconds:      1,
							SuccessThreshold:    1,
							FailureThreshold:    3,
						},
					}},
				},
			},
		},
	}

	_, err := tc.KubeClient.AppsV1().Deployments(
		tc.Namespace,
	).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create echo server deployment: %w", err)
	}

	return nil
}

func (tc *TestContext) createEchoService(ctx context.Context) error {
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

	_, err := tc.KubeClient.CoreV1().Services(
		tc.Namespace,
	).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create echo server service: %w", err)
	}

	return nil
}

func (tc *TestContext) waitForEchoDeployment(ctx context.Context) error {
	tc.T.Log("Waiting for echo server to be ready...")

	pollFn := func(ctx context.Context) (bool, error) {
		dep, err := tc.KubeClient.AppsV1().Deployments(
			tc.Namespace,
		).Get(ctx, "echo-server", metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if dep.Status.ReadyReplicas > 0 {
			tc.T.Log("Echo server deployment is ready")

			return true, nil
		}

		return false, nil
	}

	return wait.PollUntilContextTimeout(
		ctx, DefaultPollInterval, DefaultTimeout, true, pollFn,
	)
}

func (tc *TestContext) waitForEchoEndpoints(ctx context.Context) error {
	tc.T.Log("Waiting for echo server endpoints to be ready...")

	pollFn := func(ctx context.Context) (bool, error) {
		endpoints, err := tc.KubeClient.CoreV1().Endpoints(
			tc.Namespace,
		).Get(ctx, "echo-server", metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		for _, subset := range endpoints.Subsets {
			if len(subset.Addresses) > 0 {
				tc.T.Log("Echo server endpoints are ready")

				return true, nil
			}
		}

		return false, nil
	}

	return wait.PollUntilContextTimeout(
		ctx, DefaultPollInterval, 30*time.Second, true, pollFn,
	)
}

func (tc *TestContext) verifyEchoNetworking(ctx context.Context) error {
	tc.T.Log("Verifying echo server service networking with warmup pod...")

	if err := tc.createWarmupPod(ctx); err != nil {
		return err
	}

	err := tc.waitForWarmupPod(ctx)

	// Clean up warmup pod regardless of result
	deletePolicy := metav1.DeletePropagationBackground
	tc.KubeClient.CoreV1().Pods(tc.Namespace).Delete(ctx, "echo-warmup", metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})

	return err
}

func (tc *TestContext) createWarmupPod(ctx context.Context) error {
	echoURL := fmt.Sprintf(
		"http://echo-server.%s.svc.cluster.local:80", tc.Namespace,
	)
	curlCmd := fmt.Sprintf(
		"for i in $(seq 1 120); do if curl -sf --max-time 2 %s;"+
			" then exit 0; fi; sleep 0.25; done; exit 1",
		echoURL,
	)

	warmupPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-warmup",
			Namespace: tc.Namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "curl",
				Image:   "curlimages/curl:latest",
				Command: []string{"sh", "-c"},
				Args:    []string{curlCmd},
			}},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	_, err := tc.KubeClient.CoreV1().Pods(
		tc.Namespace,
	).Create(ctx, warmupPod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create warmup pod: %w", err)
	}

	return nil
}

func (tc *TestContext) waitForWarmupPod(ctx context.Context) error {
	pollFn := func(ctx context.Context) (bool, error) {
		pod, err := tc.KubeClient.CoreV1().Pods(
			tc.Namespace,
		).Get(ctx, "echo-warmup", metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if pod.Status.Phase == corev1.PodSucceeded {
			tc.T.Log("Echo server service networking verified")

			return true, nil
		}

		if pod.Status.Phase == corev1.PodFailed {
			return false, errors.New("warmup pod failed to connect to echo server")
		}

		return false, nil
	}

	return wait.PollUntilContextTimeout(
		ctx, 500*time.Millisecond, 40*time.Second, true, pollFn,
	)
}

func getGatewayInfo() (string, string) {
	ns := "kourier-system"
	if override := os.Getenv("GATEWAY_NAMESPACE_OVERRIDE"); override != "" {
		ns = override
	}

	name := "kourier"
	if override := os.Getenv("GATEWAY_OVERRIDE"); override != "" {
		name = override
	}

	return ns, name
}

func (tc *TestContext) waitForLoadBalancer(
	ctx context.Context,
	gatewayNamespace, gatewayName string,
) (string, error) {
	tc.T.Log("Waiting for LoadBalancer to be ready...")

	var lbAddr string

	pollFn := func(ctx context.Context) (bool, error) {
		svc, err := tc.KubeClient.CoreV1().Services(
			gatewayNamespace,
		).Get(ctx, gatewayName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			return false, nil
		}

		ingress := svc.Status.LoadBalancer.Ingress[0]
		if ingress.IP != "" {
			lbAddr = ingress.IP

			return true, nil
		}

		if ingress.Hostname != "" {
			lbAddr = ingress.Hostname

			return true, nil
		}

		return false, nil
	}

	err := wait.PollUntilContextTimeout(
		ctx, time.Second, 2*time.Minute, true, pollFn,
	)
	if err != nil {
		return "", fmt.Errorf("LoadBalancer not ready after 2 minutes: %w", err)
	}

	tc.T.Logf("Using LoadBalancer IP: %s", lbAddr)

	return lbAddr, nil
}

func (tc *TestContext) getNodePortAddress(
	ctx context.Context,
	svc *corev1.Service,
	gatewayNamespace, gatewayName string,
) (string, error) {
	nodes, err := tc.KubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodes.Items) == 0 {
		return "", errors.New("no nodes found in cluster")
	}

	nodeIP := findNodeInternalIP(nodes.Items[0])
	if nodeIP == "" {
		return "", errors.New("no internal IP found for node")
	}

	for _, port := range svc.Spec.Ports {
		if (port.Name == "http2" || port.Port == 80) && port.NodePort > 0 {
			tc.T.Logf("Using NodePort access: %s:%d", nodeIP, port.NodePort)

			return fmt.Sprintf("%s:%d", nodeIP, port.NodePort), nil
		}
	}

	return "", fmt.Errorf(
		"no suitable port found in gateway service %s/%s",
		gatewayNamespace, gatewayName,
	)
}

func findNodeInternalIP(node corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}

	return ""
}

func (tc *TestContext) httpGetViaIngress(
	ctx context.Context,
	parsed *url.URL,
	ingressAddr string,
) (string, error) {
	requestURL := fmt.Sprintf("http://%s%s", ingressAddr, parsed.RequestURI())
	tc.T.Logf("Making request to %s with Host: %s", requestURL, parsed.Host)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	var (
		lastErr  error
		lastBody string
	)

	retryInterval := 250 * time.Millisecond
	retryTimeout := 2 * time.Minute
	deadline := time.Now().Add(retryTimeout)

	for time.Now().Before(deadline) {
		body, done, err := tc.doIngressRequest(ctx, client, requestURL, parsed)
		if done {
			return body, err
		}

		if err != nil {
			lastErr = err
			lastBody = body
		}

		time.Sleep(retryInterval)
	}

	return lastBody, fmt.Errorf(
		"request timed out after %v, last error: %w", retryTimeout, lastErr,
	)
}

// doIngressRequest performs a single HTTP request through the ingress.
// Returns (body, done, error) where done=true means stop retrying.
func (tc *TestContext) doIngressRequest(
	ctx context.Context,
	client *http.Client,
	requestURL string,
	parsed *url.URL,
) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", true, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Host header to route to the correct Knative service
	req.Host = parsed.Host

	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("failed to perform request: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return "", true, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusOK {
		return string(body), true, nil
	}

	bodyStr := strings.TrimSpace(string(body))
	statusErr := fmt.Errorf(
		"unexpected status code: %d, body: %s", resp.StatusCode, bodyStr,
	)

	// Retry on 404/502/503 (ingress not ready yet or upstream unavailable)
	if resp.StatusCode == http.StatusNotFound ||
		resp.StatusCode == http.StatusBadGateway ||
		resp.StatusCode == http.StatusServiceUnavailable {
		return string(body), false, statusErr
	}

	// For other errors, fail immediately
	return string(body), true, statusErr
}

// int64Ptr returns a pointer to an int64.
func int64Ptr(i int64) *int64 {
	return &i
}

// int32Ptr returns a pointer to an int32.
func int32Ptr(i int32) *int32 {
	return &i
}
