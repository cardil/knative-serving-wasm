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

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	corev1listers "k8s.io/client-go/listers/core/v1"

	api "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	apireconciler "github.com/cardil/knative-serving-wasm/pkg/client/injection/reconciler/wasm/v1alpha1/wasmmodule"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/network"
	"knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
)

// Reconciler implements apireconciler.Interface for
// WasmModule resources.
type Reconciler struct {
	// Tracker builds an index of what resources are watching other resources
	// so that we can immediately react to changes tracked resources.
	Tracker tracker.Interface

	// Listers index properties about resources
	ServiceLister corev1listers.ServiceLister
}

// Check that our Reconciler implements Interface.
var _ apireconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, module *api.WasmModule) reconciler.Event {
	logger := logging.FromContext(ctx)

	err := r.Tracker.TrackReference(tracker.Reference{
		APIVersion: "v1",
		Kind:       "Service",
		Name:       module.Spec.ServiceName,
		Namespace:  module.Namespace,
	}, module)
	if err != nil {
		logger.Errorf("Error tracking service %s: %v", module.Spec.ServiceName, err)

		return err
	}

	if _, err := r.ServiceLister.Services(module.Namespace).Get(module.Spec.ServiceName); apierrs.IsNotFound(err) {
		logger.Info("Service does not yet exist:", module.Spec.ServiceName)
		module.Status.MarkServiceUnavailable(module.Spec.ServiceName)

		return nil
	} else if err != nil {
		logger.Errorf("Error reconciling service %s: %v", module.Spec.ServiceName, err)

		return err
	}

	module.Status.MarkServiceAvailable()
	module.Status.Address = &duckv1.Addressable{
		URL: &apis.URL{
			Scheme: "http",
			Host:   network.GetServiceHostname(module.Spec.ServiceName, module.Namespace),
		},
	}

	return nil
}
