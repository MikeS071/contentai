package initflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadFeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feeds.toml")
	want := FeedsConfig{Feeds: []string{"https://one.example/rss", "https://two.example/rss"}}
	if err := SaveFeeds(path, want); err != nil {
		t.Fatalf("SaveFeeds() error = %v", err)
	}
	got, err := LoadFeeds(path)
	if err != nil {
		t.Fatalf("LoadFeeds() error = %v", err)
	}
	if len(got.Feeds) != len(want.Feeds) {
		t.Fatalf("feeds length = %d, want %d", len(got.Feeds), len(want.Feeds))
	}
	for i := range want.Feeds {
		if got.Feeds[i] != want.Feeds[i] {
			t.Fatalf("feeds[%d] = %q, want %q", i, got.Feeds[i], want.Feeds[i])
		}
	}
}

func TestLoadFeedsInvalidToml(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feeds.toml")
	if err := os.WriteFile(path, []byte("not = ["), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := LoadFeeds(path); err == nil {
		t.Fatal("LoadFeeds() error = nil, want parse error")
	}
}

func TestSaveFeedsMkdirError(t *testing.T) {
	base := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(base, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := SaveFeeds(filepath.Join(base, "feeds.toml"), FeedsConfig{}); err == nil {
		t.Fatal("SaveFeeds() error = nil, want mkdir error")
	}
}
