package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/templates"
)

const (
	defaultSocialModel     = "gpt-4o-mini"
	ArticleLinkPlaceholder = "{{ARTICLE_URL}}"
)

type templateRenderer interface {
	GetWithVars(name string, vars map[string]any) (string, error)
}

type PlatformCopy struct {
	Text        string    `json:"text"`
	GeneratedAt time.Time `json:"generated_at"`
}

type SocialJSON struct {
	X        PlatformCopy `json:"x"`
	LinkedIn PlatformCopy `json:"linkedin"`
}

type Generator struct {
	Store      *content.Store
	ContentDir string
	LLM        llm.LLMClient
	Templates  templateRenderer
	Model      string
	Now        func() time.Time
}

func NewGenerator(contentDir string, client llm.LLMClient, tpl templateRenderer, store *content.Store) *Generator {
	if strings.TrimSpace(contentDir) == "" {
		contentDir = "content"
	}
	if tpl == nil {
		tpl = templates.NewEngine(contentDir)
	}
	if store == nil {
		store = content.NewStore(contentDir)
	}
	return &Generator{
		Store:      store,
		ContentDir: contentDir,
		LLM:        client,
		Templates:  tpl,
		Model:      defaultSocialModel,
		Now:        time.Now,
	}
}

func (g *Generator) Generate(ctx context.Context, slug string) (*SocialJSON, error) {
	if g == nil {
		return nil, errors.New("generator is nil")
	}
	if g.Store == nil {
		return nil, errors.New("content store is required")
	}
	if g.LLM == nil {
		return nil, errors.New("llm client is required")
	}
	if g.Templates == nil {
		return nil, errors.New("template renderer is required")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, errors.New("slug is required")
	}

	meta, err := g.Store.Get(slug)
	if err != nil {
		return nil, err
	}
	if meta.Status != content.StatusPublished || strings.TrimSpace(meta.PublishURL) == "" {
		return nil, fmt.Errorf("cannot generate social copy: article must be published and include a publish URL")
	}

	article, err := g.Store.ReadArticle(slug)
	if err != nil {
		return nil, err
	}
	voice, err := g.readVoice()
	if err != nil {
		return nil, err
	}

	basePrompt, err := g.Templates.GetWithVars("social-copy", map[string]any{
		"Article":      strings.TrimSpace(article),
		"VoiceProfile": voice,
		"URL":          strings.TrimSpace(meta.PublishURL),
	})
	if err != nil {
		return nil, fmt.Errorf("load social-copy template: %w", err)
	}

	xRaw, err := g.complete(ctx, basePrompt, "Write only X post copy. Constraints: <=280 chars, one compact paragraph, include the placeholder "+ArticleLinkPlaceholder+" exactly once.")
	if err != nil {
		return nil, fmt.Errorf("generate x copy: %w", err)
	}
	liRaw, err := g.complete(ctx, basePrompt, "Write only LinkedIn copy. Constraints: 1-3 short paragraphs, professional tone, include 1-3 relevant hashtags, end with "+ArticleLinkPlaceholder+".")
	if err != nil {
		return nil, fmt.Errorf("generate linkedin copy: %w", err)
	}

	now := time.Now().UTC()
	if g.Now != nil {
		now = g.Now().UTC()
	}
	social := &SocialJSON{
		X: PlatformCopy{
			Text:        normalizeX(xRaw),
			GeneratedAt: now,
		},
		LinkedIn: PlatformCopy{
			Text:        normalizeLinkedIn(liRaw),
			GeneratedAt: now,
		},
	}
	if err := g.Save(slug, social); err != nil {
		return nil, err
	}
	meta.Status = content.StatusSocialGenerated
	meta.UpdatedAt = now
	if err := g.Store.UpdateMeta(slug, meta); err != nil {
		return nil, err
	}
	return social, nil
}

func (g *Generator) Save(slug string, social *SocialJSON) error {
	if g == nil || g.Store == nil {
		return errors.New("content store is required")
	}
	if social == nil {
		return errors.New("social payload is required")
	}
	path := filepath.Join(g.Store.SlugDir(slug), "social.json")
	raw, err := json.MarshalIndent(social, "", "  ")
	if err != nil {
		return fmt.Errorf("encode social.json: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write social.json: %w", err)
	}
	return nil
}

func (g *Generator) complete(ctx context.Context, prompt, task string) (string, error) {
	model := strings.TrimSpace(g.Model)
	if model == "" {
		model = defaultSocialModel
	}
	resp, err := g.LLM.Complete(ctx, llm.Request{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: task},
		},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func (g *Generator) readVoice() (string, error) {
	path := filepath.Join(g.ContentDir, "voice.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read voice profile: %w", err)
	}
	voice := strings.TrimSpace(string(data))
	if voice == "" {
		return "", errors.New("voice profile is empty")
	}
	return voice, nil
}

func normalizeX(raw string) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if text == "" {
		text = "New article is live. Key insight inside. " + ArticleLinkPlaceholder
	}
	if !strings.Contains(text, ArticleLinkPlaceholder) {
		text = strings.TrimSpace(text + " " + ArticleLinkPlaceholder)
	}
	if len(text) <= 280 {
		return text
	}

	withoutLink := strings.TrimSpace(strings.ReplaceAll(text, ArticleLinkPlaceholder, ""))
	maxRunes := 280 - len(ArticleLinkPlaceholder) - 1
	if maxRunes < 0 {
		return ArticleLinkPlaceholder
	}
	r := []rune(withoutLink)
	if len(r) > maxRunes {
		withoutLink = strings.TrimSpace(string(r[:maxRunes]))
	}
	return strings.TrimSpace(withoutLink + " " + ArticleLinkPlaceholder)
}

func normalizeLinkedIn(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		text = "New article is live.\n\nIt breaks down one practical insight you can apply this week.\n\n#Leadership #AI"
	}
	if !strings.Contains(text, "#") {
		text = strings.TrimSpace(text) + "\n\n#Leadership #AI"
	}
	if !strings.Contains(text, ArticleLinkPlaceholder) {
		text = strings.TrimSpace(text) + "\n\n" + ArticleLinkPlaceholder
	}
	return text
}
