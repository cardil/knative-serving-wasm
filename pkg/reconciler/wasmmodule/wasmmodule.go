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

	api "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	apireconciler "github.com/cardil/knative-serving-wasm/pkg/client/injection/reconciler/wasm/v1alpha1/wasmmodule"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
	servingv1listers "knative.dev/serving/pkg/client/listers/serving/v1"
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

	if err := r.Tracker.TrackReference(tracker.Reference{
		APIVersion: servingv1.SchemeGroupVersion.String(),
		Kind:       "Service",
		Name:       module.Spec.ServiceName,
		Namespace:  module.Namespace,
	}, module); err != nil {
		log.Errorf("Error tracking service %s: %v", module.Spec.ServiceName, err)

		return err
	}

	srv, err := r.ServiceLister.Services(module.Namespace).Get(module.Spec.ServiceName)

	if apierrs.IsNotFound(err) {
		log.Info("Service does not exist. Creating: ", module.Spec.ServiceName)

		if srv, err = r.createService(ctx, module); err != nil {
			module.Status.MarkServiceUnavailable(module.Spec.ServiceName)

			return err
		}
	} else if err != nil {
		log.Errorf("Error reconciling service %s: %v", module.Spec.ServiceName, err)

		return err
	}

	module.Status.MarkServiceAvailable()
	module.Status.Address = srv.Status.Address.DeepCopy()

	return nil
}

func (r *Reconciler) createService(ctx context.Context, module *api.WasmModule) (*servingv1.Service, error) {
	log := logging.FromContext(ctx)

	srv, err := r.Client.Services(module.Namespace).Create(ctx, &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      module.Spec.ServiceName,
			Namespace: module.Namespace,
		},
		Spec: servingv1.ServiceSpec{
			ConfigurationSpec: servingv1.ConfigurationSpec{
				Template: servingv1.RevisionTemplateSpec{
					Spec: servingv1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Image: "quay.io/cardil/knative/serving/wasm/runner",
								Env: []corev1.EnvVar{{
									Name:  "IMAGE",
									Value: module.Spec.Source.Image,
								}},
							}},
						},
					},
				},
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		log.Errorf("Error creating kservice %s: %v", module.Spec.ServiceName, err)
	}

	return srv, err
}
