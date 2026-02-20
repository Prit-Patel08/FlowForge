package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate Markdown documentation for the CLI",
	Long:  `Generates standard Markdown documentation for all flowforge commands in the ./docs/reference/cli directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		outputDir := "./docs/reference/cli"
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Printf("Error creating docs directory: %v\n", err)
			os.Exit(1)
		}

		if err := doc.GenMarkdownTree(rootCmd, outputDir); err != nil {
			fmt.Printf("Error generating docs: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Documentation generated in %s/\n", outputDir)
	},
}

func init() {
	rootCmd.AddCommand(docsCmd)
}
