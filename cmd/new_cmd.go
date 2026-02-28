package cmd

import (
	"fmt"
	"strings"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	var (
		title    string
		fromIdea int
	)

	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Create a new content item scaffold",
		Long:  "Create content/<slug>/ with metadata, article, and social scaffold files.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadDraftConfig(cfgFile)
			if err != nil {
				return err
			}
			contentDir := strings.TrimSpace(cfg.Project.ContentDir)
			if contentDir == "" {
				contentDir = "content"
			}

			store := content.NewStore(contentDir)
			slug := args[0]
			effectiveTitle := strings.TrimSpace(title)
			if effectiveTitle == "" {
				effectiveTitle = slug
			}

			if err := store.Create(slug, effectiveTitle); err != nil {
				return err
			}
			if fromIdea > 0 {
				outline, err := loadOutlineFromLatestBatch(store.ContentDir, fromIdea)
				if err != nil {
					return err
				}
				if err := store.WriteArticle(slug, outline); err != nil {
					return err
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created content item: %s\n", store.SlugDir(slug))
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Custom title for the content item")
	cmd.Flags().IntVar(&fromIdea, "from-idea", 0, "Seed article from an idea index in the latest ideas batch")
	return cmd
}
