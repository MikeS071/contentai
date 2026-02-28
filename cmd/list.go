package cmd

import "github.com/spf13/cobra"

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List content items and lifecycle states",
		Long:  "List content items and their current lifecycle state.",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("not implemented")
		},
	}
}
