package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/operator-framework/catalogd/api/core/v1alpha1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

var listCmd = cobra.Command{
	Use:   "list [flags]",
	Short: "Lists catalog objects",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		schema, _ := cmd.Flags().GetString("schema")
		pkg, _ := cmd.Flags().GetString("package")
		name, _ := cmd.Flags().GetString("name")
		catalog, _ := cmd.Flags().GetString("catalog")

		return list(schema, pkg, name, catalog)
	},
}

func init() {
	listCmd.Flags().String("schema", "", "specify the FBC object schema that should be used to filter the resulting output")
	listCmd.Flags().String("package", "", "specify the FBC object package that should be used to filter the resulting output")
	listCmd.Flags().String("name", "", "specify the FBC object name that should be used to filter the resulting output")
	listCmd.Flags().String("catalog", "", "specify the catalog that should be used. By default it will fetch from all catalogs")

}

func list(schema, pkg, name, catalogName string) error {
	sc := glamour.DraculaStyleConfig
	sc.Document.BlockSuffix = ""
	sc.Document.BlockPrefix = ""
	tr, err := glamour.NewTermRenderer(glamour.WithStyles(sc))
	if err != nil {
		return err
	}

	cfg := ctrl.GetConfigOrDie()
	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	dynamicClient := dynamic.NewForConfigOrDie(cfg)
	ctx := context.Background()

	catalogs := &v1alpha1.CatalogList{}
	if catalogName == "" {
		// get Catalog list
		unstructCatalogs, err := dynamicClient.Resource(v1alpha1.GroupVersion.WithResource("catalogs")).List(ctx, v1.ListOptions{})
		if err != nil {
			return err
		}

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructCatalogs.UnstructuredContent(), catalogs)
		if err != nil {
			return err
		}
	} else {
		// get Catalog
		unstructCatalog, err := dynamicClient.Resource(v1alpha1.GroupVersion.WithResource("catalogs")).Get(ctx, catalogName, v1.GetOptions{})
		if err != nil {
			return err
		}

		ctlg := v1alpha1.Catalog{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructCatalog.UnstructuredContent(), &ctlg)
		if err != nil {
			return err
		}
		catalogs.Items = append(catalogs.Items, ctlg)
	}

	for _, catalog := range catalogs.Items {
		if !meta.IsStatusConditionTrue(catalog.Status.Conditions, v1alpha1.TypeUnpacked) {
			continue
		}

		rw := kubeClient.CoreV1().Services("catalogd-system").ProxyGet(
			"http",
			"catalogd-catalogserver",
			"80",
			fmt.Sprintf("catalogs/%s/all.json", catalog.Name),
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

			if schema != "" {
				if meta.Schema != schema {
					return nil
				}
			}

			if pkg != "" {
				if meta.Package != pkg {
					return nil
				}
			}

			if name != "" {
				if meta.Name != name {
					return nil
				}
			}

			outMd := strings.Builder{}
			outMd.WriteString(fmt.Sprintf("`%s` **%s** ", catalog.Name, meta.Schema))
			if meta.Package != "" {
				outMd.WriteString(fmt.Sprintf("_%s_ ", meta.Package))
			}
			outMd.WriteString(meta.Name)

			out, _ := tr.Render(outMd.String())
			fmt.Print(out)
			return nil
		})
		if err != nil {
			return fmt.Errorf("reading FBC for catalog %q: %w", catalog.Name, err)
		}
	}

	return nil
}
