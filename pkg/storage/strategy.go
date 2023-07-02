package storage

import (
	"context"
	"fmt"

	"github.com/operator-framework/catalogd/api/optional/v1alpha1"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
)

// NewStrategy creates and returns a Strategy instance
func NewStrategy(typer runtime.ObjectTyper) Strategy {
	return Strategy{typer, names.SimpleNameGenerator}
}

// GetAttrs returns labels.Set, fields.Set, the presence of Initializers if any
// and error in case the given runtime.Object is not a CatalogMetadata
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	catalogmetadata, ok := obj.(*v1alpha1.CatalogMetadata)
	if !ok {
		return nil, nil, fmt.Errorf("given object is not a CatalogMetadata")
	}
	return labels.Set(catalogmetadata.ObjectMeta.Labels), SelectableFields(catalogmetadata), nil
}

// MatchCatalogMetadta is the filter used by the generic etcd backend to watch events
// from etcd to clients of the apiserver only interested in specific labels/fields.
func MatchCatalogMetadata(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

// SelectableFields returns a field set that represents the object.
func SelectableFields(obj *v1alpha1.CatalogMetadata) fields.Set {
	return generic.ObjectMetaFieldsSet(&obj.ObjectMeta, true)
}

type Strategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

func (Strategy) NamespaceScoped() bool {
	return false
}

func (Strategy) PrepareForCreate(_ context.Context, _ runtime.Object) {
}

func (Strategy) PrepareForUpdate(_ context.Context, _, _ runtime.Object) {
}

func (Strategy) Canonicalize(_ runtime.Object) {
}

func (Strategy) AllowCreateOnUpdate() bool {
	return false
}

func (Strategy) AllowUnconditionalUpdate() bool {
	return false
}

func (Strategy) Validate(_ context.Context, _ runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

func (Strategy) WarningsOnCreate(_ context.Context, _ runtime.Object) []string {
	return []string{}
}

func (Strategy) ValidateUpdate(_ context.Context, _, _ runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

func (Strategy) WarningsOnUpdate(_ context.Context, _, _ runtime.Object) []string {
	return []string{}
}
