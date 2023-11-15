package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/operator-framework/catalogd/api/core/v1alpha1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func CatalogdTermRenderer(style string) (*glamour.TermRenderer, error) {
	sc := glamour.DefaultStyles[style]
	sc.Document.BlockSuffix = ""
	sc.Document.BlockPrefix = ""
	return glamour.NewTermRenderer(glamour.WithStyles(*sc))
}

type CatalogFilterFunc func(catalog *v1alpha1.Catalog) bool

func FetchCatalogs(cfg *rest.Config, ctx context.Context, filters ...CatalogFilterFunc) ([]v1alpha1.Catalog, error) {
	dynamicClient := dynamic.NewForConfigOrDie(cfg)

	catalogList := &v1alpha1.CatalogList{}
	unstructCatalogs, err := dynamicClient.Resource(v1alpha1.GroupVersion.WithResource("catalogs")).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructCatalogs.UnstructuredContent(), catalogList)
	if err != nil {
		return nil, err
	}

	catalogs := []v1alpha1.Catalog{}
	for _, catalog := range catalogList.Items {
		for _, filter := range filters {
			if !filter(&catalog) {
				continue
			}
		}

		catalogs = append(catalogs, catalog)
	}

	return catalogs, nil
}

func WithNameCatalogFilter(name string) CatalogFilterFunc {
	return func(catalog *v1alpha1.Catalog) bool {
		if name == "" {
			return true
		}
		return catalog.Name == name
	}
}

type ContentFilterFunc func(meta *declcfg.Meta) bool
type WriteFunc func(meta *declcfg.Meta, catalog *v1alpha1.Catalog) error

func WriteContents(cfg *rest.Config, ctx context.Context, catalogs []v1alpha1.Catalog, writeFunc WriteFunc, filters ...ContentFilterFunc) error {
	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	for _, catalog := range catalogs {
		if !meta.IsStatusConditionTrue(catalog.Status.Conditions, v1alpha1.TypeUnpacked) {
			continue
		}

		url, err := url.Parse(catalog.Status.ContentURL)
		if err != nil {
			return fmt.Errorf("parsing catalog content url for catalog %q: %w", catalog.Name, err)
		}
		// url is expected to be in the format of
		// http://{service_name}.{namespace}.svc/{catalog_name}/all.json
		// so to get the namespace and name of the service we grab only
		// the hostname and split it on the '.' character
		ns := strings.Split(url.Hostname(), ".")[1]
		name := strings.Split(url.Hostname(), ".")[0]
		port := url.Port()
		// the ProxyGet() call below needs an explicit port value, so if
		// value from url.Port() is empty, we assume port 80.
		if port == "" {
			port = "80"
		}

		rw := kubeClient.CoreV1().Services(ns).ProxyGet(
			url.Scheme,
			name,
			port,
			url.Path,
			map[string]string{},
		)

		rc, err := rw.Stream(ctx)
		if err != nil {
			return fmt.Errorf("getting catalog contents for catalog %q: %w", catalog.Name, err)
		}
		defer rc.Close()

		err = declcfg.WalkMetasReader(rc, func(meta *declcfg.Meta, err error) error {
			if err != nil {
				return err
			}

			for _, filter := range filters {
				if !filter(meta) {
					return nil
				}
			}

			writeErr := writeFunc(meta, &catalog)
			if writeErr != nil {
				return writeErr
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("reading FBC for catalog %q: %w", catalog.Name, err)
		}
	}

	return nil
}

func WithSchemaContentFilter(schema string) ContentFilterFunc {
	return func(meta *declcfg.Meta) bool {
		if schema == "" {
			return true
		}
		return meta.Schema == schema
	}
}

func WithPackageContentFilter(pkg string) ContentFilterFunc {
	return func(meta *declcfg.Meta) bool {
		if pkg == "" {
			return true
		}
		return meta.Package == pkg
	}
}

func WithNameContentFilter(name string) ContentFilterFunc {
	return func(meta *declcfg.Meta) bool {
		if name == "" {
			return true
		}
		return meta.Name == name
	}
}

func WithNameContainsContentFilter(name string) ContentFilterFunc {
	return func(meta *declcfg.Meta) bool {
		if name == "" {
			return true
		}
		return strings.Contains(meta.Name, name)
	}
}
