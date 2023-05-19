package provisioner

import (
	"context"
	"fmt"
	"io/fs"

	corev1beta1 "github.com/operator-framework/catalogd/pkg/apis/core/v1beta1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/provisioner/bundle"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ bundle.Handler = &CatalogdBundleHandler{}

type CatalogdBundleHandler struct {
	Client client.Client
}

func (cbh *CatalogdBundleHandler) Handle(ctx context.Context, fsys fs.FS, bundle *rukpakv1alpha1.Bundle) (fs.FS, error) {
	fmt.Println("XXX Loading FBC from FS")
	dcfg, err := declcfg.LoadFS(fsys)
	if err != nil {
		return nil, err
	}

	// TODO: This should probably be handled by a bundle deployment provisioner .... but I'm making the rules for now
	fmt.Println("XXX Creating Packages from FBC")
	if err := cbh.createPackages(ctx, dcfg, bundle); err != nil {
		return nil, err
	}

	fmt.Println("XXX Creating BundleMetadata from FBC")
	if err := cbh.createBundleMetadata(ctx, dcfg, bundle); err != nil {
		return nil, err
	}

	fmt.Println("XXX Returning!")
	return fsys, nil
}

func (cbh *CatalogdBundleHandler) createBundleMetadata(ctx context.Context, declCfg *declcfg.DeclarativeConfig, bdl *rukpakv1alpha1.Bundle) error {
	for _, bundle := range declCfg.Bundles {
		bundleMeta := corev1beta1.BundleMetadata{
			ObjectMeta: metav1.ObjectMeta{
				Name: bundle.Name,
			},
			Spec: corev1beta1.BundleMetadataSpec{
				CatalogSource: bdl.Name,
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

		// TODO: Having some issues with properties values - find the actual fix and don't ignore them
		// for _, prop := range bundle.Properties {
		// 	// skip any properties that are of type `olm.bundle.object`
		// 	if prop.Type == "olm.bundle.object" {
		// 		continue
		// 	}

		// 	bundleMeta.Spec.Properties = append(bundleMeta.Spec.Properties, corev1beta1.Property{
		// 		Type:  prop.Type,
		// 		Value: runtime.RawExtension{Raw: prop.Value},
		// 	})
		// }

		if err := ctrlutil.SetOwnerReference(bdl, &bundleMeta, cbh.Client.Scheme()); err != nil {
			return fmt.Errorf("setting ownerref on bundlemetadata %q: %w", bundleMeta.Name, err)
		}

		// ignore already exist errors cause I don't feel like dealing with that as part of this
		if err := cbh.Client.Create(ctx, &bundleMeta); client.IgnoreAlreadyExists(err) != nil {
			return fmt.Errorf("creating bundlemetadata %q: %w", bundleMeta.Name, err)
		}
	}

	return nil
}

// createPackages will create a `Package` resource for each
// "olm.package" object that exists for the given catalog contents.
// `Package.Spec.Channels` is populated by filtering all "olm.channel" objects
// where the "packageName" == `Package.Name`. Returns an error if any are encountered.
func (cbh *CatalogdBundleHandler) createPackages(ctx context.Context, declCfg *declcfg.DeclarativeConfig, bdl *rukpakv1alpha1.Bundle) error {
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
				CatalogSource:  bdl.Name,
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

		if err := ctrlutil.SetOwnerReference(bdl, &pack, cbh.Client.Scheme()); err != nil {
			return fmt.Errorf("setting ownerref on package %q: %w", pack.Name, err)
		}

		// ignore already exist errors cause I don't feel like dealing with that as part of this
		if err := cbh.Client.Create(ctx, &pack); client.IgnoreAlreadyExists(err) != nil {
			return fmt.Errorf("creating package %q: %w", pack.Name, err)
		}
	}
	return nil
}
