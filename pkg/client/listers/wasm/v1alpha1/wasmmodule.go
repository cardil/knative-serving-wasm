/*
Copyright 2020 The Knative Authors

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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// WasmModuleLister helps list WasmModules.
// All objects returned here must be treated as read-only.
type WasmModuleLister interface {
	// List lists all WasmModules in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.WasmModule, err error)
	// WasmModules returns an object that can list and get WasmModules.
	WasmModules(namespace string) WasmModuleNamespaceLister
	WasmModuleListerExpansion
}

// wasmModuleLister implements the WasmModuleLister interface.
type wasmModuleLister struct {
	indexer cache.Indexer
}

// NewWasmModuleLister returns a new WasmModuleLister.
func NewWasmModuleLister(indexer cache.Indexer) WasmModuleLister {
	return &wasmModuleLister{indexer: indexer}
}

// List lists all WasmModules in the indexer.
func (s *wasmModuleLister) List(selector labels.Selector) (ret []*v1alpha1.WasmModule, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.WasmModule))
	})
	return ret, err
}

// WasmModules returns an object that can list and get WasmModules.
func (s *wasmModuleLister) WasmModules(namespace string) WasmModuleNamespaceLister {
	return wasmModuleNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// WasmModuleNamespaceLister helps list and get WasmModules.
// All objects returned here must be treated as read-only.
type WasmModuleNamespaceLister interface {
	// List lists all WasmModules in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.WasmModule, err error)
	// Get retrieves the WasmModule from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.WasmModule, error)
	WasmModuleNamespaceListerExpansion
}

// wasmModuleNamespaceLister implements the WasmModuleNamespaceLister
// interface.
type wasmModuleNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all WasmModules in the indexer for a given namespace.
func (s wasmModuleNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.WasmModule, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.WasmModule))
	})
	return ret, err
}

// Get retrieves the WasmModule from the indexer for a given namespace and name.
func (s wasmModuleNamespaceLister) Get(name string) (*v1alpha1.WasmModule, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("wasmmodule"), name)
	}
	return obj.(*v1alpha1.WasmModule), nil
}
