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

package core

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	corev1beta1 "github.com/operator-framework/catalogd/pkg/apis/core/v1beta1"
	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// CatalogReconciler reconciles a Catalog object
type CatalogReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Cfg      *rest.Config
	OpmImage string
}

//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=catalogs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=catalogs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=catalogs/finalizers,verbs=update
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=bundlemetadata,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=bundlemetadata/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=bundlemetadata/finalizers,verbs=update
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=packages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=packages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=packages/finalizers,verbs=update
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=list;watch;update;patch;create;
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/finalizers,verbs=update
//+kubebuilder:rbac:verbs=get,urls=/bundles/*;/uploads/*
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;watch;create;delete;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=list;watch
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *CatalogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: Where and when should we be logging errors and at which level?
	_ = log.FromContext(ctx).WithName("catalogd-controller")

	existingCatsrc := corev1beta1.Catalog{}
	if err := r.Client.Get(ctx, req.NamespacedName, &existingCatsrc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconciledCatsrc := existingCatsrc.DeepCopy()
	res, reconcileErr := r.reconcile(ctx, reconciledCatsrc)

	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	if !equality.Semantic.DeepEqual(existingCatsrc.Status, reconciledCatsrc.Status) {
		if updateErr := r.Client.Status().Update(ctx, reconciledCatsrc); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	existingCatsrc.Status, reconciledCatsrc.Status = corev1beta1.CatalogStatus{}, corev1beta1.CatalogStatus{}
	if !equality.Semantic.DeepEqual(existingCatsrc, reconciledCatsrc) {
		if updateErr := r.Client.Update(ctx, reconciledCatsrc); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

// SetupWithManager sets up the controller with the Manager.
func (r *CatalogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// TODO: Due to us not having proper error handling,
		// not having this results in the controller getting into
		// an error state because once we update the status it requeues
		// and then errors out when trying to create all the Packages again
		// even though they already exist. This should be resolved by the fix
		// for https://github.com/operator-framework/catalogd/issues/6. The fix for
		// #6 should also remove the usage of `builder.WithPredicates(predicate.GenerationChangedPredicate{})`
		For(&corev1beta1.Catalog{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&rukpakv1alpha1.Bundle{}).
		Complete(r)
}

func (r *CatalogReconciler) reconcile(ctx context.Context, catalog *corev1beta1.Catalog) (ctrl.Result, error) {
	bundle, err := r.ensureBundle(ctx, catalog)
	if err != nil {
		updateStatusError(catalog, err)
		return ctrl.Result{}, err
	}

	if bundle == nil {
		return ctrl.Result{}, fmt.Errorf("%s", "nil bundle!!")
	}
	cond := meta.FindStatusCondition(bundle.Status.Conditions, rukpakv1alpha1.TypeUnpacked)
	if cond == nil || cond != nil && cond.Status != metav1.ConditionTrue {
		return ctrl.Result{}, nil
	}

	// update Catalog status as "Ready" since at this point
	// all catalog content should be available on cluster
	updateStatusReady(catalog)
	return ctrl.Result{}, nil
}

// updateStatusReady will update the Catalog.Status.Conditions
// to have the "Ready" condition with a status of "True" and a Reason
// of "ContentsAvailable". This function is used to signal that a Catalog
// has been successfully unpacked and all catalog contents are available on cluster
func updateStatusReady(catalog *corev1beta1.Catalog) {
	meta.SetStatusCondition(&catalog.Status.Conditions, metav1.Condition{
		Type:    corev1beta1.TypeReady,
		Reason:  corev1beta1.ReasonContentsAvailable,
		Status:  metav1.ConditionTrue,
		Message: "catalog contents have been unpacked and are available on cluster",
	})
}

// updateStatusError will update the Catalog.Status.Conditions
// to have the condition Type "Ready" with a Status of "False" and a Reason
// of "UnpackError". This function is used to signal that a Catalog
// is in an error state and that catalog contents are not available on cluster
func updateStatusError(catalog *corev1beta1.Catalog, err error) {
	meta.SetStatusCondition(&catalog.Status.Conditions, metav1.Condition{
		Type:    corev1beta1.TypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  corev1beta1.ReasonUnpackError,
		Message: err.Error(),
	})
}

func (r *CatalogReconciler) ensureBundle(ctx context.Context, catalog *corev1beta1.Catalog) (*rukpakv1alpha1.Bundle, error) {
	bundle := &rukpakv1alpha1.Bundle{
		ObjectMeta: metav1.ObjectMeta{
			Name: catalog.Name + "-bundle",
		},
		Spec: rukpakv1alpha1.BundleSpec{
			ProvisionerClassName: "catalogd-bundle-provisioner",
			Source: rukpakv1alpha1.BundleSource{
				Type: rukpakv1alpha1.SourceTypeImage,
				Image: &rukpakv1alpha1.ImageSource{
					Ref: catalog.Spec.Image,
				},
			},
		},
	}

	if err := ctrlutil.SetOwnerReference(catalog, bundle, r.Client.Scheme()); err != nil {
		return nil, fmt.Errorf("setting ownerref on bundle %q: %w", bundle.Name, err)
	}

	if err := r.Client.Create(ctx, bundle); err != nil {
		if errors.IsAlreadyExists(err) {
			err = r.Client.Get(ctx, client.ObjectKeyFromObject(bundle), bundle)
			if err != nil {
				return nil, err
			}
			return bundle, nil
		}
		return nil, err
	}

	return bundle, nil
}
