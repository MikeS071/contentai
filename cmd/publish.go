package cmd

import (
	"fmt"
	"strings"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/publish"
	"github.com/spf13/cobra"
)

func newPublishCmd() *cobra.Command {
	var (
		approve bool
		dryRun  bool
	)

	cmd := &cobra.Command{
		Use:   "publish <slug>",
		Short: "Publish a QA-passed article via configured adapter",
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

			publisher, err := publish.NewPublisherFromConfig(cfg.Publish)
			if err != nil {
				return err
			}

			svc := publish.NewService(content.NewStore(contentDir), publisher, publish.ServiceConfig{
				RequireApprove: true,
				QAGate:         cfg.Project.QAGate,
			})

			out, err := svc.PublishSlug(cmd.Context(), args[0], publish.PublishOptions{
				Approve: approve,
				DryRun:  dryRun,
			})
			if err != nil {
				return err
			}

			if out.DryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run payload:")
				fmt.Fprint(cmd.OutOrStdout(), string(out.PayloadJSON))
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Published %s: %s\n", args[0], out.Result.URL)
			return nil
		},
	}

	cmd.Flags().BoolVar(&approve, "approve", false, "Approve and execute publish")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show payload without publishing")
	return cmd
}
