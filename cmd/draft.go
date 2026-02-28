package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/draft"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/templates"
	"github.com/spf13/cobra"
)

func newDraftCmd() *cobra.Command {
	var sourcePath string
	var interactive bool

	cmd := &cobra.Command{
		Use:   "draft <slug>",
		Short: "Generate or refine an article draft",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadDraftConfig(cfgFile)
			if err != nil {
				return err
			}

			contentDir := cfg.Project.ContentDir
			if strings.TrimSpace(contentDir) == "" {
				contentDir = "content"
			}

			apiKey, err := resolveDraftAPIKey(cfg)
			if err != nil {
				return err
			}

			provider := strings.TrimSpace(cfg.LLM.Provider)
			if provider == "" {
				provider = "openai"
			}
			model := strings.TrimSpace(cfg.LLM.Model)
			if cfg.LLMDraft != nil && strings.TrimSpace(cfg.LLMDraft.Model) != "" {
				model = strings.TrimSpace(cfg.LLMDraft.Model)
			}
			if model == "" {
				model = "gpt-4o-mini"
			}

			client, err := llm.NewClient(provider, model, apiKey, strings.TrimSpace(cfg.LLM.BaseURL))
			if err != nil {
				return fmt.Errorf("create llm client: %w", err)
			}

			store := content.NewStore(contentDir)
			engine := templates.NewEngine(contentDir)
			d := &draft.Drafter{
				Store:      store,
				ContentDir: contentDir,
				LLM:        client,
				Templates:  engine,
				Stdin:      cmd.InOrStdin(),
				Stdout:     cmd.OutOrStdout(),
				Model:      model,
			}
			if cfg.LLMDraft != nil {
				d.Temperature = cfg.LLMDraft.Temperature
			}

			return d.Draft(cmd.Context(), draft.Options{
				Slug:         args[0],
				SourcePath:   sourcePath,
				Interactive:  interactive,
				Conversation: readOpenClawConversation(cfg),
			})
		},
	}

	cmd.Flags().StringVar(&sourcePath, "source", "", "Path to extra source material")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Run interactive insight-mining before drafting")
	return cmd
}

func loadDraftConfig(path string) (*config.Config, error) {
	if strings.TrimSpace(path) != "" {
		if _, err := os.Stat(path); err == nil {
			return config.Load(path)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	cfg := config.Default()
	cfg.Project.Name = "contentai-project"
	return cfg, nil
}

func resolveDraftAPIKey(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config is required")
	}
	if cmd := strings.TrimSpace(cfg.LLM.APIKeyCmd); cmd != "" {
		key, err := llm.ResolveAPIKey(cmd)
		if err != nil {
			return "", err
		}
		if key != "" {
			return key, nil
		}
	}
	for _, env := range []string{"CONTENTAI_LLM_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("set llm.api_key_cmd in config or provide CONTENTAI_LLM_API_KEY/OPENAI_API_KEY")
}

func readOpenClawConversation(cfg *config.Config) string {
	if cfg == nil || !cfg.OpenClaw.Enabled || !cfg.OpenClaw.ChannelHistory {
		return ""
	}
	workspace := strings.TrimSpace(cfg.OpenClaw.Workspace)
	if workspace == "" {
		return ""
	}
	if strings.HasPrefix(workspace, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			workspace = filepath.Join(home, strings.TrimPrefix(workspace, "~/"))
		}
	}
	path := filepath.Join(workspace, "channel-history.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
