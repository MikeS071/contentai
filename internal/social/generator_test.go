package social

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
)

type mockLLM struct {
	responses []string
	requests  []llm.Request
	err       error
}

func (m *mockLLM) Complete(_ context.Context, req llm.Request) (*llm.Response, error) {
	m.requests = append(m.requests, req)
	if m.err != nil {
		return nil, m.err
	}
	if len(m.responses) == 0 {
		return &llm.Response{Content: "default response"}, nil
	}
	out := m.responses[0]
	m.responses = m.responses[1:]
	return &llm.Response{Content: out}, nil
}

func (m *mockLLM) Stream(_ context.Context, _ llm.Request) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockLLM) Name() string { return "mock" }

type mockTemplates struct {
	data map[string]string
}

func (m *mockTemplates) GetWithVars(name string, vars map[string]any) (string, error) {
	tpl, ok := m.data[name]
	if !ok {
		return "", errors.New("template not found")
	}
	tpl = strings.ReplaceAll(tpl, "{{.VoiceProfile}}", strings.TrimSpace(vars["VoiceProfile"].(string)))
	tpl = strings.ReplaceAll(tpl, "{{.Article}}", strings.TrimSpace(vars["Article"].(string)))
	tpl = strings.ReplaceAll(tpl, "{{.URL}}", strings.TrimSpace(vars["URL"].(string)))
	return tpl, nil
}

func TestGenerateXCopy(t *testing.T) {
	gen, _, _ := setupSocialGenerator(t, []string{"Short hook + insight " + ArticleLinkPlaceholder, "#BuildInPublic\n\nLonger LinkedIn post"})

	result, err := gen.Generate(context.Background(), "launch-post")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(result.X.Text) > 280 {
		t.Fatalf("expected x copy <= 280 chars, got %d", len(result.X.Text))
	}
	if !strings.Contains(result.X.Text, ArticleLinkPlaceholder) {
		t.Fatalf("expected x copy to include link placeholder, got %q", result.X.Text)
	}
}

func TestGenerateLinkedInCopy(t *testing.T) {
	gen, _, _ := setupSocialGenerator(t, []string{"x text " + ArticleLinkPlaceholder, "Paragraph one.\n\nParagraph two with details.\n\n#ContentMarketing #AI"})

	result, err := gen.Generate(context.Background(), "launch-post")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(strings.Fields(result.LinkedIn.Text)) < 8 {
		t.Fatalf("expected longer linkedin copy, got %q", result.LinkedIn.Text)
	}
	if !strings.Contains(result.LinkedIn.Text, "#") {
		t.Fatalf("expected linkedin copy to include hashtags, got %q", result.LinkedIn.Text)
	}
}

func TestSavesSocialJson(t *testing.T) {
	gen, store, _ := setupSocialGenerator(t, []string{"x text " + ArticleLinkPlaceholder, "LinkedIn paragraph.\n\n#Growth"})

	result, err := gen.Generate(context.Background(), "launch-post")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(store.SlugDir("launch-post"), "social.json"))
	if err != nil {
		t.Fatalf("read social.json: %v", err)
	}
	if !strings.Contains(string(raw), `"x"`) || !strings.Contains(string(raw), `"linkedin"`) {
		t.Fatalf("expected nested social payload, got: %s", string(raw))
	}
	if !strings.Contains(string(raw), `"generated_at"`) {
		t.Fatalf("expected generated_at fields, got: %s", string(raw))
	}
	if !strings.Contains(string(raw), result.X.Text) {
		t.Fatalf("social.json should include generated X text")
	}
}

func TestVoiceIncluded(t *testing.T) {
	gen, _, mllm := setupSocialGenerator(t, []string{"x text " + ArticleLinkPlaceholder, "LinkedIn paragraph.\n\n#Growth"})

	if _, err := gen.Generate(context.Background(), "launch-post"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mllm.requests) < 1 {
		t.Fatalf("expected at least one llm request")
	}
	joined := joinMessages(mllm.requests[0].Messages)
	if !strings.Contains(joined, "Write with high signal and direct language") {
		t.Fatalf("expected voice profile in llm context, got %q", joined)
	}
}

func TestRequiresPublishedArticle(t *testing.T) {
	gen, store, _ := setupSocialGenerator(t, []string{"x text", "linkedin"})

	meta, err := store.Get("launch-post")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	meta.Status = content.StatusQAPassed
	meta.PublishURL = ""
	meta.UpdatedAt = time.Now().UTC()
	if err := store.UpdateMeta("launch-post", meta); err != nil {
		t.Fatalf("update meta: %v", err)
	}

	_, err = gen.Generate(context.Background(), "launch-post")
	if err == nil || !strings.Contains(err.Error(), "published") {
		t.Fatalf("expected published requirement error, got %v", err)
	}
}

