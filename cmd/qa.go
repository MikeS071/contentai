package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/qa"
	"github.com/spf13/cobra"
)

func newQACmd() *cobra.Command {
	var autoFix bool
	var approve bool

	cmd := &cobra.Command{
		Use:   "qa <slug>",
		Short: "Run QA checks and optional auto-fix",
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

			apiKey, err := resolveDraftAPIKey(cfg)
			if err != nil {
				return err
			}

			provider := strings.TrimSpace(cfg.LLM.Provider)
			if provider == "" {
				provider = "openai"
			}
			model := strings.TrimSpace(cfg.LLM.Model)
			if cfg.LLMQA != nil && strings.TrimSpace(cfg.LLMQA.Model) != "" {
				model = strings.TrimSpace(cfg.LLMQA.Model)
			}
			if model == "" {
				model = "gpt-4o-mini"
			}
			client, err := llm.NewClient(provider, model, apiKey, strings.TrimSpace(cfg.LLM.BaseURL))
			if err != nil {
				return fmt.Errorf("create llm client: %w", err)
			}

			effectiveAutoFix := autoFix
			if !cmd.Flags().Changed("auto-fix") {
				effectiveAutoFix = cfg.QA.AutoFix
			}

			engine := &qa.Engine{
				Store:       content.NewStore(contentDir),
				ContentDir:  contentDir,
				LLM:         client,
				Model:       model,
				Temperature: 0,
			}
			result, err := engine.Run(cmd.Context(), qa.RunOptions{
				Slug:    args[0],
				AutoFix: effectiveAutoFix,
				Approve: approve,
			})
			if err != nil {
				return err
			}

			printQASummary(cmd.OutOrStdout(), result)
			return nil
		},
	}

	cmd.Flags().BoolVar(&autoFix, "auto-fix", false, "Apply LLM-assisted fixes for flagged issues")
	cmd.Flags().BoolVar(&approve, "approve", false, "Mark content as qa_passed")
	return cmd
}

func printQASummary(out io.Writer, result *qa.RunResult) {
	if result == nil || result.QA == nil {
		fmt.Fprintln(out, "No QA results")
		return
	}

	green := "\033[32m"
	yellow := "\033[33m"
	red := "\033[31m"
	reset := "\033[0m"

	for _, check := range result.QA.Checks {
		status := red + "FAIL" + reset
		if check.Passed {
			status = green + "PASS" + reset
		} else if check.Name == "length" {
			status = yellow + "WARN" + reset
		}
		fmt.Fprintf(out, "%s %-18s issues=%d\n", status, check.Name, len(check.Issues))
	}

	if strings.TrimSpace(result.Diff) != "" {
		fmt.Fprintln(out, "\nAuto-fix diff:")
		fmt.Fprintln(out, result.Diff)
	}
}
