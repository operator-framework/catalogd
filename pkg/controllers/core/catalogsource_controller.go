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

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/operator-framework/catalogd/internal/source"
	corev1beta1 "github.com/operator-framework/catalogd/pkg/apis/core/v1beta1"
)

// CatalogSourceReconciler reconciles a CatalogSource object
type CatalogSourceReconciler struct {
	client.Client

	Unpacker source.Unpacker
}

//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=catalogsources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=catalogsources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=catalogsources/finalizers,verbs=update
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=bundlemetadata,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=bundlemetadata/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=bundlemetadata/finalizers,verbs=update
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=packages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=packages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=catalogd.operatorframework.io,resources=packages/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=create;update;patch;delete;get;list;watch
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *CatalogSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: Where and when should we be logging errors and at which level?
	_ = log.FromContext(ctx).WithName("catalogd-controller")

	existingCatsrc := corev1beta1.CatalogSource{}
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
	existingCatsrc.Status, reconciledCatsrc.Status = corev1beta1.CatalogSourceStatus{}, corev1beta1.CatalogSourceStatus{}
	if !equality.Semantic.DeepEqual(existingCatsrc, reconciledCatsrc) {
		if updateErr := r.Client.Update(ctx, reconciledCatsrc); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

// SetupWithManager sets up the controller with the Manager.
func (r *CatalogSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// TODO: Due to us not having proper error handling,
		// not having this results in the controller getting into
		// an error state because once we update the status it requeues
		// and then errors out when trying to create all the Packages again
		// even though they already exist. This should be resolved by the fix
		// for https://github.com/operator-framework/catalogd/issues/6. The fix for
		// #6 should also remove the usage of `builder.WithPredicates(predicate.GenerationChangedPredicate{})`
		For(&corev1beta1.CatalogSource{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func (r *CatalogSourceReconciler) reconcile(ctx context.Context, catalogSource *corev1beta1.CatalogSource) (ctrl.Result, error) {
	unpackResult, err := r.Unpacker.Unpack(ctx, catalogSource)
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&catalogSource.Status, fmt.Errorf("source bundle content: %v", err))
	}

	switch unpackResult.State {
	case source.StatePending:
		updateStatusUnpackPending(&catalogSource.Status, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacking:
		updateStatusUnpacking(&catalogSource.Status, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacked:
		// TODO: We should check to see if the unpacked result has the same content
		//   as the already unpacked content. If it does, we should skip this rest
		//   of the unpacking steps.
		fbc, err := declcfg.LoadFS(unpackResult.FS)
		if err != nil {
			return ctrl.Result{}, updateStatusUnpackFailing(&catalogSource.Status, fmt.Errorf("load FBC from filesystem: %v", err))
		}

		if err := r.createPackages(ctx, fbc, catalogSource); err != nil {
			return ctrl.Result{}, updateStatusUnpackFailing(&catalogSource.Status, fmt.Errorf("create package objects: %v", err))
		}

		if err := r.createBundleMetadata(ctx, fbc, catalogSource); err != nil {
			return ctrl.Result{}, updateStatusUnpackFailing(&catalogSource.Status, fmt.Errorf("create bundle metadata objects: %v", err))
		}

		updateStatusUnpacked(&catalogSource.Status, unpackResult)
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, updateStatusUnpackFailing(&catalogSource.Status, fmt.Errorf("unknown unpack state %q: %v", unpackResult.State, err))
	}

}

func updateStatusUnpackPending(status *corev1beta1.CatalogSourceStatus, result *source.Result) {
	status.ResolvedSource = nil
	status.Phase = corev1beta1.PhasePending
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    corev1beta1.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  corev1beta1.ReasonUnpackPending,
		Message: result.Message,
	})
}

func updateStatusUnpacking(status *corev1beta1.CatalogSourceStatus, result *source.Result) {
	status.ResolvedSource = nil
	status.Phase = corev1beta1.PhaseUnpacking
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    corev1beta1.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  corev1beta1.ReasonUnpacking,
		Message: result.Message,
	})
}

func updateStatusUnpacked(status *corev1beta1.CatalogSourceStatus, result *source.Result) {
	status.ResolvedSource = result.ResolvedSource
	status.Phase = corev1beta1.PhaseUnpacked
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    corev1beta1.TypeUnpacked,
		Status:  metav1.ConditionTrue,
		Reason:  corev1beta1.ReasonUnpackSuccessful,
		Message: result.Message,
	})
}

