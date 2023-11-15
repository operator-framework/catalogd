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

var searchCmd = cobra.Command{
	Use:   "search [input] [flags]",
	Short: "Searches catalog objects",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pkg, _ := cmd.Flags().GetString("package")
		catalog, _ := cmd.Flags().GetString("catalog")
		schema, _ := cmd.Flags().GetString("schema")
		style, _ := cmd.Flags().GetString("style")
		input := args[0]
		return search(input, schema, pkg, catalog, style)
	},
}

func init() {
	searchCmd.Flags().String("schema", "", "specify the FBC object schema that should be used to filter the resulting output")
	searchCmd.Flags().String("package", "", "specify the FBC object package that should be used to filter the resulting output")
	searchCmd.Flags().String("catalog", "", "specify the catalog that should be used. By default it will fetch from all catalogs")
	searchCmd.Flags().String("style", "dracula", "specify the style that should be used to render the output")
}

func search(input, schema, pkg, catalogName, style string) error {
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
		WithNameContainsContentFilter(input), WithSchemaContentFilter(schema), WithPackageContentFilter(pkg),
	)

	if err != nil {
		return err
	}
	return nil
}
