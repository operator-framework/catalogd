package webhook

import (
	"context"
	"fmt"
	"github.com/operator-framework/catalogd/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ClusterCatalog wraps the external v1alpha1.ClusterCatalog type and implements admission.Defaulter
type ClusterCatalog struct{}

// Default is the method that will be called by the webhook to apply defaults.
func (r *ClusterCatalog) Default(ctx context.Context, obj runtime.Object) error {
	log := log.FromContext(ctx)
	log.Info("Invoking Default method for ClusterCatalog", "object", obj)
	catalog, ok := obj.(*v1alpha1.ClusterCatalog)
	if !ok {
		return fmt.Errorf("expected a ClusterCatalog but got a %T", obj)
	}

	// Defaulting logic: add the "name" label with the value set to the object's Name
	if catalog.Labels == nil {
		catalog.Labels = map[string]string{}
	}
	catalog.Labels["name"] = catalog.GetName()
	log.Info("default", "name", catalog.Name, "labels", catalog.Labels)

	return nil
}

// SetupWebhookWithManager sets up the webhook with the manager
func (r *ClusterCatalog) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.ClusterCatalog{}).
		WithDefaulter(r).
		Complete()
}
