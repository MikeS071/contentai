package initflow

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func newTestWizard(t *testing.T, dir string, input string, client *mockLLM) *Wizard {
	t.Helper()
	out := &bytes.Buffer{}
	w := &Wizard{
		Stdin:      strings.NewReader(input),
		Stdout:     out,
		WorkDir:    dir,
		Project:    "demo",
		LLM:        client,
		Templates:  &mockTemplates{data: map[string]string{"perspective-architect": "PA", "voice-extractor": "VE"}},
		APIKeyTest: true,
	}
	return w
}

func TestNewWizard(t *testing.T) {
	dir := t.TempDir()
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	client := &mockLLM{}
	engine := &mockTemplates{data: map[string]string{}}
	w := NewWizard(in, out, dir, "demo", client, engine)
	if w == nil {
		t.Fatal("NewWizard() returned nil")
	}
	if !w.APIKeyTest {
		t.Fatal("APIKeyTest should default to true")
	}
}

func TestWizardRunRequiresDependencies(t *testing.T) {
	err := (&Wizard{}).Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want dependency error")
	}
	if !strings.Contains(err.Error(), "stdin and stdout are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWizardRunRequiresWorkdir(t *testing.T) {
	w := &Wizard{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}}
	err := w.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "workdir is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWizardRunRequiresLLM(t *testing.T) {
	w := &Wizard{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, WorkDir: t.TempDir()}
	err := w.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "llm client is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWizardRunRequiresTemplates(t *testing.T) {
	w := &Wizard{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, WorkDir: t.TempDir(), LLM: &mockLLM{}}
	err := w.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "template engine is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWizardCreatesDirs(t *testing.T) {
	dir := t.TempDir()
	w := newTestWizard(t, dir, "Article\n---END---\n\nllm-key\n\n\n\n", &mockLLM{})

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	paths := []string{
		"content",
		"content/examples",
		"content/kb",
		"content/kb/blogs",
		"content/kb/notes",
	}
	for _, p := range paths {
		if st, err := os.Stat(filepath.Join(dir, p)); err != nil || !st.IsDir() {
			t.Fatalf("expected dir %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "contentai.toml")); err != nil {
		t.Fatalf("expected config file: %v", err)
	}
}

func TestWizardInlineMissingDelimiter(t *testing.T) {
	dir := t.TempDir()
	w := newTestWizard(t, dir, "Article without delimiter\n", &mockLLM{})
	if err := w.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want inline delimiter error")
	}
}

func TestWizardSavesExampleArticles(t *testing.T) {
	dir := t.TempDir()
	fromPath := filepath.Join(dir, "source.md")
	if err := os.WriteFile(fromPath, []byte("file article"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		fromPath,
		"inline article line 1",
		"inline article line 2",
		"---END---",
		"",
		"llm-key",
		"",
		"",
		"",
	}, "\n")
	w := newTestWizard(t, dir, input, &mockLLM{})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	first, err := os.ReadFile(filepath.Join(dir, "content/examples/1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(first)) != "file article" {
		t.Fatalf("example 1 mismatch: %q", string(first))
	}

	second, err := os.ReadFile(filepath.Join(dir, "content/examples/2.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(second), "inline article line 1") || !strings.Contains(string(second), "inline article line 2") {
		t.Fatalf("example 2 mismatch: %q", string(second))
	}
}

func TestWizardRunsPerspectiveArchitect(t *testing.T) {
	dir := t.TempDir()
	client := &mockLLM{responses: []*llm.Response{{Content: "blueprint output"}, {Content: "voice output"}, {Content: "ok"}}}
	w := newTestWizard(t, dir, "Article\n---END---\n\nllm-key\n\n\n\n", client)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	bp, err := os.ReadFile(filepath.Join(dir, "content/blueprint.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(bp)) != "blueprint output" {
		t.Fatalf("blueprint mismatch: %q", string(bp))
	}

	if len(client.requests) < 1 || client.requests[0].Messages[0].Content != "PA" {
		t.Fatalf("expected perspective prompt in first request, got %#v", client.requests)
	}
}

func TestWizardGeneratesVoiceProfile(t *testing.T) {
	dir := t.TempDir()
	client := &mockLLM{responses: []*llm.Response{{Content: "blueprint output"}, {Content: "voice output"}, {Content: "ok"}}}
	w := newTestWizard(t, dir, "Article\n---END---\n\nllm-key\n\n\n\n", client)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	voice, err := os.ReadFile(filepath.Join(dir, "content/voice.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(voice)) != "voice output" {
		t.Fatalf("voice mismatch: %q", string(voice))
	}

	if len(client.requests) < 2 || client.requests[1].Messages[0].Content != "VE" {
		t.Fatalf("expected voice prompt in second request, got %#v", client.requests)
	}
	if !strings.Contains(client.requests[1].Messages[1].Content, "blueprint output") {
		t.Fatalf("expected blueprint in voice context")
	}
}

func TestWizardConfiguresAPIKeys(t *testing.T) {
	dir := t.TempDir()
	client := &mockLLM{responses: []*llm.Response{{Content: "bp"}, {Content: "voice"}, {Content: "ok"}}}
	w := newTestWizard(t, dir, "Article\n---END---\n\nsk-llm-secret\nsk-img-secret\nhttps://publish.example.com\n\n", client)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	cfg, err := os.ReadFile(filepath.Join(dir, "contentai.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(cfg)
	if strings.Contains(text, "sk-llm-secret") || strings.Contains(text, "sk-img-secret") {
		t.Fatalf("raw keys leaked in config: %s", text)
	}
	if !strings.Contains(text, "api_key_cmd") {
		t.Fatalf("expected api_key_cmd in config: %s", text)
	}
}

func TestWizardSavesFeedsToml(t *testing.T) {
	dir := t.TempDir()
	input := "Article\n---END---\n\nllm-key\n\n\nhttps://a.example/rss\nhttps://b.example/feed\n\n"
	w := newTestWizard(t, dir, input, &mockLLM{})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	feeds, err := LoadFeeds(filepath.Join(dir, "content/kb/feeds.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds.Feeds) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(feeds.Feeds))
	}
	if feeds.Feeds[0] != "https://a.example/rss" || feeds.Feeds[1] != "https://b.example/feed" {
		t.Fatalf("unexpected feeds: %#v", feeds.Feeds)
	}
}

func TestWizardMinimumOneArticle(t *testing.T) {
	dir := t.TempDir()
	w := newTestWizard(t, dir, "\n", &mockLLM{})

	if err := w.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want minimum article error")
	}
}

func TestWizardIdempotent(t *testing.T) {
	dir := t.TempDir()
	first := newTestWizard(t, dir, "First article\n---END---\n\nllm-key\n\n\n\n", &mockLLM{})
	if err := first.Run(context.Background()); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}

	second := newTestWizard(t, dir, "Second article\n---END---\n\nllm-key\n\n\n\n", &mockLLM{})
	if err := second.Run(context.Background()); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	orig, err := os.ReadFile(filepath.Join(dir, "content/examples/1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(orig), "First article") {
		t.Fatalf("existing content overwritten: %q", string(orig))
	}
}
