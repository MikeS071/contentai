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
		Long:          "ContentAI is a CLI for end-to-end content workflows: research, ideation, drafting, QA, publishing, and scheduling.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "contentai.toml", "path to config file")
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newKBCmd())
	rootCmd.AddCommand(newIdeasCmd())
	rootCmd.AddCommand(newNewCmd())
	rootCmd.AddCommand(newDraftCmd())
	rootCmd.AddCommand(newQACmd())
	rootCmd.AddCommand(newHeroCmd())
	rootCmd.AddCommand(newPublishCmd())
	rootCmd.AddCommand(newSocialCmd())
	rootCmd.AddCommand(newScheduleCmd())
	rootCmd.AddCommand(newTemplatesCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}
