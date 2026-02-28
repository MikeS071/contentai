package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAPIKey(t *testing.T) {
	clearAPIKeyCache()
	key, err := ResolveAPIKey("echo test-key")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if key != "test-key" {
		t.Fatalf("unexpected key: %q", key)
	}
}

func TestResolveAPIKeyCache(t *testing.T) {
	clearAPIKeyCache()
	cmd := fmt.Sprintf("f=%s; if [ -f \"$f\" ]; then echo second; else touch \"$f\"; echo first; fi", filepath.Join(t.TempDir(), "marker"))

	first, err := ResolveAPIKey(cmd)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	second, err := ResolveAPIKey(cmd)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if first != "first" || second != "first" {
		t.Fatalf("expected cached first value, got first=%q second=%q", first, second)
	}
}

func TestResolveAPIKeyEmptyCmd(t *testing.T) {
	clearAPIKeyCache()
	_, err := ResolveAPIKey("   ")
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestPromptHelpers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "prompt.md")
	mustWrite(t, p, "Hello {{name}} from {{place}}")

	content, err := LoadPrompt(p)
	if err != nil {
		t.Fatalf("load prompt: %v", err)
	}
	if content == "" {
		t.Fatal("expected prompt content")
	}
	rendered := RenderPrompt(content, map[string]string{"name": "Sam", "place": "ContentAI"})
	if rendered != "Hello Sam from ContentAI" {
		t.Fatalf("unexpected rendered prompt: %q", rendered)
	}

	rendered2, err := LoadAndRenderPrompt(p, map[string]string{"name": "Alex", "place": "CLI"})
	if err != nil {
		t.Fatalf("load and render: %v", err)
	}
	if rendered2 != "Hello Alex from CLI" {
		t.Fatalf("unexpected load+render output: %q", rendered2)
	}
}

func TestPromptHelpersLoadError(t *testing.T) {
	_, err := LoadPrompt(filepath.Join(t.TempDir(), "missing.md"))
	if err == nil {
		t.Fatal("expected load error")
	}
}

func TestHelperFunctions(t *testing.T) {
	if got := parseAPIError([]byte(`{"error":{"message":"rate limited"}}`)); got != "rate limited" {
		t.Fatalf("unexpected parsed api error: %q", got)
	}
	if got := parseAPIError([]byte("raw body")); got != "raw body" {
		t.Fatalf("unexpected raw parse api error: %q", got)
	}
	if got := parseAPIError(nil); got == "" {
		t.Fatal("expected non-empty error for empty body")
	}
	if !(backoff(1) > backoff(0)) {
		t.Fatal("expected increasing backoff")
	}
	if max(3, 2) != 3 || max(2, 3) != 3 {
		t.Fatal("max helper failed")
	}
	if estimateTokens("abcd") <= 0 {
		t.Fatal("estimateTokens should be positive")
	}
	if truncateToTokens("abcdef", 0) != "" {
		t.Fatal("truncateToTokens should return empty for zero limit")
	}
}

func TestExtractBlueprintCoreIdeas(t *testing.T) {
	bp := "# Blueprint\n## Mission\nx\n## Core Ideas\n- one\n- two\n## Closing\nz"
	out := extractBlueprintCoreIdeas(bp)
	if !strings.Contains(strings.ToLower(out), "core ideas") {
		t.Fatalf("expected core ideas section, got: %q", out)
	}

	noCore := "plain blueprint text"
	if got := extractBlueprintCoreIdeas(noCore); got != noCore {
		t.Fatalf("expected original blueprint when no core ideas section, got %q", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
