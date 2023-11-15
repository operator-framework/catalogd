package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/alecthomas/chroma/quick"
	"github.com/operator-framework/catalogd/api/core/v1alpha1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"
)

var inspectCmd = cobra.Command{
	Use:   "inspect [schema] [name] [flags]",
	Short: "Inspects catalog objects",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		pkg, _ := cmd.Flags().GetString("package")
		catalog, _ := cmd.Flags().GetString("catalog")
		output, _ := cmd.Flags().GetString("output")
		schema := args[0]
		name := args[1]
		return inspect(schema, pkg, name, catalog, output)
	},
}

func init() {
	inspectCmd.Flags().String("package", "", "specify the FBC object package that should be used to filter the resulting output")
	inspectCmd.Flags().String("catalog", "", "specify the catalog that should be used. By default it will fetch from all catalogs")
	inspectCmd.Flags().String("output", "json", "specify the output format. Valid values are 'json' and 'yaml'")
}

func inspect(schema, pkg, name, catalogName, out string) error {
	cfg := ctrl.GetConfigOrDie()
	ctx := context.Background()

	catalogs, err := FetchCatalogs(cfg, ctx, WithNameCatalogFilter(catalogName))
	if err != nil {
		return err
	}

	err = WriteContents(cfg, ctx, catalogs,
		func(meta *declcfg.Meta, _ *v1alpha1.Catalog) error {
			outBytes, err := json.MarshalIndent(meta.Blob, "", "  ")
			if err != nil {
				return err
			}
			if out == "yaml" {
				outBytes, err = yaml.JSONToYAML(outBytes)
				if err != nil {
					return err
				}
			}
			// TODO: This uses ansi escape codes to colorize the output. Unfortunately, this
			// means it isn't compatible with jq or yq that expect the output to be plain text.
			return quick.Highlight(os.Stdout, string(outBytes), out, "terminal16m", "dracula")
		},
		WithNameContentFilter(name), WithSchemaContentFilter(schema), WithPackageContentFilter(pkg),
	)

	if err != nil {
		return err
	}
	return nil
}
