package draft

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
)

const defaultDraftModel = "gpt-4o-mini"

type templateGetter interface {
	Get(name string) (string, error)
}

type Options struct {
	Slug         string
	SourcePath   string
	Interactive  bool
	Conversation string
}

type Drafter struct {
	Store      *content.Store
	ContentDir string
	LLM        llm.LLMClient
	Templates  templateGetter
	Stdin      io.Reader
	Stdout     io.Writer

	Model       string
	Temperature float64
}

func (d *Drafter) Draft(ctx context.Context, opts Options) error {
	if d.Store == nil {
		return fmt.Errorf("content store is required")
	}
	if d.LLM == nil {
		return fmt.Errorf("llm client is required")
	}
	if d.Templates == nil {
		return fmt.Errorf("template getter is required")
	}
	slug := strings.TrimSpace(opts.Slug)
	if slug == "" {
		return fmt.Errorf("slug is required")
	}

	draftCtx, err := AssembleContext(d.Store, slug, opts.SourcePath, opts.Conversation)
	if err != nil {
		return err
	}

	creativeTemplate, err := d.Templates.Get("creative-thought-partner")
	if err != nil {
		return fmt.Errorf("load creative-thought-partner template: %w", err)
	}
	blogTemplate, err := d.Templates.Get("blog-writer")
	if err != nil {
		return fmt.Errorf("load blog-writer template: %w", err)
	}

	insights, err := d.runCreativeStage(ctx, opts.Interactive, creativeTemplate, draftCtx)
	if err != nil {
		return err
	}

	blogPrompt, err := renderTemplate(blogTemplate, map[string]string{
		"VoiceProfile":   draftCtx.VoiceProfile,
		"CoreIdeas":      draftCtx.CoreIdeas(),
		"Outline":        draftCtx.Outline(),
		"Insights":       strings.TrimSpace(insights),
		"Context":        draftCtx.FullText(),
		"Source":         draftCtx.Source,
		"SourceMaterial": draftCtx.Source,
	})
	if err != nil {
		return fmt.Errorf("render blog-writer template: %w", err)
	}

	articleResp, err := d.LLM.Complete(ctx, d.request([]llm.Message{
		{Role: "system", Content: blogPrompt},
		{Role: "user", Content: "Use this full context while writing:\n\n" + draftCtx.FullText() + "\n\nCreative insights:\n" + strings.TrimSpace(insights)},
	}))
	if err != nil {
		return fmt.Errorf("run blog writer prompt: %w", err)
	}

	article := strings.TrimSpace(articleResp.Content)
	if err := d.Store.WriteArticle(slug, article+"\n"); err != nil {
		return err
	}

	meta, err := d.Store.Get(slug)
	if err != nil {
		return err
	}
	meta.Status = content.StatusDraft
	meta.UpdatedAt = time.Now().UTC()
	if err := d.Store.UpdateMeta(slug, meta); err != nil {
		return err
	}

	return nil
}

func (d *Drafter) runCreativeStage(ctx context.Context, interactive bool, creativeTemplate string, draftCtx *Context) (string, error) {
	rendered, err := renderTemplate(creativeTemplate, map[string]string{
		"Context": draftCtx.FullText(),
		"Source":  draftCtx.Source,
	})
	if err != nil {
		return "", fmt.Errorf("render creative-thought-partner template: %w", err)
	}

	if !interactive {
		resp, err := d.LLM.Complete(ctx, d.request([]llm.Message{
			{Role: "system", Content: rendered},
			{Role: "user", Content: "Mine insights from this context:\n\n" + draftCtx.FullText()},
		}))
		if err != nil {
			return "", fmt.Errorf("run creative thought partner prompt: %w", err)
		}
		return strings.TrimSpace(resp.Content), nil
	}

	return d.runInteractiveCreative(ctx, rendered, draftCtx)
}

func (d *Drafter) runInteractiveCreative(ctx context.Context, renderedPrompt string, draftCtx *Context) (string, error) {
	stdin := d.Stdin
	stdout := d.Stdout
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stdout == nil {
		stdout = io.Discard
	}

	history := []llm.Message{
		{Role: "system", Content: renderedPrompt},
		{Role: "user", Content: "Start the conversation with one sharp framing question. Use this context:\n\n" + draftCtx.FullText()},
	}

	fmt.Fprintln(stdout, "Interactive mode: respond to prompts. Type /done when ready to synthesize.")
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var assistant string
	for round := 0; round < 8; round++ {
		resp, err := d.LLM.Complete(ctx, d.request(history))
		if err != nil {
			return "", fmt.Errorf("run interactive creative thought partner: %w", err)
		}
		assistant = strings.TrimSpace(resp.Content)
		if assistant == "" {
			return "", fmt.Errorf("interactive creative thought partner returned empty response")
		}

		history = append(history, llm.Message{Role: "assistant", Content: assistant})
		fmt.Fprintln(stdout, assistant)
		if strings.Contains(strings.ToLower(assistant), "extracted insights") {
			return assistant, nil
		}

		fmt.Fprint(stdout, "> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.EqualFold(line, "/done") || strings.EqualFold(line, "done") {
			break
		}
		history = append(history, llm.Message{Role: "user", Content: line})
	}

	history = append(history, llm.Message{Role: "user", Content: "Synthesize now. Return only the required extracted insights sections."})
	resp, err := d.LLM.Complete(ctx, d.request(history))
	if err != nil {
		return "", fmt.Errorf("synthesize creative insights: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}

func (d *Drafter) request(messages []llm.Message) llm.Request {
	model := strings.TrimSpace(d.Model)
	if model == "" {
		model = defaultDraftModel
	}
	return llm.Request{
		Model:       model,
		Temperature: d.Temperature,
		Messages:    messages,
	}
}

func renderTemplate(tmpl string, vars map[string]string) (string, error) {
	t, err := template.New("draft-template").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, vars); err != nil {
		return "", err
	}
	return b.String(), nil
}
