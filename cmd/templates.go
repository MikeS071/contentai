package cmd

import (
	"fmt"

	tpl "github.com/MikeS071/contentai/internal/templates"
	"github.com/spf13/cobra"
)

func newTemplatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "Manage prompt templates",
		Long:  "Manage prompt templates used by generation commands.",
	}

	var (
		dir   string
		force bool
	)

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export embedded prompt templates",
		Long:  "Export embedded prompt templates into a local directory for customization.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			engine := tpl.NewEngine("content")
			report, err := engine.ExportWithForce(dir, force)
			if err != nil {
				return err
			}

			for _, name := range report.Exported {
				fmt.Fprintf(cmd.OutOrStdout(), "exported: %s.md\n", name)
			}
			for _, name := range report.Skipped {
				fmt.Fprintf(cmd.ErrOrStderr(), "skipped (exists): %s.md (use --force to overwrite)\n", name)
			}
			return nil
		},
	}

	exportCmd.Flags().StringVar(&dir, "dir", "content/templates", "Directory to export templates to")
	exportCmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files")

	cmd.AddCommand(exportCmd)
	return cmd
}
