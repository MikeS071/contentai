package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/ideas"
	"github.com/MikeS071/contentai/internal/kb"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/openclaw"
	"github.com/MikeS071/contentai/internal/templates"
	"github.com/spf13/cobra"
)

func newIdeasCmd() *cobra.Command {
	var (
		fromKB            bool
		fromConversations bool
		count             int
	)

	cmd := &cobra.Command{
		Use:   "ideas",
		Short: "Generate structured content ideas from your KB and blueprint",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}

			contentDir := strings.TrimSpace(cfg.Project.ContentDir)
			if contentDir == "" {
				contentDir = "content"
			}
			client, err := newIdeasLLMClient(cfg)
			if err != nil {
				return err
			}

			gen := ideas.NewGenerator(
				contentDir,
				client,
				kb.NewStore(contentDir),
				templates.NewEngine(contentDir),
				content.NewStore(contentDir),
			)
			conversation := ""
			if fromConversations {
				history, historyErr := openclaw.NewReader(nil).ReadConversationHistory(cfg.OpenClaw)
				if historyErr != nil {
					return historyErr
				}
				conversation = history
			}

			outlines, err := gen.Generate(cmd.Context(), ideas.GenerateOptions{
				FromKB:             fromKB,
				FromConversations:  fromConversations,
				ConversationSource: conversation,
				Count:              count,
			})
			if err != nil {
				return err
			}

			batchPath, err := gen.SaveBatch(outlines)
			if err != nil {
				return err
			}
			for i, idea := range outlines {
				fmt.Fprintf(cmd.OutOrStdout(), "%d. %s\n", i+1, idea.Title)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved ideas batch: %s\n", batchPath)

			_, err = gen.PickAndCreate(cmd.InOrStdin(), cmd.OutOrStdout(), outlines)
			return err
		},
	}

	cmd.Flags().BoolVar(&fromKB, "from-kb", true, "Include recent KB articles and notes")
	cmd.Flags().BoolVar(&fromConversations, "from-conversations", false, "Include conversation snippets when available")
	cmd.Flags().IntVar(&count, "count", 5, "Number of idea outlines to generate")
	return cmd
}

func newIdeasLLMClient(cfg *config.Config) (llm.LLMClient, error) {
	provider := strings.TrimSpace(cfg.LLM.Provider)
	if provider == "" {
		provider = "openai"
	}
	model := strings.TrimSpace(cfg.LLM.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}

	apiKey := strings.TrimSpace(os.Getenv("CONTENTAI_LLM_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if strings.TrimSpace(cfg.LLM.APIKeyCmd) != "" {
		resolved, err := llm.ResolveAPIKey(cfg.LLM.APIKeyCmd)
		if err != nil {
			return nil, fmt.Errorf("resolve llm api key: %w", err)
		}
		apiKey = resolved
	}

	client, err := llm.NewClient(provider, model, apiKey, strings.TrimSpace(cfg.LLM.BaseURL))
	if err != nil {
		return nil, err
	}
	return client, nil
}
