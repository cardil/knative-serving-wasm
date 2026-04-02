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
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	api "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	"github.com/cardil/knative-serving-wasm/pkg/reconciler/wasmmodule"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/controller"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingfake "knative.dev/serving/pkg/client/clientset/versioned/fake"
)

// TestBuildRunnerConfigMatchesGolden verifies that the JSON produced by
// BuildRunnerConfig matches the documented wire format that the runner parses.
// If this test fails, both the golden file and the runner must be updated together.
func TestBuildRunnerConfigMatchesGolden(t *testing.T) {
	t.Parallel()

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
				AllowIPNameLookup: &trueVal,
				TCP: &api.TCPSpec{
					Connect: []string{"example.com:443"},
				},
			},
		},
	}

	got, err := wasmmodule.BuildRunnerConfig(module)
	if err != nil {
		t.Fatalf("BuildRunnerConfig() error: %v", err)
	}

	goldenBytes, err := os.ReadFile("testdata/wasi_config.golden.json")
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	assertJSONEqual(t, got, string(goldenBytes))
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()

	var gotMap, wantMap any

	if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}

	if err := json.Unmarshal([]byte(want), &wantMap); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}

	gotJSON, err := json.MarshalIndent(gotMap, "", "  ")
	if err != nil {
		t.Fatalf("marshal got normalized JSON: %v", err)
	}

	wantJSON, err := json.MarshalIndent(wantMap, "", "  ")
	if err != nil {
		t.Fatalf("marshal want normalized JSON: %v", err)
	}

	if string(gotJSON) != string(wantJSON) {
		t.Errorf("BuildRunnerConfig output does not match golden.\ngot:\n%s\n\nwant:\n%s", gotJSON, wantJSON)
	}
}

// buildRunnerConfigStore seeds a RunnerConfigStore with the given insecure registries.
func buildRunnerConfigStore(registries []string) *wasmmodule.RunnerConfigStore {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: wasmmodule.RunnerConfigName},
		Data:       map[string]string{},
	}

	if len(registries) > 0 {
		var sb strings.Builder
		for _, r := range registries {
			sb.WriteString("- ")
			sb.WriteString(r)
			sb.WriteString("\n")
		}

		cm.Data["insecure-registries"] = sb.String()
	}

	store := wasmmodule.NewRunnerConfigStore(noopLogger{})
	store.OnConfigChanged(cm)

	return store
}

// noopLogger satisfies configmap.Logger for unit tests.
type noopLogger struct{}

func (noopLogger) Debugf(string, ...interface{}) {}
func (noopLogger) Infof(string, ...interface{})  {}
func (noopLogger) Errorf(string, ...interface{}) {}
func (noopLogger) Fatalf(string, ...interface{}) {}

// buildReconcilerCtx returns a context with a fake event recorder attached.
func buildReconcilerCtx() context.Context {
	return controller.WithEventRecorder(context.Background(), record.NewFakeRecorder(100))
}

// TestCreateService_InsecureRegistries_Injected checks that INSECURE_REGISTRIES
// env var is set on the runner container when the config store has entries.
func TestCreateService_InsecureRegistries_Injected(t *testing.T) {
	t.Parallel()

	const ns = "default"

	const moduleName = "my-wasm"

	fakeClient := servingfake.NewSimpleClientset()
	store := buildRunnerConfigStore([]string{"registry.local:5000", "my-reg.internal:5000"})

	r := &wasmmodule.Reconciler{
		Tracker:           fakeTracker{},
		ServiceLister:     buildServiceLister(),
		Client:            fakeClient.ServingV1(),
		RunnerConfigStore: store,
	}

	module := &api.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      moduleName,
			Namespace: ns,
		},
		Spec: api.WasmModuleSpec{
			Image: "ghcr.io/example/module:latest",
		},
	}
	module.Status.InitializeConditions()

	ctx := buildReconcilerCtx()
	if err := r.ReconcileKind(ctx, module); err != nil {
		t.Fatalf("ReconcileKind() error: %v", err)
	}

	svcs, err := fakeClient.ServingV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list services: %v", err)
	}

	if len(svcs.Items) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs.Items))
	}

	containers := svcs.Items[0].Spec.Template.Spec.Containers
	if len(containers) == 0 {
		t.Fatal("expected at least one container")
	}

	envMap := make(map[string]string)
	for _, e := range containers[0].Env {
		envMap[e.Name] = e.Value
	}

	got, ok := envMap["INSECURE_REGISTRIES"]
	if !ok {
		t.Fatal("expected INSECURE_REGISTRIES env var to be set")
	}

	if got != "registry.local:5000,my-reg.internal:5000" {
		t.Errorf("INSECURE_REGISTRIES = %q, want %q", got, "registry.local:5000,my-reg.internal:5000")
	}
}

// TestCreateService_InsecureRegistries_Absent checks that INSECURE_REGISTRIES
// is NOT set when the config store has no entries.
func TestCreateService_InsecureRegistries_Absent(t *testing.T) {
	t.Parallel()

	const ns = "default"

	const moduleName = "my-wasm-clean"

	fakeClient := servingfake.NewSimpleClientset()
	store := buildRunnerConfigStore(nil)

	r := &wasmmodule.Reconciler{
		Tracker:           fakeTracker{},
		ServiceLister:     buildServiceLister(),
		Client:            fakeClient.ServingV1(),
		RunnerConfigStore: store,
	}

	module := &api.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      moduleName,
			Namespace: ns,
		},
		Spec: api.WasmModuleSpec{
			Image: "ghcr.io/example/module:latest",
		},
	}
	module.Status.InitializeConditions()

	ctx := buildReconcilerCtx()
	if err := r.ReconcileKind(ctx, module); err != nil {
		t.Fatalf("ReconcileKind() error: %v", err)
	}

	svcs, err := fakeClient.ServingV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list services: %v", err)
	}

	if len(svcs.Items) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs.Items))
	}

	containers := svcs.Items[0].Spec.Template.Spec.Containers
	if len(containers) == 0 {
		t.Fatal("expected at least one container")
	}

	for _, e := range containers[0].Env {
		if e.Name == "INSECURE_REGISTRIES" {
			t.Errorf("expected INSECURE_REGISTRIES to be absent, but got %q", e.Value)
		}
	}
}

