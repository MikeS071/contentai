package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/spf13/cobra"
)

func newScheduleCmd() *cobra.Command {
	var immediate bool

	cmd := &cobra.Command{
		Use:   "schedule <slug>",
		Short: "Mark a published item as scheduled",
		Long:  "Transition an item from social_generated to scheduled, or directly to posted with --immediate.",
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

			if err := store.Transition(slug, content.StatusScheduled, cfg.Project.QAGate); err != nil {
				return err
			}
			if immediate {
				if err := store.Transition(slug, content.StatusPosted, cfg.Project.QAGate); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Marked %s as posted at %s\n", slug, time.Now().UTC().Format(time.RFC3339))
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %s\n", slug)
			return nil
		},
	}

	cmd.Flags().BoolVar(&immediate, "immediate", false, "Immediately mark the content item as posted")
	return cmd
}
