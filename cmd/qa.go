package cmd

import (
	"bufio"
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
		Long:  "Execute built-in and LLM-assisted quality checks, optionally apply suggested fixes, and mark the item QA-approved.",
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
			store := content.NewStore(contentDir)
			engine.Store = store
			result, err := engine.Run(cmd.Context(), qa.RunOptions{
				Slug:    args[0],
				AutoFix: effectiveAutoFix,
				Approve: approve,
			})
			if err != nil {
				return err
			}
			if effectiveAutoFix {
				if err := reviewAndApplyFixes(cmd.InOrStdin(), cmd.OutOrStdout(), store, args[0], result); err != nil {
					return err
				}
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

func reviewAndApplyFixes(in io.Reader, out io.Writer, store *content.Store, slug string, result *qa.RunResult) error {
	if result == nil || result.QA == nil {
		return nil
	}
	article, err := store.ReadArticle(slug)
	if err != nil {
		return err
	}
	updated := article
	approvedAny := false
	reader := bufio.NewReader(in)

	for cIdx := range result.QA.Checks {
		check := &result.QA.Checks[cIdx]
		if len(check.Fixes) == 0 {
			continue
		}
		for fIdx := range check.Fixes {
			fix := &check.Fixes[fIdx]
			fmt.Fprintf(out, "\nProposed fix for %s:\n", check.Name)
			if len(check.Issues) > 0 {
				fmt.Fprintf(out, "- %s\n", check.Issues[0])
			}
			fmt.Fprint(out, "Apply this fix? [y/N]: ")
			line, readErr := reader.ReadString('\n')
			if readErr != nil && strings.TrimSpace(line) == "" {
				fix.Applied = false
				continue
			}
			answer := strings.ToLower(strings.TrimSpace(line))
			if answer == "y" || answer == "yes" {
				fix.Applied = true
				updated = fix.Fixed
				approvedAny = true
			} else {
				fix.Applied = false
			}
		}
	}

	if approvedAny {
		if err := store.WriteArticle(slug, strings.TrimSpace(updated)+"\n"); err != nil {
			return err
		}
	}
	return store.WriteQA(slug, result.QA)
}