// buildOldService creates a fake Knative Service with the given old image env var.
func buildOldService(ns, name, oldImage string) *servingv1.Service {
	return &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			ResourceVersion: "1",
		},
		Spec: servingv1.ServiceSpec{
			ConfigurationSpec: servingv1.ConfigurationSpec{
				Template: servingv1.RevisionTemplateSpec{
					Spec: servingv1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: wasmmodule.DefaultRunnerImage,
									Env: []corev1.EnvVar{
										{Name: "IMAGE", Value: oldImage},
										{Name: "WASI_CONFIG", Value: "{}"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// getEnvMap extracts env vars from the first container of a Service as a map.
func getEnvMap(t *testing.T, svc *servingv1.Service) map[string]string {
	t.Helper()

	containers := svc.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		t.Fatal("expected at least one container")
	}

	envMap := make(map[string]string)
	for _, e := range containers[0].Env {
		envMap[e.Name] = e.Value
	}

	return envMap
}

// TestUpdateService_SpecChanged verifies that when a WasmModule spec changes
// (e.g. Image field), the reconciler updates the existing Knative Service.
func TestUpdateService_SpecChanged(t *testing.T) {
	t.Parallel()

	const ns = "default"

	const moduleName = "my-wasm-update"

	// Seed the fake client with an existing Service that has the old IMAGE env var.
	existingSvc := buildOldService(ns, moduleName, "example.com/old-image:v1")
	fakeClient := servingfake.NewSimpleClientset(existingSvc)

	r := &wasmmodule.Reconciler{
		Tracker:       fakeTracker{},
		ServiceLister: buildServiceLister(existingSvc),
		Client:        fakeClient.ServingV1(),
	}

	// Module spec now references a new image.
	module := &api.WasmModule{
		ObjectMeta: metav1.ObjectMeta{Name: moduleName, Namespace: ns},
		Spec:       api.WasmModuleSpec{Image: "example.com/new-image:v2"},
	}
	module.Status.InitializeConditions()

	ctx := buildReconcilerCtx()
	if err := r.ReconcileKind(ctx, module); err != nil {
		t.Fatalf("ReconcileKind() error: %v", err)
	}

	// Verify the Service was updated with the new image in the IMAGE env var.
	svc, err := fakeClient.ServingV1().Services(ns).Get(ctx, moduleName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service: %v", err)
	}

	envMap := getEnvMap(t, svc)

	if got := envMap["IMAGE"]; got != "example.com/new-image:v2" {
		t.Errorf("IMAGE env var = %q, want %q", got, "example.com/new-image:v2")
	}
}

// buildMatchingService builds a Knative Service whose spec exactly matches what the reconciler
// would produce for the given WasmModule (i.e., no drift).
func buildMatchingService(t *testing.T, ns, name string, module *api.WasmModule) *servingv1.Service {
	t.Helper()

	wasiConfig, err := wasmmodule.BuildRunnerConfig(module)
	if err != nil {
		t.Fatalf("BuildRunnerConfig: %v", err)
	}

	return &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, ResourceVersion: "1"},
		Spec: servingv1.ServiceSpec{
			ConfigurationSpec: servingv1.ConfigurationSpec{
				Template: servingv1.RevisionTemplateSpec{
					Spec: servingv1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: wasmmodule.DefaultRunnerImage,
									Env: []corev1.EnvVar{
										{Name: "IMAGE", Value: module.Spec.Image},
										{Name: "WASI_CONFIG", Value: wasiConfig},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// TestUpdateService_SpecUnchanged verifies that when the WasmModule spec hasn't
// changed, the reconciler does NOT call Update (no unnecessary API calls).
func TestUpdateService_SpecUnchanged(t *testing.T) {
	t.Parallel()

	const ns = "default"

	const moduleName = "my-wasm-noop"

	module := &api.WasmModule{
		ObjectMeta: metav1.ObjectMeta{Name: moduleName, Namespace: ns},
		Spec:       api.WasmModuleSpec{Image: "example.com/img:latest"},
	}
	module.Status.InitializeConditions()

	ctx := buildReconcilerCtx()

	existingSvc := buildMatchingService(t, ns, moduleName, module)
	fakeClient := servingfake.NewSimpleClientset(existingSvc)

	r := &wasmmodule.Reconciler{
		Tracker:       fakeTracker{},
		ServiceLister: buildServiceLister(existingSvc),
		Client:        fakeClient.ServingV1(),
	}

	if err := r.ReconcileKind(ctx, module); err != nil {
		t.Fatalf("ReconcileKind() error: %v", err)
	}

	// Inspect fake client actions to confirm Update was NOT called.
	for _, a := range fakeClient.Actions() {
		if a.GetVerb() == "update" && a.GetResource().Resource == "services" {
			t.Errorf("expected no Update call when spec is unchanged, but got one: %v", a)
		}
	}
}
