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

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	wasmmoduleinformer "github.com/cardil/knative-serving-wasm/pkg/client/injection/informers/wasm/v1alpha1/wasmmodule"
	wasmmodulereconciler "github.com/cardil/knative-serving-wasm/pkg/client/injection/reconciler/wasm/v1alpha1/wasmmodule"
	svcclient "knative.dev/serving/pkg/client/injection/client"
	svcinformer "knative.dev/serving/pkg/client/injection/informers/serving/v1/service"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(
	ctx context.Context,
	_ configmap.Watcher,
) *controller.Impl {
	log := logging.FromContext(ctx)
	wasmmoduleInformer := wasmmoduleinformer.Get(ctx)
	svcInformer := svcinformer.Get(ctx)

	reconciler := &Reconciler{
		ServiceLister: svcInformer.Lister(),
		Client:        svcclient.Get(ctx).ServingV1(),
	}
	impl := wasmmodulereconciler.NewImpl(ctx, reconciler)
	reconciler.Tracker = impl.Tracker

	if _, err := wasmmoduleInformer.Informer().
		AddEventHandler(controller.HandleAll(impl.Enqueue)); err != nil {
		log.Fatal(err)
	}

	if _, err := svcInformer.Informer().AddEventHandler(controller.HandleAll(
		// Call the tracker's OnChanged method, but we've seen the objects
		// coming through this path missing TypeMeta, so ensure it is properly
		// populated.
		controller.EnsureTypeMeta(
			reconciler.Tracker.OnChanged,
			servingv1.SchemeGroupVersion.WithKind("Service"),
		),
	)); err != nil {
		log.Fatal(err)
	}

	return impl
}
