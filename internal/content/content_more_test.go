package content

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFailsWhenContentDirIsFile(t *testing.T) {
	base := t.TempDir()
	filePath := filepath.Join(base, "content-file")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	s := NewStore(filePath)
	if err := s.Create("good-slug", "Title"); err == nil {
		t.Fatalf("expected create error when content dir is a file")
	}
}

func TestGetDecodeError(t *testing.T) {
	s := NewStore(t.TempDir())
	slug := "good-slug"
	if err := os.MkdirAll(s.SlugDir(slug), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.SlugDir(slug), "meta.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if _, err := s.Get(slug); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestListErrorAndBranches(t *testing.T) {
	base := t.TempDir()
	filePath := filepath.Join(base, "content-file")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := NewStore(filePath).List(nil); err == nil {
		t.Fatalf("expected list error for file content dir")
	}

	s := NewStore(filepath.Join(base, "content"))
	if err := os.MkdirAll(s.SlugDir("missing-meta"), 0o755); err != nil {
		t.Fatalf("mkdir missing-meta: %v", err)
	}
	mustCreate(t, s, "good-slug", "Title")
	items, err := s.List(nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("expected one valid item, len=%d err=%v", len(items), err)
	}

	if err := os.MkdirAll(s.SlugDir("broken-slug"), 0o755); err != nil {
		t.Fatalf("mkdir broken: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.SlugDir("broken-slug"), "meta.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write broken meta: %v", err)
	}
	if _, err := s.List(nil); err == nil {
		t.Fatalf("expected list error for broken meta json")
	}
}

func TestReadArticleAndQAErrorBranches(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "good-slug", "Title")
	if err := os.Remove(filepath.Join(s.SlugDir("good-slug"), "article.md")); err != nil {
		t.Fatalf("remove article: %v", err)
	}
	if err := os.Mkdir(filepath.Join(s.SlugDir("good-slug"), "article.md"), 0o755); err != nil {
		t.Fatalf("mkdir article dir: %v", err)
	}
	if _, err := s.ReadArticle("good-slug"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("expected non-notfound read article error, got %v", err)
	}

	if err := os.WriteFile(filepath.Join(s.SlugDir("good-slug"), "qa.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write qa bad json: %v", err)
	}
	if _, err := s.ReadQA("good-slug"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("expected decode read qa error, got %v", err)
	}
}

func TestTransitionBranches(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "good-slug", "Title")
	if err := s.Transition("good-slug", StatusPublished, true); err == nil {
		t.Fatalf("expected invalid transition with qa gate")
	}
	if err := s.Transition("good-slug", StatusPublished, false); err != nil {
		t.Fatalf("expected draft->published when qa gate false: %v", err)
	}
	meta, err := s.Get("good-slug")
	if err != nil {
		t.Fatalf("get after publish: %v", err)
	}
	if meta.PublishedAt == nil {
		t.Fatalf("published_at should be set")
	}
}

func TestUpdateMetaWriteFailure(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "good-slug", "Title")
	meta, err := s.Get("good-slug")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := os.Remove(filepath.Join(s.SlugDir("good-slug"), "meta.json")); err != nil {
		t.Fatalf("remove meta: %v", err)
	}
	if err := os.Mkdir(filepath.Join(s.SlugDir("good-slug"), "meta.json"), 0o755); err != nil {
		t.Fatalf("mkdir meta dir: %v", err)
	}
	if err := s.UpdateMeta("good-slug", meta); err == nil {
		t.Fatalf("expected update meta write failure")
	}
}
