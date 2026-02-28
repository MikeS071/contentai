package qa

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
)

func AutoFix(ctx context.Context, client llm.LLMClient, model, article string, checks []content.QACheck) (string, string, map[string][]content.QAFix, error) {
	failing := make([]content.QACheck, 0)
	for _, c := range checks {
		if !c.Passed {
			failing = append(failing, c)
		}
	}
	if len(failing) == 0 {
		return article, "", map[string][]content.QAFix{}, nil
	}
	if client == nil {
		return "", "", nil, fmt.Errorf("llm client is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultQAModel
	}

	var issues strings.Builder
	for _, c := range failing {
		issues.WriteString("- ")
		issues.WriteString(c.Name)
		for _, issue := range c.Issues {
			issues.WriteString("\n  - ")
			issues.WriteString(issue)
		}
		issues.WriteString("\n")
	}

	prompt := "Rewrite the article to resolve only these QA issues while preserving intent and structure. Return only the revised article.\n\nIssues:\n" + strings.TrimSpace(issues.String())
	resp, err := client.Complete(ctx, llm.Request{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: "You are a meticulous editor. Preserve voice and meaning while fixing concrete QA issues."},
			{Role: "user", Content: prompt + "\n\nArticle:\n" + article},
		},
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("run qa autofix: %w", err)
	}

	fixed := strings.TrimSpace(resp.Content)
	if fixed == "" {
		fixed = strings.TrimSpace(article)
	}
	d := buildSimpleDiff(article, fixed)

	fixes := make(map[string][]content.QAFix, len(failing))
	for _, c := range failing {
		fixes[c.Name] = []content.QAFix{{
			Original: article,
			Fixed:    fixed,
			Applied:  true,
		}}
	}

	return fixed, d, fixes, nil
}

func buildSimpleDiff(before, after string) string {
	if before == after {
		return ""
	}
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	maxLen := len(beforeLines)
	if len(afterLines) > maxLen {
		maxLen = len(afterLines)
	}

	var b strings.Builder
	b.WriteString("--- original\n")
	b.WriteString("+++ revised\n")
	for i := 0; i < maxLen; i++ {
		var oldLine, newLine string
		if i < len(beforeLines) {
			oldLine = beforeLines[i]
		}
		if i < len(afterLines) {
			newLine = afterLines[i]
		}
		if oldLine == newLine {
			continue
		}
		if oldLine != "" {
			b.WriteString("-")
			b.WriteString(oldLine)
			b.WriteString("\n")
		}
		if newLine != "" {
			b.WriteString("+")
			b.WriteString(newLine)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
