package ideas

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/kb"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/templates"
)

type mockLLMClient struct {
	lastReq  llm.Request
	response string
}

func (m *mockLLMClient) Complete(_ context.Context, req llm.Request) (*llm.Response, error) {
	m.lastReq = req
	return &llm.Response{Content: m.response}, nil
}

func (m *mockLLMClient) Stream(_ context.Context, _ llm.Request) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockLLMClient) Name() string { return "mock" }

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newTestGenerator(t *testing.T, contentDir string, client *mockLLMClient) *Generator {
	t.Helper()
	return NewGenerator(
		contentDir,
		client,
		kb.NewStore(contentDir),
		templates.NewEngine(contentDir),
		content.NewStore(contentDir),
	)
}

func sampleLLMOutlines() string {
	return strings.TrimSpace(`## Idea 1
### Working Title
Designing friction that creates momentum

### Core Paradox
You need constraints to stay creative.

### Transformation Arc
Stuck in endless options -> choose constraints -> consistent output.

### Key Examples
- A weekly publishing deadline.
- A fixed writing window.

### Actionable Steps
- Name one hard constraint.
- Publish before you feel ready.

## Idea 2
### Working Title
The clarity tax most experts avoid

### Core Paradox
Simpler language increases rigor.

### Transformation Arc
Complex language as shield -> plain language as test -> trusted authority.

### Key Examples
- Explaining to a junior teammate.
- Rewriting one paragraph at grade 8.

### Actionable Steps
- Cut one abstract noun per sentence.
- Replace claims with evidence.
`)
}

func TestGenerateIdeasFromKB(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, filepath.Join(contentDir, "voice.md"), "Write with directness.")
	writeFile(t, filepath.Join(contentDir, "blueprint.md"), "## Core Ideas\n- Prove by example")
	writeFile(t, filepath.Join(contentDir, "kb", "blogs", "source-a", "post.md"), "# KB Post\n\nA practical example from RSS.")
	writeFile(t, filepath.Join(contentDir, "kb", "notes", "note.md"), "# Note\n\nA field note from conversation.")

	client := &mockLLMClient{response: sampleLLMOutlines()}
	gen := newTestGenerator(t, contentDir, client)

	outlines, err := gen.Generate(context.Background(), GenerateOptions{FromKB: true, Count: 2})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(outlines) != 2 {
		t.Fatalf("len(outlines) = %d, want 2", len(outlines))
	}
	if outlines[0].Title == "" || outlines[0].CoreParadox == "" || outlines[0].TransformationArc == "" {
		t.Fatalf("expected parsed structured fields: %+v", outlines[0])
	}
	if len(outlines[0].KeyExamples) == 0 || len(outlines[0].ActionableSteps) == 0 {
		t.Fatalf("expected examples and steps parsed: %+v", outlines[0])
	}

	joined := strings.ToLower(client.lastReq.Messages[1].Content)
	if !strings.Contains(joined, "kb post") || !strings.Contains(joined, "field note") {
		t.Fatalf("expected KB sources in prompt, got: %s", client.lastReq.Messages[1].Content)
	}
}

func TestGenerateIdeasWithBlueprint(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, filepath.Join(contentDir, "voice.md"), "Voice profile")
	writeFile(t, filepath.Join(contentDir, "blueprint.md"), "# Intellectual Blueprint\n\n## Core Ideas\n- Leverage compounds")

	client := &mockLLMClient{response: sampleLLMOutlines()}
	gen := newTestGenerator(t, contentDir, client)

	_, err := gen.Generate(context.Background(), GenerateOptions{Count: 1})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	system := client.lastReq.Messages[0].Content
	if !strings.Contains(system, "BLUEPRINT CORE IDEAS") || !strings.Contains(system, "Leverage compounds") {
		t.Fatalf("expected blueprint core ideas in system context, got: %s", system)
	}
}

func TestSaveIdeasBatch(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, filepath.Join(contentDir, "voice.md"), "Voice profile")

	client := &mockLLMClient{response: sampleLLMOutlines()}
	gen := newTestGenerator(t, contentDir, client)
	gen.Now = func() time.Time {
		return time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC)
	}

	path, err := gen.SaveBatch([]Outline{{
		Title:             "Designing friction that creates momentum",
		CoreParadox:       "You need constraints to stay creative.",
		TransformationArc: "Stuck -> constraints -> output",
		KeyExamples:       []string{"A weekly publishing deadline"},
		ActionableSteps:   []string{"Name one hard constraint"},
	}})
	if err != nil {
		t.Fatalf("SaveBatch() error = %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("ideas", "2026-02-28-batch.md")) {
		t.Fatalf("batch path = %q", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read batch: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "# Idea Batch - 2026-02-28") || !strings.Contains(got, "## Idea 1: Designing friction that creates momentum") {
		t.Fatalf("unexpected batch format: %s", got)
	}
}

func TestPickerCreatesContentItem(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, filepath.Join(contentDir, "voice.md"), "Voice profile")

	client := &mockLLMClient{response: sampleLLMOutlines()}
	gen := newTestGenerator(t, contentDir, client)

	createdSlug, err := gen.PickAndCreate(strings.NewReader("1\n"), io.Discard, []Outline{{
		Title:             "Designing friction that creates momentum",
		CoreParadox:       "You need constraints to stay creative.",
		TransformationArc: "Stuck -> constraints -> output",
	}})
	if err != nil {
		t.Fatalf("PickAndCreate() error = %v", err)
	}
	if createdSlug != "designing-friction-that-creates-momentum" {
		t.Fatalf("created slug = %q", createdSlug)
	}
	if _, err := os.Stat(filepath.Join(contentDir, createdSlug, "meta.json")); err != nil {
		t.Fatalf("expected content item created: %v", err)
	}
}

func TestCountFlag(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, filepath.Join(contentDir, "voice.md"), "Voice profile")
	writeFile(t, filepath.Join(contentDir, "blueprint.md"), "Blueprint")

	client := &mockLLMClient{response: sampleLLMOutlines()}
	gen := newTestGenerator(t, contentDir, client)

	outlines, err := gen.Generate(context.Background(), GenerateOptions{Count: 2})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(outlines) != 2 {
		t.Fatalf("len(outlines) = %d, want 2", len(outlines))
	}
	if !strings.Contains(client.lastReq.Messages[1].Content, "Produce exactly 2 outlines.") {
		t.Fatalf("expected prompt count replacement, got: %s", client.lastReq.Messages[1].Content)
	}
}

func TestNoKBGraceful(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, filepath.Join(contentDir, "voice.md"), "Voice profile")
	writeFile(t, filepath.Join(contentDir, "blueprint.md"), "Blueprint")

	client := &mockLLMClient{response: sampleLLMOutlines()}
	gen := newTestGenerator(t, contentDir, client)

	outlines, err := gen.Generate(context.Background(), GenerateOptions{FromKB: true, Count: 1})
	if err != nil {
		t.Fatalf("Generate() should handle empty KB, got error: %v", err)
	}
	if len(outlines) != 1 {
		t.Fatalf("len(outlines) = %d, want 1", len(outlines))
	}
}
