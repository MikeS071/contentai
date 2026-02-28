package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/social"
	"github.com/MikeS071/contentai/internal/templates"
	"github.com/spf13/cobra"
)

func newSocialCmd() *cobra.Command {
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "social <slug>",
		Short: "Generate social copy for X and LinkedIn",
		Long:  "Generate, review, and save social copy derived from the published article and voice profile.",
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
			if model == "" {
				model = "gpt-4o-mini"
			}

			client, err := llm.NewClient(provider, model, apiKey, strings.TrimSpace(cfg.LLM.BaseURL))
			if err != nil {
				return fmt.Errorf("create llm client: %w", err)
			}

			gen := social.NewGenerator(
				contentDir,
				client,
				templates.NewEngine(contentDir),
				content.NewStore(contentDir),
			)
			gen.Model = model

			result, err := gen.Generate(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			printSocialPreview(cmd.OutOrStdout(), result)
			if nonInteractive {
				return nil
			}
			return reviewSocialCopy(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), gen, args[0], result)
		},
	}

	cmd.Flags().BoolVar(&nonInteractive, "no-interactive", false, "Skip inline review/edit loop")
	return cmd
}

func printSocialPreview(out io.Writer, socialCopy *social.SocialJSON) {
	fmt.Fprintln(out, "Social preview:")
	fmt.Fprintln(out, "\n[X]")
	fmt.Fprintln(out, socialCopy.X.Text)
	fmt.Fprintln(out, "\n[LinkedIn]")
	fmt.Fprintln(out, socialCopy.LinkedIn.Text)
}

func reviewSocialCopy(ctx context.Context, in io.Reader, out io.Writer, gen *social.Generator, slug string, socialCopy *social.SocialJSON) error {
	reader := bufio.NewReader(in)
	for {
		fmt.Fprintln(out, "\nOptions: [s]ave, edit [x], edit [l]inkedin, [r]egenerate, [q]uit")
		fmt.Fprint(out, "> ")

		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return nil
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "s", "save":
			return gen.Save(slug, socialCopy)
		case "q", "quit":
			return nil
		case "x":
			fmt.Fprintln(out, "Enter new X text:")
			fmt.Fprint(out, "> ")
			text, readErr := reader.ReadString('\n')
			if readErr != nil && strings.TrimSpace(text) == "" {
				return readErr
			}
			if strings.TrimSpace(text) != "" {
				socialCopy.X.Text = strings.TrimSpace(text)
				if gen.Now != nil {
					socialCopy.X.GeneratedAt = gen.Now().UTC()
				}
			}
			printSocialPreview(out, socialCopy)
		case "l", "linkedin":
			fmt.Fprintln(out, "Enter LinkedIn text. End input with a single dot on its own line:")
			var lines []string
			for {
				fmt.Fprint(out, "> ")
				next, readErr := reader.ReadString('\n')
				next = strings.TrimRight(next, "\r\n")
				if readErr != nil && strings.TrimSpace(next) == "" {
					return readErr
				}
				if strings.TrimSpace(next) == "." {
					break
				}
				lines = append(lines, next)
				if readErr != nil {
					break
				}
			}
			merged := strings.TrimSpace(strings.Join(lines, "\n"))
			if merged != "" {
				socialCopy.LinkedIn.Text = merged
				if gen.Now != nil {
					socialCopy.LinkedIn.GeneratedAt = gen.Now().UTC()
				}
			}
			printSocialPreview(out, socialCopy)
		case "r", "regen", "regenerate":
			newCopy, genErr := gen.Generate(ctx, slug)
			if genErr != nil {
				return genErr
			}
			socialCopy = newCopy
			printSocialPreview(out, socialCopy)
		default:
			fmt.Fprintln(out, "Unknown option")
		}
	}
}
