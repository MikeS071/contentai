package templates

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGetEmbedded(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	got, err := e.Get("blog-writer")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	if !strings.Contains(got, "You are writing an article based on insights") {
		t.Fatalf("expected embedded template content, got: %q", got)
	}
}

func TestGetLocalOverride(t *testing.T) {
	t.Parallel()

	contentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contentDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(contentDir, "templates", "blog-writer.md")
	if err := os.WriteFile(path, []byte("local override"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	e := NewEngine(contentDir)
	got, err := e.Get("blog-writer")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != "local override" {
		t.Fatalf("expected local override, got: %q", got)
	}
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	if _, err := e.Get("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestGetInvalidName(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	if _, err := e.Get(""); err == nil {
		t.Fatal("expected error for empty template name")
	}
	if _, err := e.Get("../bad"); err == nil {
		t.Fatal("expected error for invalid template name")
	}
}

func TestGetWithVars(t *testing.T) {
	t.Parallel()

	contentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contentDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "templates", "custom.md"), []byte("Title: {{.Title}}"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	e := NewEngine(contentDir)
	got, err := e.GetWithVars("custom", map[string]interface{}{"Title": "Hello"})
	if err != nil {
		t.Fatalf("GetWithVars returned error: %v", err)
	}
	if got != "Title: Hello" {
		t.Fatalf("expected substituted output, got: %q", got)
	}
}

func TestGetWithVarsMissing(t *testing.T) {
	t.Parallel()

	contentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contentDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "templates", "custom.md"), []byte("Title: {{.Title}}"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	e := NewEngine(contentDir)
	if _, err := e.GetWithVars("custom", map[string]interface{}{}); err == nil {
		t.Fatal("expected missing variable error")
	}
}

func TestGetWithVarsParseError(t *testing.T) {
	t.Parallel()

	contentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contentDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "templates", "custom.md"), []byte("bad {{"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	e := NewEngine(contentDir)
	if _, err := e.GetWithVars("custom", map[string]interface{}{"Title": "Hello"}); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestList(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	got := e.List()
	expected := []string{
		"blog-writer",
		"creative-thought-partner",
		"deep-post-ideas",
		"hero-prompt",
		"perspective-architect",
		"qa-checklist",
		"social-copy",
		"voice-extractor",
	}

	if !slices.Equal(got, expected) {
		t.Fatalf("unexpected templates list: %#v", got)
	}
}

func TestExport(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	dest := t.TempDir()

	if err := e.Export(dest); err != nil {
		t.Fatalf("Export returned error: %v", err)
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 8 {
		t.Fatalf("expected 8 exported templates, got %d", len(entries))
	}

	exported, err := os.ReadFile(filepath.Join(dest, "blog-writer.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	embedded, err := e.Get("blog-writer")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(exported) != embedded {
		t.Fatalf("exported template mismatch")
	}
}

func TestExportNoOverwrite(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	dest := t.TempDir()
	target := filepath.Join(dest, "blog-writer.md")
	if err := os.WriteFile(target, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	report, err := e.export(dest, false)
	if err != nil {
		t.Fatalf("export returned error: %v", err)
	}
	if !slices.Contains(report.Skipped, "blog-writer") {
		t.Fatalf("expected blog-writer to be skipped, report: %#v", report)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != "keep me" {
		t.Fatalf("expected existing file to remain unchanged, got: %q", got)
	}
}

func TestExportForce(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	dest := t.TempDir()
	target := filepath.Join(dest, "blog-writer.md")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	report, err := e.export(dest, true)
	if err != nil {
		t.Fatalf("export returned error: %v", err)
	}
	if !slices.Contains(report.Exported, "blog-writer") {
		t.Fatalf("expected blog-writer to be exported, report: %#v", report)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) == "old" {
		t.Fatal("expected existing file to be overwritten")
	}
}

func TestExportWithForceWrapper(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	dest := t.TempDir()
	report, err := e.ExportWithForce(dest, false)
	if err != nil {
		t.Fatalf("ExportWithForce returned error: %v", err)
	}
	if len(report.Exported) != 8 {
		t.Fatalf("expected 8 exported templates, got %d", len(report.Exported))
	}
}

func TestExportDestinationIsFile(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	dest := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := e.Export(dest); err == nil {
		t.Fatal("expected error when export destination is a file")
	}
}

func TestIsOverridden(t *testing.T) {
	t.Parallel()

	contentDir := t.TempDir()
	e := NewEngine(contentDir)
	if e.IsOverridden("blog-writer") {
		t.Fatal("expected false when no local override exists")
	}

	if err := os.MkdirAll(filepath.Join(contentDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "templates", "blog-writer.md"), []byte("override"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if !e.IsOverridden("blog-writer") {
		t.Fatal("expected true when local override exists")
	}
}

func TestIsOverriddenInvalidName(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	if e.IsOverridden("../bad") {
		t.Fatal("expected false for invalid template name")
	}
}

func TestEmbeddedTemplatesExist(t *testing.T) {
	t.Parallel()

	expected := []string{
		"templates/blog-writer.md",
		"templates/creative-thought-partner.md",
		"templates/deep-post-ideas.md",
		"templates/hero-prompt.md",
		"templates/perspective-architect.md",
		"templates/qa-checklist.md",
		"templates/social-copy.md",
		"templates/voice-extractor.md",
	}

	for _, name := range expected {
		if _, err := fs.Stat(embeddedTemplates, name); err != nil {
			t.Fatalf("embedded template missing: %s (%v)", name, err)
		}
	}
}

func TestTemplateContentNotEmpty(t *testing.T) {
	t.Parallel()

	e := NewEngine(t.TempDir())
	for _, name := range e.List() {
		content, err := e.Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		if len(strings.TrimSpace(content)) <= 100 {
			t.Fatalf("template %q too short (%d chars)", name, len(strings.TrimSpace(content)))
		}
	}
}
