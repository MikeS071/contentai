package cmd

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize ContentAI in the current project",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("not implemented")
		},
	}
}
