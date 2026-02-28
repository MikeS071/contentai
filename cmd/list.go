package cmd

import "github.com/spf13/cobra"

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List content items and lifecycle states",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("not implemented")
		},
	}
}
