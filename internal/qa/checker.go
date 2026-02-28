package qa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
)

const defaultQAModel = "gpt-4o-mini"

type Engine struct {
	Store       *content.Store
	ContentDir  string
	LLM         llm.LLMClient
	Model       string
	Temperature float64
}

type RunOptions struct {
	Slug    string
	AutoFix bool
	Approve bool
}

type RunResult struct {
	QA   *content.QAJSON
	Diff string
}

func (e *Engine) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if e.Store == nil {
		return nil, fmt.Errorf("content store is required")
	}
	if e.LLM == nil {
		return nil, fmt.Errorf("llm client is required")
	}
	slug := strings.TrimSpace(opts.Slug)
	if slug == "" {
		return nil, fmt.Errorf("slug is required")
	}
	contentDir := strings.TrimSpace(e.ContentDir)
	if contentDir == "" {
		contentDir = "content"
	}

	article, err := e.Store.ReadArticle(slug)
	if err != nil {
		return nil, err
	}
	article = strings.TrimSpace(article)
	voicePath := filepath.Join(contentDir, "voice.md")
	voiceBytes, err := os.ReadFile(voicePath)
	if err != nil {
		return nil, fmt.Errorf("read voice profile: %w", err)
	}
	voice := strings.TrimSpace(string(voiceBytes))

	checks, err := e.runChecks(ctx, article, voice)
	if err != nil {
		return nil, err
	}

	result := &RunResult{}
	if opts.AutoFix {
		model := strings.TrimSpace(e.Model)
		if model == "" {
			model = defaultQAModel
		}
		fixed, diff, fixMap, err := AutoFix(ctx, e.LLM, model, article, checks)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(diff) != "" {
			if err := e.Store.WriteArticle(slug, strings.TrimSpace(fixed)+"\n"); err != nil {
				return nil, err
			}
			article = fixed
			result.Diff = diff
		}
		for i := range checks {
			if fixes, ok := fixMap[checks[i].Name]; ok {
				checks[i].Fixes = fixes
			}
		}
	}

	passed := true
	for _, c := range checks {
		if !c.Passed {
			passed = false
			break
		}
	}
	qaDoc := &content.QAJSON{Checks: checks, Passed: passed, RunAt: time.Now().UTC()}
	if err := e.Store.WriteQA(slug, qaDoc); err != nil {
		return nil, err
	}
	result.QA = qaDoc

	if opts.Approve {
		if err := e.markApproved(slug); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (e *Engine) runChecks(ctx context.Context, article, voice string) ([]content.QACheck, error) {
	checks := []content.QACheck{
		CheckNoSecrets(article),
	}

	voiceCheck, err := e.runLLMCheck(ctx, "voice_consistency", buildVoicePrompt(article, voice))
	if err != nil {
		return nil, err
	}
	checks = append(checks, voiceCheck)

	accuracyCheck, err := e.runLLMCheck(ctx, "accuracy", buildAccuracyPrompt(article))
	if err != nil {
		return nil, err
	}
	checks = append(checks, accuracyCheck)

	checks = append(checks,
		CheckDashCleanup(article),
		CheckDedup(article),
		CheckReadingLevel(article),
		CheckLength(article),
	)
	return checks, nil
}

func (e *Engine) runLLMCheck(ctx context.Context, name, prompt string) (content.QACheck, error) {
	model := strings.TrimSpace(e.Model)
	if model == "" {
		model = defaultQAModel
	}
	resp, err := e.LLM.Complete(ctx, llm.Request{
		Model:       model,
		Temperature: e.Temperature,
		Messages: []llm.Message{
			{Role: "system", Content: "Return JSON array of issues. Return [] if no issues."},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return content.QACheck{}, fmt.Errorf("run %s check: %w", name, err)
	}
	check := checkLLMOutput(name, resp.Content)
	check.Name = name
	return check, nil
}

func (e *Engine) markApproved(slug string) error {
	meta, err := e.Store.Get(slug)
	if err != nil {
		return err
	}
	meta.Status = content.StatusQAPassed
	meta.UpdatedAt = time.Now().UTC()
	return e.Store.UpdateMeta(slug, meta)
}

func buildVoicePrompt(article, voice string) string {
	return "Compare the article against the voice profile and list passages that are inconsistent. Return [] if consistent.\n\nVoice:\n" + voice + "\n\nArticle:\n" + article
}

func buildAccuracyPrompt(article string) string {
	return "List claims that sound factual but lack supporting evidence, qualifiers, or clear sourcing. Return [] if none.\n\nArticle:\n" + article
}
