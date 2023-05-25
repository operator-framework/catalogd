/*
Copyright 2023.

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

package v1beta1

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// CatalogMetadataSpec defines the desired state of CatalogMetadata
type CatalogMetadataSpec struct {
	Catalog v1.LocalObjectReference `json:"catalog"`
	Schema  string                  `json:"schema"`
	Package string                  `json:"package,omitempty"`
	Name    string                  `json:"name,omitempty"`
	Content json.RawMessage         `json:"content,omitempty"`
}

//+kubebuilder:object:root=true

// CatalogMetadata is the Schema for the catalogs API
type CatalogMetadata struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CatalogMetadataSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// CatalogMetadataList contains a list of CatalogMetadata
type CatalogMetadataList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CatalogMetadata `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CatalogMetadata{}, &CatalogMetadataList{})
}

func (_ CatalogMetadata) NamespaceScoped() bool {
	return false
}

func (_ CatalogMetadata) New() runtime.Object {
	return &CatalogMetadata{}
}

func (_ CatalogMetadata) NewList() runtime.Object {
	return &CatalogMetadataList{}
}

func (_ CatalogMetadata) GetGroupVersionResource() schema.GroupVersionResource {
	return GroupVersion.WithResource("catalogmetadatas")
}

func (_ CatalogMetadata) IsStorageVersion() bool {
	return true
}

func (c *CatalogMetadata) GetObjectMeta() *metav1.ObjectMeta {
	return &c.ObjectMeta
}
