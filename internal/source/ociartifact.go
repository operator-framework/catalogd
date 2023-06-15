package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/docker/distribution/reference"
	pkg "github.com/joelanford/olm-oci/api/v1"
	"github.com/joelanford/olm-oci/pkg/fetch"
	"github.com/joelanford/olm-oci/pkg/remote"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	catalogdv1alpha1 "github.com/operator-framework/catalogd/api/core/v1alpha1"
)

type OCIArtifact struct {
	LocalStore *oci.Store
	FBCRootDir string
}

func (i *OCIArtifact) Unpack(ctx context.Context, catalog *catalogdv1alpha1.Catalog) (*Result, error) {
	if catalog.Spec.Source.Type != catalogdv1alpha1.SourceTypeOCIArtifact {
		panic(fmt.Sprintf("source type %q is unable to handle specified catalog source type %q", catalogdv1alpha1.SourceTypeOCIArtifact, catalog.Spec.Source.Type))
	}
	if catalog.Spec.Source.OCIArtifact == nil {
		return nil, fmt.Errorf("catalog source image configuration is unset")
	}

	repo, ref, err := remote.ParseNameAndReference(catalog.Spec.Source.OCIArtifact.Ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference: %v", err)
	}

	ignoreMediaTypes := []string{pkg.MediaTypeBundleContent}

	desc, err := repo.Resolve(ctx, ref.String())
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %v", err)
	}
	if err := oras.CopyGraph(ctx, repo, i.LocalStore, desc, oras.CopyGraphOptions{
		Concurrency:    runtime.NumCPU(),
		FindSuccessors: fetch.IgnoreMediaTypes(ignoreMediaTypes...),
	}); err != nil {
		return nil, fmt.Errorf("pull artifact to local storage: %v", err)
	}

	art, err := fetch.FetchArtifact(ctx, i.LocalStore, desc)
	if err != nil {
		return nil, fmt.Errorf("fetch artifact descriptor: %v", err)
	}

	type toFBCer interface {
		ToFBC(context.Context, string) (*declcfg.DeclarativeConfig, error)
	}

	var tfbc toFBCer
	switch art.ArtifactType {
	case pkg.MediaTypeCatalog:
		tfbc, err = fetch.FetchCatalog(ctx, i.LocalStore, art, ignoreMediaTypes...)
	case pkg.MediaTypePackage:
		tfbc, err = fetch.FetchPackage(ctx, i.LocalStore, art, ignoreMediaTypes...)
	default:
		return nil, fmt.Errorf("unsupported artifact type %q", desc.ArtifactType)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch artifact: %v", err)
	}

	fbc, err := tfbc.ToFBC(ctx, ref.Name())
	if err != nil {
		return nil, fmt.Errorf("convert to FBC: %v", err)
	}

	fbcDir := filepath.Join(i.FBCRootDir, catalog.Name)
	if err := os.RemoveAll(fbcDir); err != nil {
		return nil, fmt.Errorf("remove old FBC: %v", err)
	}
	if err := declcfg.WriteFS(*fbc, fbcDir, declcfg.WriteJSON, ".json"); err != nil {
		return nil, fmt.Errorf("write FBC: %v", err)
	}

	digestRef, err := reference.WithDigest(ref, desc.Digest)
	if err != nil {
		return nil, fmt.Errorf("create digest reference: %v", err)
	}
	resolvedSource := &catalogdv1alpha1.CatalogSource{
		Type: catalogdv1alpha1.SourceTypeOCIArtifact,
		OCIArtifact: &catalogdv1alpha1.ImageSource{
			Ref: digestRef.String(),
		},
	}
	message := fmt.Sprintf("successfully unpacked the catalog image %q", ref.String())

	return &Result{FS: os.DirFS(fbcDir), ResolvedSource: resolvedSource, State: StateUnpacked, Message: message}, nil
}