func updateStatusUnpackFailing(status *corev1beta1.CatalogSourceStatus, err error) error {
	status.ResolvedSource = nil
	status.Phase = corev1beta1.PhaseFailing
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    corev1beta1.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  corev1beta1.ReasonUnpackFailed,
		Message: err.Error(),
	})
	return err
}

// createBundleMetadata will create a `BundleMetadata` resource for each
// "olm.bundle" object that exists for the given catalog contents. Returns an
// error if any are encountered.
func (r *CatalogSourceReconciler) createBundleMetadata(ctx context.Context, declCfg *declcfg.DeclarativeConfig, catalogSource *corev1beta1.CatalogSource) error {
	for _, bundle := range declCfg.Bundles {
		bundleMeta := corev1beta1.BundleMetadata{
			ObjectMeta: metav1.ObjectMeta{
				Name: bundle.Name,
			},
			Spec: corev1beta1.BundleMetadataSpec{
				CatalogSource: catalogSource.Name,
				Package:       bundle.Package,
				Image:         bundle.Image,
				Properties:    []corev1beta1.Property{},
				RelatedImages: []corev1beta1.RelatedImage{},
			},
		}

		for _, relatedImage := range bundle.RelatedImages {
			bundleMeta.Spec.RelatedImages = append(bundleMeta.Spec.RelatedImages, corev1beta1.RelatedImage{
				Name:  relatedImage.Name,
				Image: relatedImage.Image,
			})
		}

		for _, prop := range bundle.Properties {
			// skip any properties that are of type `olm.bundle.object`
			if prop.Type == "olm.bundle.object" {
				continue
			}

			bundleMeta.Spec.Properties = append(bundleMeta.Spec.Properties, corev1beta1.Property{
				Type:  prop.Type,
				Value: runtime.RawExtension{Raw: prop.Value},
			})
		}

		if err := ctrlutil.SetControllerReference(catalogSource, &bundleMeta, r.Client.Scheme()); err != nil {
			return fmt.Errorf("set controller reference on bundlemetadata %q: %v", bundleMeta.Name, err)
		}

		if err := r.Client.Create(ctx, &bundleMeta); err != nil {
			return fmt.Errorf("creating bundlemetadata %q: %w", bundleMeta.Name, err)
		}
	}

	return nil
}

// createPackages will create a `Package` resource for each
// "olm.package" object that exists for the given catalog contents.
// `Package.Spec.Channels` is populated by filtering all "olm.channel" objects
// where the "packageName" == `Package.Name`. Returns an error if any are encountered.
func (r *CatalogSourceReconciler) createPackages(ctx context.Context, declCfg *declcfg.DeclarativeConfig, catalogSource *corev1beta1.CatalogSource) error {
	for _, pkg := range declCfg.Packages {
		pack := corev1beta1.Package{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: If we just provide the name of the package, then
				// we are inherently saying no other catalog sources can provide a package
				// of the same name due to this being a cluster scoped resource. We should
				// look into options for configuring admission criteria for the Package
				// resource to resolve this potential clash.
				Name: pkg.Name,
			},
			Spec: corev1beta1.PackageSpec{
				CatalogSource:  catalogSource.Name,
				DefaultChannel: pkg.DefaultChannel,
				Channels:       []corev1beta1.PackageChannel{},
				Description:    pkg.Description,
			},
		}
		for _, ch := range declCfg.Channels {
			if ch.Package == pkg.Name {
				packChannel := corev1beta1.PackageChannel{
					Name:    ch.Name,
					Entries: []corev1beta1.ChannelEntry{},
				}
				for _, entry := range ch.Entries {
					packChannel.Entries = append(packChannel.Entries, corev1beta1.ChannelEntry{
						Name:      entry.Name,
						Replaces:  entry.Replaces,
						Skips:     entry.Skips,
						SkipRange: entry.SkipRange,
					})
				}

				pack.Spec.Channels = append(pack.Spec.Channels, packChannel)
			}
		}

		if err := ctrlutil.SetControllerReference(catalogSource, &pack, r.Client.Scheme()); err != nil {
			return fmt.Errorf("set controller reference on package %q: %v", pack.Name, err)
		}

		if err := r.Client.Create(ctx, &pack); err != nil {
			return fmt.Errorf("creating package %q: %w", pack.Name, err)
		}
	}
	return nil
}
