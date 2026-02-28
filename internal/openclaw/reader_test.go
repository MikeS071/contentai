package openclaw

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/contentai/internal/config"
)

func TestReadConversationHistory(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatalf("mkdir memory: %v", err)
	}
	mustWriteFile(t, filepath.Join(workspace, "memory", "2026-01.md"), "# Jan\nSignal one")
	mustWriteFile(t, filepath.Join(workspace, "memory", "2026-02.md"), "# Feb\nSignal two")
	mustWriteFile(t, filepath.Join(workspace, "channel-history.md"), "chat transcript")

	cfg := config.OpenClawConfig{
		Enabled:        true,
		Workspace:      workspace,
		ChannelHistory: true,
	}

	reader := NewReader(nil)
	got, err := reader.ReadConversationHistory(cfg)
	if err != nil {
		t.Fatalf("ReadConversationHistory() error = %v", err)
	}
	if !strings.Contains(got, "Signal one") || !strings.Contains(got, "Signal two") {
		t.Fatalf("conversation history missing memory snippets: %q", got)
	}
	if !strings.Contains(got, "chat transcript") {
		t.Fatalf("conversation history missing transcript: %q", got)
	}
}

func TestReadMemorySearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "creator economy" {
			t.Fatalf("query q = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results":[{"text":"memd snippet","source":"memory.md"}]}`)
	}))
	defer ts.Close()

	workspace := t.TempDir()
	cfg := config.OpenClawConfig{Enabled: true, Workspace: workspace}
	reader := NewReader(&http.Client{})
	reader.MemdURL = ts.URL

	results, err := reader.SearchMemory(context.Background(), cfg, "creator economy", 5)
	if err != nil {
		t.Fatalf("SearchMemory() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Text != "memd snippet" || results[0].Source != "memory.md" {
		t.Fatalf("unexpected memd result: %#v", results[0])
	}
}

func TestFallbackGrepSearch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatalf("mkdir memory: %v", err)
	}
	mustWriteFile(t, filepath.Join(workspace, "memory", "ideas.md"), "A note about creator economy patterns")
	mustWriteFile(t, filepath.Join(workspace, "memory", "other.md"), "Unrelated text")

	cfg := config.OpenClawConfig{Enabled: true, Workspace: workspace}
	reader := NewReader(&http.Client{})
	reader.MemdURL = "http://127.0.0.1:1"

	results, err := reader.SearchMemory(context.Background(), cfg, "creator economy", 5)
	if err != nil {
		t.Fatalf("SearchMemory() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected fallback results, got none")
	}
	if !strings.Contains(strings.ToLower(results[0].Text), "creator economy") {
		t.Fatalf("unexpected fallback snippet: %#v", results[0])
	}
}

func TestDisabledOpenClaw(t *testing.T) {
	cfg := config.OpenClawConfig{Enabled: false, Workspace: t.TempDir(), ChannelHistory: true}
	reader := NewReader(nil)

	history, err := reader.ReadConversationHistory(cfg)
	if err != nil {
		t.Fatalf("ReadConversationHistory() error = %v", err)
	}
	if history != "" {
		t.Fatalf("history = %q, want empty", history)
	}

	results, err := reader.SearchMemory(context.Background(), cfg, "anything", 5)
	if err != nil {
		t.Fatalf("SearchMemory() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results len = %d, want 0", len(results))
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
