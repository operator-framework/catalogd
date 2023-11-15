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

//TODO: Make common global flags

func init() {
	root.AddCommand(&listCmd)
	root.AddCommand(&inspectCmd)
	root.AddCommand(&searchCmd)
}

func Execute() {
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
