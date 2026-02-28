package draft

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
)

type mockLLM struct {
	responses []*llm.Response
	requests  []llm.Request
}

func (m *mockLLM) Complete(_ context.Context, req llm.Request) (*llm.Response, error) {
	m.requests = append(m.requests, req)
	if len(m.responses) == 0 {
		return &llm.Response{Content: "ok"}, nil
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp, nil
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

func (m *mockTemplates) Get(name string) (string, error) {
	return m.data[name], nil
}

func TestDraftAssemblesFullContext(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "Voice benchmark")
	writeFixture(t, filepath.Join(contentDir, "blueprint.md"), "# Blueprint\n## Core Ideas\n- Core one")
	writeFixture(t, filepath.Join(contentDir, "examples", "1.md"), "Example A")
	writeFixture(t, filepath.Join(contentDir, "examples", "2.md"), "Example B")
	writeFixture(t, filepath.Join(store.SlugDir("alpha-post"), "article.md"), "Existing draft body")
	sourcePath := filepath.Join(t.TempDir(), "source.md")
	writeFixture(t, sourcePath, "Source transcript content")

	mllm := &mockLLM{responses: []*llm.Response{{Content: "insights"}, {Content: "article"}}}
	d := &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"creative-thought-partner": "CTP {{.Context}}\n{{.Source}}",
			"blog-writer":              "BW {{.VoiceProfile}} {{.CoreIdeas}} {{.Outline}} {{.Insights}}",
		}},
	}

	if err := d.Draft(ctx, Options{Slug: "alpha-post", SourcePath: sourcePath}); err != nil {
		t.Fatalf("Draft() error = %v", err)
	}

	if len(mllm.requests) != 2 {
		t.Fatalf("expected 2 llm requests, got %d", len(mllm.requests))
	}
	first := joinMessages(mllm.requests[0].Messages)
	second := joinMessages(mllm.requests[1].Messages)

	for _, want := range []string{"Voice benchmark", "Core one", "Example A", "Example B", "alpha post title", "Existing draft body", "Source transcript content"} {
		if !strings.Contains(first, want) {
			t.Fatalf("expected first prompt to include %q", want)
		}
		if !strings.Contains(second, want) {
			t.Fatalf("expected second prompt to include %q", want)
		}
	}
}

func TestDraftChainsTwoPrompts(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "voice")
	mllm := &mockLLM{responses: []*llm.Response{{Content: "insights-output"}, {Content: "final-article"}}}

	d := &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"creative-thought-partner": "creative-marker {{.Context}} {{.Source}}",
			"blog-writer":              "writer-marker {{.Insights}}",
		}},
	}

	if err := d.Draft(ctx, Options{Slug: "alpha-post"}); err != nil {
		t.Fatalf("Draft() error = %v", err)
	}
	if len(mllm.requests) != 2 {
		t.Fatalf("expected 2 prompt calls, got %d", len(mllm.requests))
	}
	if !strings.Contains(joinMessages(mllm.requests[0].Messages), "creative-marker") {
		t.Fatalf("first prompt was not creative thought partner")
	}
	if !strings.Contains(joinMessages(mllm.requests[1].Messages), "writer-marker") {
		t.Fatalf("second prompt was not blog writer")
	}
	if !strings.Contains(joinMessages(mllm.requests[1].Messages), "insights-output") {
		t.Fatalf("blog writer prompt missing creative output")
	}
}

func TestDraftSavesArticle(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "voice")
	mllm := &mockLLM{responses: []*llm.Response{{Content: "insights"}, {Content: "# Final Draft\n\nBody"}}}

	d := &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"creative-thought-partner": "{{.Context}}",
			"blog-writer":              "{{.Insights}}",
		}},
	}

	if err := d.Draft(ctx, Options{Slug: "alpha-post"}); err != nil {
		t.Fatalf("Draft() error = %v", err)
	}

	article, err := store.ReadArticle("alpha-post")
	if err != nil {
		t.Fatalf("ReadArticle() error = %v", err)
	}
	if strings.TrimSpace(article) != "# Final Draft\n\nBody" {
		t.Fatalf("unexpected saved article: %q", article)
	}
}

func TestDraftRefinesExisting(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "voice")
	writeFixture(t, filepath.Join(store.SlugDir("alpha-post"), "article.md"), "old draft sentence")
	mllm := &mockLLM{responses: []*llm.Response{{Content: "insights"}, {Content: "new article"}}}

	d := &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"creative-thought-partner": "{{.Context}}",
			"blog-writer":              "{{.Insights}}",
		}},
	}

	if err := d.Draft(ctx, Options{Slug: "alpha-post"}); err != nil {
		t.Fatalf("Draft() error = %v", err)
	}

	if !strings.Contains(joinMessages(mllm.requests[0].Messages), "old draft sentence") {
		t.Fatalf("expected existing article in context")
	}
}

func TestDraftWithSource(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "voice")
	sourcePath := filepath.Join(t.TempDir(), "source.md")
	writeFixture(t, sourcePath, "external source body")
	mllm := &mockLLM{responses: []*llm.Response{{Content: "insights"}, {Content: "article"}}}

	d := &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"creative-thought-partner": "{{.Context}} {{.Source}}",
			"blog-writer":              "{{.Insights}} {{.SourceMaterial}}",
		}},
	}

	if err := d.Draft(ctx, Options{Slug: "alpha-post", SourcePath: sourcePath}); err != nil {
		t.Fatalf("Draft() error = %v", err)
	}

	if !strings.Contains(joinMessages(mllm.requests[0].Messages), "external source body") {
		t.Fatalf("source content missing from creative prompt")
	}
	if !strings.Contains(joinMessages(mllm.requests[1].Messages), "external source body") {
		t.Fatalf("source content missing from writer prompt")
	}
}

