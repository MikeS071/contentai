package cmd

import "github.com/spf13/cobra"

var (
	cfgFile string
	version = "dev"
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "contentai",
		Short:         "AI-powered content creation and publishing CLI",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "contentai.toml", "path to config file")
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newKBCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}
