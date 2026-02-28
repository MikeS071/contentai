package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	initflow "github.com/MikeS071/contentai/internal/init"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/templates"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [name]",
		Short: "Initialize ContentAI in the current project",
		Long:  "Run the onboarding wizard to generate blueprint and voice files, configure integrations, and create initial workspace folders.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			projectName := ""
			if len(args) == 1 {
				projectName = args[0]
			}

			apiKey := os.Getenv("CONTENTAI_LLM_API_KEY")
			if apiKey == "" {
				apiKey = os.Getenv("OPENAI_API_KEY")
			}
			if apiKey == "" {
				return fmt.Errorf("set CONTENTAI_LLM_API_KEY or OPENAI_API_KEY before running init")
			}

			baseURL := strings.TrimSpace(os.Getenv("CONTENTAI_LLM_BASE_URL"))
			client, err := llm.NewClient("openai", "gpt-4o-mini", apiKey, baseURL)
			if err != nil {
				return fmt.Errorf("create llm client: %w", err)
			}

			engine := templates.NewEngine(filepath.Join(workDir, "content"))
			wizard := initflow.NewWizard(cmd.InOrStdin(), cmd.OutOrStdout(), workDir, projectName, client, engine)
			return wizard.Run(cmd.Context())
		},
	}
}
