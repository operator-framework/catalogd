/*
Copyright 2022.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: The source types, reason, etc. are all copy/pasted from the rukpak
//   repository. We should look into whether it is possible to share these.

type SourceType string

const (
	SourceTypeImage SourceType = "image"

	TypeUnpacked = "Unpacked"

	ReasonUnpackPending    = "UnpackPending"
	ReasonUnpacking        = "Unpacking"
	ReasonUnpackSuccessful = "UnpackSuccessful"
	ReasonUnpackFailed     = "UnpackFailed"

	PhasePending   = "Pending"
	PhaseUnpacking = "Unpacking"
	PhaseFailing   = "Failing"
	PhaseUnpacked  = "Unpacked"
)

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:subresource:status

// Catalog is the Schema for the Catalogs API
type Catalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CatalogSpec   `json:"spec,omitempty"`
	Status CatalogStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CatalogList contains a list of Catalog
type CatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Catalog `json:"items"`
}

// CatalogSpec defines the desired state of Catalog
type CatalogSpec struct {
	// Source is the source of a Catalog that contains Operators' metadata in the FBC format
	// https://olm.operatorframework.io/docs/reference/file-based-catalogs/#docs
	Source CatalogSource `json:"source"`
}

// CatalogStatus defines the observed state of Catalog
type CatalogStatus struct {
	// Conditions store the status conditions of the Catalog instances
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	ResolvedSource *CatalogSource `json:"resolvedSource,omitempty"`
	Phase          string         `json:"phase,omitempty"`
}

// CatalogSource contains the sourcing information for a Catalog
type CatalogSource struct {
	// Type defines the kind of Catalog content being sourced.
	Type SourceType `json:"type"`
	// Image is the catalog image that backs the content of this catalog.
	Image *ImageSource `json:"image,omitempty"`
}

// ImageSource contains information required for sourcing a Catalog from an OCI image
type ImageSource struct {
	// Ref contains the reference to a container image containing Catalog contents.
	Ref string `json:"ref"`
	// PullSecret contains the name of the image pull secret in the namespace that catalogd is deployed.
	PullSecret string `json:"pullSecret,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Catalog{}, &CatalogList{})
}
