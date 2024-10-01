package source

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/archive"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	catalogdv1alpha1 "github.com/operator-framework/catalogd/api/core/v1alpha1"
)

// TODO: Make asynchronous

type ImageRegistry struct {
	BaseCachePath string
	AuthNamespace string
}

func (i *ImageRegistry) Unpack(ctx context.Context, catalog *catalogdv1alpha1.ClusterCatalog) (*Result, error) {
	l := log.FromContext(ctx)
	if catalog.Spec.Source.Type != catalogdv1alpha1.SourceTypeImage {
		panic(fmt.Sprintf("programmer error: source type %q is unable to handle specified catalog source type %q", catalogdv1alpha1.SourceTypeImage, catalog.Spec.Source.Type))
	}

	if catalog.Spec.Source.Image == nil {
		return nil, wrapTerminal(fmt.Errorf("error parsing catalog, catalog %s has a nil image source", catalog.Name), true)
	}

	imgRef, err := name.ParseReference(catalog.Spec.Source.Image.Ref)
	if err != nil {
		return nil, wrapTerminal(fmt.Errorf("error parsing image reference: %w", err), true)
	}

	remoteOpts := []remote.Option{}
	digest, isDigest := imgRef.(name.Digest)
	if isDigest {
		hexVal := strings.TrimPrefix(digest.DigestStr(), "sha256:")
		unpackPath := filepath.Join(i.BaseCachePath, catalog.Name, hexVal)
		if stat, err := os.Stat(unpackPath); err == nil && stat.IsDir() {
			l.V(1).Info("found image in filesystem cache", "digest", hexVal)
			return unpackedResult(os.DirFS(unpackPath), digest.String(), metav1.Time{Time: time.Now()}), nil
		}
	}

	// always fetch the hash
	imgDesc, err := remote.Head(imgRef, remoteOpts...)
	if err != nil {
		return nil, fmt.Errorf("error fetching image descriptor: %w", err)
	}
	l.V(1).Info("resolved image descriptor", "digest", imgDesc.Digest.String())

	unpackPath := filepath.Join(i.BaseCachePath, catalog.Name, imgDesc.Digest.Hex)
	if _, err = os.Stat(unpackPath); errors.Is(err, os.ErrNotExist) { //nolint: nestif
		// Ensure any previous unpacked catalog is cleaned up before unpacking the new catalog.
		if err := i.Cleanup(ctx, catalog); err != nil {
			return nil, fmt.Errorf("error cleaning up catalog cache: %w", err)
		}

		if err = os.MkdirAll(unpackPath, 0700); err != nil {
			return nil, fmt.Errorf("error creating unpack path: %w", err)
		}

		if err = unpackImage(ctx, imgRef, unpackPath, remoteOpts...); err != nil {
			cleanupErr := os.RemoveAll(unpackPath)
			if cleanupErr != nil {
				err = apimacherrors.NewAggregate(
					[]error{
						err,
						fmt.Errorf("error cleaning up unpack path after unpack failed: %w", cleanupErr),
					},
				)
			}
			return nil, wrapUnrecoverable(fmt.Errorf("error unpacking image: %w", err), isDigest)
		}
	} else if err != nil {
		return nil, fmt.Errorf("error checking if image is in filesystem cache: %w", err)
	}

	ref := fmt.Sprintf("%s@sha256:%s", imgRef.Context().Name(), imgDesc.Digest.Hex)
	return unpackedResult(os.DirFS(unpackPath), ref, metav1.Time{Time: time.Now()}), nil
}

func wrapUnrecoverable(err error, isUnrecoverable bool) error {
	if isUnrecoverable {
		return wrapTerminal(err, isUnrecoverable)
	}
	return err
}

func (i *ImageRegistry) Cleanup(_ context.Context, catalog *catalogdv1alpha1.ClusterCatalog) error {
	return os.RemoveAll(filepath.Join(i.BaseCachePath, catalog.Name))
}

func unpackedResult(fsys fs.FS, ref string, lastUnpacked metav1.Time) *Result {
	return &Result{
		FS: fsys,
		ResolvedSource: &catalogdv1alpha1.ResolvedCatalogSource{
			Type: catalogdv1alpha1.SourceTypeImage,
			Image: &catalogdv1alpha1.ResolvedImageSource{
				Ref:                       ref,
				LastSuccessfulPollAttempt: metav1.Time{Time: time.Now()},
				LastUnpacked:              lastUnpacked,
			},
		},
		State: StateUnpacked,
	}
}

// unpackImage unpacks a catalog image reference to the provided unpackPath,
// returning an error if any errors are encountered along the way.
func unpackImage(ctx context.Context, imgRef name.Reference, unpackPath string, remoteOpts ...remote.Option) error {
	img, err := remote.Image(imgRef, remoteOpts...)
	if err != nil {
		return fmt.Errorf("error fetching remote image %q: %w", imgRef.Name(), err)
	}

	cfgFile, err := img.ConfigFile()
	if err != nil {
		return fmt.Errorf("error parsing remote image %q config file: %w", imgRef.Name(), err)
	}

	dirToUnpack, ok := cfgFile.Config.Labels[ConfigDirLabel]
	if !ok {
		return fmt.Errorf("catalog image %q is missing the required label %q", imgRef.String(), ConfigDirLabel)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("error getting image layers: %w", err)
	}

	dirBase := filepath.Base(dirToUnpack)
	for _, layer := range layers {
		layerRc, err := layer.Uncompressed()
		if err != nil {
			return fmt.Errorf("error getting uncompressed layer data: %w", err)
		}

		// Apply the layer contents, but filter on the directory that contains catalog contents so we only cache the
		// catalog contents and nothing else. This filter ensures that the files created have the proper UID and GID
		// for the filesystem they will be stored on to ensure no permission errors occur when attempting to create the
		// files.
		_, err = archive.Apply(ctx, unpackPath, layerRc, archive.WithFilter(func(th *tar.Header) (bool, error) {
			th.Uid = os.Getuid()
			th.Gid = os.Getgid()
			dir, file := filepath.Split(th.Name)
			return (dir == "" && file == dirBase) || strings.HasPrefix(dir, fmt.Sprintf("%s/", dirBase)), nil
		}))
		if err != nil {
			return fmt.Errorf("error applying layer to archive: %w", err)
		}
	}

	return nil
}
