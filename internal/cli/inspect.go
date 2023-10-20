package cli

import (
	"context"
	"encoding/json"
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

var inspectCmd = cobra.Command{
	Use:   "inspect [schema] [name] [flags]",
	Short: "Inspects catalog objects",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		pkg, _ := cmd.Flags().GetString("package")
		catalog, _ := cmd.Flags().GetString("catalog")
		schema := args[0]
		name := args[1]
		return inspect(schema, pkg, name, catalog)
	},
}

func init() {
	inspectCmd.Flags().String("package", "", "specify the FBC object package that should be used to filter the resulting output")
	inspectCmd.Flags().String("catalog", "", "specify the catalog that should be used. By default it will fetch from all catalogs")
}

func inspect(schema, pkg, name, catalogName string) error {
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

			outJson, err := json.MarshalIndent(meta.Blob, "", "  ")
			if err != nil {
				return err
			}
			outMd := strings.Builder{}
			outMd.WriteString("```json\n")
			outMd.WriteString(string(outJson))
			outMd.WriteString("\n```\n")

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