func TestGenerateTruncatesXAndAddsLinkedInTags(t *testing.T) {
	long := strings.Repeat("insight ", 70)
	gen, _, _ := setupSocialGenerator(t, []string{long, "LinkedIn paragraph without tags"})

	result, err := gen.Generate(context.Background(), "launch-post")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(result.X.Text) > 280 {
		t.Fatalf("expected truncated x copy <= 280 chars, got %d", len(result.X.Text))
	}
	if !strings.Contains(result.X.Text, ArticleLinkPlaceholder) {
		t.Fatalf("expected x copy link placeholder, got %q", result.X.Text)
	}
	if !strings.Contains(result.LinkedIn.Text, "#") {
		t.Fatalf("expected generated linkedin hashtags, got %q", result.LinkedIn.Text)
	}
}

func TestGenerateRequiresVoiceProfile(t *testing.T) {
	gen, _, _ := setupSocialGenerator(t, []string{"x text", "linkedin"})
	if err := os.Remove(filepath.Join(gen.ContentDir, "voice.md")); err != nil {
		t.Fatalf("remove voice: %v", err)
	}

	if _, err := gen.Generate(context.Background(), "launch-post"); err == nil || !strings.Contains(err.Error(), "voice profile") {
		t.Fatalf("expected missing voice profile error, got %v", err)
	}
}

func TestSaveValidation(t *testing.T) {
	gen, _, _ := setupSocialGenerator(t, []string{"x text", "linkedin"})

	if err := gen.Save("launch-post", nil); err == nil {
		t.Fatalf("expected nil payload error")
	}
	if err := (&Generator{}).Save("launch-post", &SocialJSON{}); err == nil {
		t.Fatalf("expected missing store error")
	}
}

func TestNewGeneratorDefaults(t *testing.T) {
	gen := NewGenerator("", &mockLLM{}, nil, nil)
	if gen.Store == nil || gen.Templates == nil || gen.Now == nil {
		t.Fatalf("expected defaults to be populated")
	}
	if gen.Model != "gpt-4o-mini" {
		t.Fatalf("expected default model, got %q", gen.Model)
	}
}

func TestGenerateValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		gen  *Generator
		slug string
	}{
		{name: "nil generator", gen: nil, slug: "x"},
		{name: "missing store", gen: &Generator{LLM: &mockLLM{}, Templates: &mockTemplates{}}, slug: "x"},
		{name: "missing llm", gen: &Generator{Store: content.NewStore(t.TempDir()), Templates: &mockTemplates{}}, slug: "x"},
		{name: "missing templates", gen: &Generator{Store: content.NewStore(t.TempDir()), LLM: &mockLLM{}}, slug: "x"},
		{name: "missing slug", gen: &Generator{Store: content.NewStore(t.TempDir()), LLM: &mockLLM{}, Templates: &mockTemplates{}}, slug: " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.gen != nil && tc.gen.ContentDir == "" {
				tc.gen.ContentDir = t.TempDir()
			}
			if _, err := tc.gen.Generate(context.Background(), tc.slug); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestGenerateLLMError(t *testing.T) {
	gen, _, mllm := setupSocialGenerator(t, []string{"x text", "linkedin"})
	mllm.err = errors.New("llm down")
	if _, err := gen.Generate(context.Background(), "launch-post"); err == nil || !strings.Contains(err.Error(), "generate x copy") {
		t.Fatalf("expected llm error, got %v", err)
	}
}

func setupSocialGenerator(t *testing.T, responses []string) (*Generator, *content.Store, *mockLLM) {
	t.Helper()
	contentDir := t.TempDir()
	store := content.NewStore(contentDir)
	if err := store.Create("launch-post", "Launch Post"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.WriteArticle("launch-post", "# Launch\n\nThis is the article body."); err != nil {
		t.Fatalf("write article: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "voice.md"), []byte("Write with high signal and direct language"), 0o644); err != nil {
		t.Fatalf("write voice: %v", err)
	}
	meta, err := store.Get("launch-post")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	meta.Status = content.StatusPublished
	meta.PublishURL = "https://example.com/launch-post"
	meta.UpdatedAt = time.Now().UTC()
	if err := store.UpdateMeta("launch-post", meta); err != nil {
		t.Fatalf("update meta: %v", err)
	}

	mllm := &mockLLM{responses: responses}
	gen := &Generator{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"social-copy": "VOICE={{.VoiceProfile}}\nARTICLE={{.Article}}\nURL={{.URL}}",
		}},
		Model: "gpt-4o-mini",
		Now:   func() time.Time { return time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC) },
	}
	return gen, store, mllm
}

func joinMessages(msgs []llm.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		parts = append(parts, msg.Role+": "+msg.Content)
	}
	return strings.Join(parts, "\n")
}
