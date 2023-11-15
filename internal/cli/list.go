package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/operator-framework/catalogd/api/core/v1alpha1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/spf13/cobra"
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
		style, _ := cmd.Flags().GetString("style")

		return list(schema, pkg, name, catalog, style)
	},
}

func init() {
	listCmd.Flags().String("schema", "", "specify the FBC object schema that should be used to filter the resulting output")
	listCmd.Flags().String("package", "", "specify the FBC object package that should be used to filter the resulting output")
	listCmd.Flags().String("name", "", "specify the FBC object name that should be used to filter the resulting output")
	listCmd.Flags().String("catalog", "", "specify the catalog that should be used. By default it will fetch from all catalogs")
	listCmd.Flags().String("style", "dracula", "specify the style that should be used to render the output")
}

func list(schema, pkg, name, catalogName, style string) error {
	renderer, err := CatalogdTermRenderer(style)
	if err != nil {
		return err
	}

	cfg := ctrl.GetConfigOrDie()
	ctx := context.Background()
	catalogs, err := FetchCatalogs(cfg, ctx, WithNameCatalogFilter(catalogName))
	if err != nil {
		return err
	}

	err = WriteContents(cfg, ctx, catalogs,
		func(meta *declcfg.Meta, catalog *v1alpha1.Catalog) error {
			outMd := strings.Builder{}
			outMd.WriteString(fmt.Sprintf("`%s` **%s** ", catalog.Name, meta.Schema))
			if meta.Package != "" {
				outMd.WriteString(fmt.Sprintf("_%s_ ", meta.Package))
			}
			outMd.WriteString(meta.Name)

			out, _ := renderer.Render(outMd.String())
			fmt.Print(out)
			return nil
		},
		WithNameContentFilter(name), WithSchemaContentFilter(schema), WithPackageContentFilter(pkg),
	)
	if err != nil {
		return err
	}
	return nil
}