func TestDraftInteractiveMode(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "voice")
	mllm := &mockLLM{responses: []*llm.Response{
		{Content: "What stake feels most urgent?"},
		{Content: "What concrete moment captures this?"},
		{Content: "## Extracted Insights\n- Insight one"},
		{Content: "# Final\n\nDraft body"},
	}}
	stdout := &bytes.Buffer{}

	d := &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"creative-thought-partner": "{{.Context}} {{.Source}}",
			"blog-writer":              "{{.Insights}}",
		}},
		Stdin:  strings.NewReader("Trust was dropping\nA customer call broke the pattern\n/done\n"),
		Stdout: stdout,
	}

	if err := d.Draft(ctx, Options{Slug: "alpha-post", Interactive: true}); err != nil {
		t.Fatalf("Draft() error = %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "What stake feels most urgent?") || !strings.Contains(got, "What concrete moment captures this?") {
		t.Fatalf("expected interactive questions in stdout, got: %q", got)
	}
	if len(mllm.requests) < 4 {
		t.Fatalf("expected multiple interactive llm calls, got %d", len(mllm.requests))
	}
	article, err := store.ReadArticle("alpha-post")
	if err != nil {
		t.Fatalf("ReadArticle() error = %v", err)
	}
	if !strings.Contains(article, "Draft body") {
		t.Fatalf("expected final article saved, got: %q", article)
	}
}

func TestDraftUpdatesMetaStatus(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "voice")
	mllm := &mockLLM{responses: []*llm.Response{{Content: "insights"}, {Content: "article"}}}

	meta, err := store.Get("alpha-post")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	meta.Status = content.StatusQAPassed
	before := meta.UpdatedAt
	if err := store.UpdateMeta("alpha-post", meta); err != nil {
		t.Fatalf("UpdateMeta() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	d := &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"creative-thought-partner": "{{.Context}}",
			"blog-writer":              "{{.Insights}}",
		}},
	}

	if err := d.Draft(ctx, Options{Slug: "alpha-post"}); err != nil {
		t.Fatalf("Draft() error = %v", err)
	}

	updated, err := store.Get("alpha-post")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status != content.StatusDraft {
		t.Fatalf("meta status = %q, want %q", updated.Status, content.StatusDraft)
	}
	if !updated.UpdatedAt.After(before) {
		t.Fatalf("expected updated_at to move forward")
	}
}

func TestDraftRequiresDependencies(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "voice")

	d := &Drafter{}
	if err := d.Draft(ctx, Options{Slug: "alpha-post"}); err == nil {
		t.Fatalf("expected dependency validation error")
	}

	d = &Drafter{Store: store}
	if err := d.Draft(ctx, Options{Slug: "alpha-post"}); err == nil {
		t.Fatalf("expected llm dependency validation error")
	}

	d = &Drafter{Store: store, LLM: &mockLLM{}}
	if err := d.Draft(ctx, Options{Slug: "alpha-post"}); err == nil {
		t.Fatalf("expected template dependency validation error")
	}

	d = &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        &mockLLM{},
		Templates:  &mockTemplates{data: map[string]string{"creative-thought-partner": "x", "blog-writer": "y"}},
	}
	if err := d.Draft(ctx, Options{}); err == nil {
		t.Fatalf("expected slug validation error")
	}
}

func TestAssembleContextMissingVoice(t *testing.T) {
	contentDir := filepath.Join(t.TempDir(), "content")
	store := content.NewStore(contentDir)
	if err := store.Create("alpha-post", "alpha post title"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := AssembleContext(store, "alpha-post", "", ""); err == nil {
		t.Fatalf("expected missing voice error")
	}
}

func TestDraftInteractiveDoneSynthesizes(t *testing.T) {
	ctx := context.Background()
	store, contentDir := newTestStore(t)
	writeFixture(t, filepath.Join(contentDir, "voice.md"), "voice")
	mllm := &mockLLM{responses: []*llm.Response{
		{Content: "What is the core tension?"},
		{Content: "## Extracted Insights\n- named insight"},
		{Content: "Final article"},
	}}
	stdout := &bytes.Buffer{}

	d := &Drafter{
		Store:      store,
		ContentDir: contentDir,
		LLM:        mllm,
		Templates: &mockTemplates{data: map[string]string{
			"creative-thought-partner": "{{.Context}}",
			"blog-writer":              "{{.Insights}}",
		}},
		Stdin:  strings.NewReader("/done\n"),
		Stdout: stdout,
	}

	if err := d.Draft(ctx, Options{Slug: "alpha-post", Interactive: true}); err != nil {
		t.Fatalf("Draft() error = %v", err)
	}
	if len(mllm.requests) != 3 {
		t.Fatalf("expected 3 llm calls (question, synthesis, writer), got %d", len(mllm.requests))
	}
	if got := stdout.String(); !strings.Contains(got, "core tension") {
		t.Fatalf("expected question output, got: %q", got)
	}
}

func newTestStore(t *testing.T) (*content.Store, string) {
	t.Helper()
	contentDir := filepath.Join(t.TempDir(), "content")
	if err := os.MkdirAll(filepath.Join(contentDir, "examples"), 0o755); err != nil {
		t.Fatalf("mkdir examples: %v", err)
	}
	store := content.NewStore(contentDir)
	if err := store.Create("alpha-post", "alpha post title"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return store, contentDir
}

func writeFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func joinMessages(msgs []llm.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		parts = append(parts, msg.Content)
	}
	return strings.Join(parts, "\n")
}
