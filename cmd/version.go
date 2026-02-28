package cmd

import "github.com/spf13/cobra"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print contentai version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println(cmd.Root().Version)
		},
	}
}
