package cli

import (
	"log"

	"github.com/spf13/cobra"
)

var root = cobra.Command{
	Use:   "catalogd",
	Short: "catalogd CLI",
	Long:  "CLI for interacting with catalogd",
}

func init() {
	root.AddCommand(&listCmd)
	root.AddCommand(&inspectCmd)
}

func Execute() {
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
