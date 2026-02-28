package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/hero"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/templates"
	"github.com/spf13/cobra"
)

func newHeroCmd() *cobra.Command {
	var regenerate bool

	cmd := &cobra.Command{
		Use:   "hero <slug>",
		Short: "Generate hero images for a content item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}

			contentDir := strings.TrimSpace(cfg.Project.ContentDir)
			if contentDir == "" {
				contentDir = "content"
			}

			provider := strings.ToLower(strings.TrimSpace(cfg.Images.Provider))
			if provider == "" {
				provider = "openai"
			}
			if provider != "openai" {
				return fmt.Errorf("unsupported image provider %q", provider)
			}

			apiKey, err := resolveImageAPIKey(cfg)
			if err != nil {
				return err
			}

			imageGen := hero.NewOpenAIImageGenerator(apiKey, "")
			gen := hero.NewGenerator(contentDir, imageGen, templates.NewEngine(contentDir), content.NewStore(contentDir))
			if model := strings.TrimSpace(cfg.Images.Model); model != "" {
				gen.Model = model
			}
			if size := strings.TrimSpace(cfg.Images.Size); size != "" {
				gen.Size = size
			}
			gen.TitleOverlay = cfg.Images.TitleOverlay

			if err := gen.Generate(cmd.Context(), args[0], hero.GenerateOptions{Regenerate: regenerate}); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Generated hero images: %s, %s\n", content.NewStore(contentDir).HeroPath(args[0]), content.NewStore(contentDir).HeroLinkedInPath(args[0]))
			return nil
		},
	}

	cmd.Flags().BoolVar(&regenerate, "regenerate", false, "Overwrite existing hero images")
	return cmd
}

func resolveImageAPIKey(cfg *config.Config) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("CONTENTAI_IMAGE_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if cmd := strings.TrimSpace(cfg.Images.APIKeyCmd); cmd != "" {
		resolved, err := llm.ResolveAPIKey(cmd)
		if err != nil {
			return "", fmt.Errorf("resolve image api key: %w", err)
		}
		apiKey = resolved
	}
	if apiKey == "" {
		return "", fmt.Errorf("image api key is required via CONTENTAI_IMAGE_API_KEY, OPENAI_API_KEY, or images.api_key_cmd")
	}
	return apiKey, nil
}
