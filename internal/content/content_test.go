package content

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir())
}

func mustCreate(t *testing.T, s *Store, slug, title string) {
	t.Helper()
	if err := s.Create(slug, title); err != nil {
		t.Fatalf("create %q: %v", slug, err)
	}
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")

	meta, err := s.Get("hello-world")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if meta.Slug != "hello-world" {
		t.Fatalf("slug = %q, want hello-world", meta.Slug)
	}
	if meta.Title != "Hello World" {
		t.Fatalf("title = %q, want Hello World", meta.Title)
	}
	if meta.Status != StatusDraft {
		t.Fatalf("status = %q, want %q", meta.Status, StatusDraft)
	}
	if meta.CreatedAt.IsZero() || meta.UpdatedAt.IsZero() {
		t.Fatalf("timestamps should be set")
	}
}

func TestCreateDuplicate(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")

	err := s.Create("hello-world", "Another")
	if err == nil {
		t.Fatalf("expected duplicate create error")
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "first-post", "First")
	mustCreate(t, s, "second-post", "Second")
	mustCreate(t, s, "third-post", "Third")

	items, err := s.List(nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
}

func TestListFilterByStatus(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "first-post", "First")
	mustCreate(t, s, "second-post", "Second")
	mustCreate(t, s, "third-post", "Third")

	second, err := s.Get("second-post")
	if err != nil {
		t.Fatalf("get second: %v", err)
	}
	second.Status = StatusPublished
	if err := s.UpdateMeta("second-post", second); err != nil {
		t.Fatalf("update second: %v", err)
	}

	third, err := s.Get("third-post")
	if err != nil {
		t.Fatalf("get third: %v", err)
	}
	third.Status = StatusPublished
	if err := s.UpdateMeta("third-post", third); err != nil {
		t.Fatalf("update third: %v", err)
	}

	filter := StatusPublished
	items, err := s.List(&filter)
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	for _, item := range items {
		if item.Status != StatusPublished {
			t.Fatalf("status = %q, want %q", item.Status, StatusPublished)
		}
	}
}

func TestReadWriteArticle(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")
	want := "# Title\n\nBody"

	if err := s.WriteArticle("hello-world", want); err != nil {
		t.Fatalf("write article: %v", err)
	}
	got, err := s.ReadArticle("hello-world")
	if err != nil {
		t.Fatalf("read article: %v", err)
	}
	if got != want {
		t.Fatalf("article mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestReadWriteSocial(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")

	want := &SocialJSON{
		XText:        "x",
		LinkedInText: "li",
		XPostURL:     "https://x.example/post",
		LinkedInURL:  "https://linkedin.example/post",
	}
	if err := s.WriteSocial("hello-world", want); err != nil {
		t.Fatalf("write social: %v", err)
	}
	got, err := s.ReadSocial("hello-world")
	if err != nil {
		t.Fatalf("read social: %v", err)
	}
	if got.XText != want.XText || got.LinkedInText != want.LinkedInText || got.XPostURL != want.XPostURL || got.LinkedInURL != want.LinkedInURL {
		t.Fatalf("social mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestReadWriteQA(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")

	runAt := time.Now().UTC().Round(0)
	want := &QAJSON{
		Passed: true,
		RunAt:  runAt,
		Checks: []QACheck{{
			Name:   "no_secrets",
			Passed: true,
			Fixes: []QAFix{{
				Original: "old",
				Fixed:    "new",
				Applied:  true,
			}},
		}},
	}
	if err := s.WriteQA("hello-world", want); err != nil {
		t.Fatalf("write qa: %v", err)
	}
	got, err := s.ReadQA("hello-world")
	if err != nil {
		t.Fatalf("read qa: %v", err)
	}
	if got.Passed != want.Passed || !got.RunAt.Equal(want.RunAt) || len(got.Checks) != len(want.Checks) {
		t.Fatalf("qa mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestExists(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")

	if !s.Exists("hello-world") {
		t.Fatalf("expected exists true")
	}
	if s.Exists("does-not-exist") {
		t.Fatalf("expected exists false")
	}
}

func TestValidTransitionHappyPath(t *testing.T) {
	transitions := [][2]Status{
		{StatusDraft, StatusQAPassed},
		{StatusQAPassed, StatusDraft},
		{StatusQAPassed, StatusPublished},
		{StatusPublished, StatusSocialGenerated},
		{StatusSocialGenerated, StatusScheduled},
		{StatusScheduled, StatusPosted},
	}
	for _, tr := range transitions {
		if err := ValidTransition(tr[0], tr[1], true); err != nil {
			t.Fatalf("transition %q->%q should be valid: %v", tr[0], tr[1], err)
		}
	}
}

func TestValidTransitionInvalid(t *testing.T) {
	err := ValidTransition(StatusDraft, StatusPublished, true)
	if err == nil {
		t.Fatalf("expected invalid transition error")
	}
	if !strings.Contains(err.Error(), "invalid transition") {
		t.Fatalf("expected clear invalid transition message, got %q", err)
	}
}

func TestValidTransitionSkipQA(t *testing.T) {
	if err := ValidTransition(StatusDraft, StatusPublished, false); err != nil {
		t.Fatalf("draft->published should be allowed when qaGate=false: %v", err)
	}
}

func TestTransition(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")

	before, err := s.Get("hello-world")
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	beforeUpdated := before.UpdatedAt

	time.Sleep(5 * time.Millisecond)
	if err := s.Transition("hello-world", StatusQAPassed, true); err != nil {
		t.Fatalf("transition: %v", err)
	}
	after, err := s.Get("hello-world")
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if after.Status != StatusQAPassed {
		t.Fatalf("status = %q, want %q", after.Status, StatusQAPassed)
	}
	if !after.UpdatedAt.After(beforeUpdated) {
		t.Fatalf("updated_at should move forward")
	}
}

func TestSlugValidation(t *testing.T) {
	s := newTestStore(t)
	invalid := []string{
		"ab",
		"UPPER",
		"has space",
		"has_underscore",
		"bad!",
		strings.Repeat("a", 101),
	}
	for _, slug := range invalid {
		err := s.Create(slug, "Title")
		if err == nil {
			t.Fatalf("expected invalid slug error for %q", slug)
		}
	}

	if err := s.Create("good-slug-123", "Title"); err != nil {
		t.Fatalf("valid slug rejected: %v", err)
	}
}

func TestMetaSerialization(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")

	publishedAt := time.Now().UTC().Round(0)
	meta := &Meta{
		Title:       "Changed",
		Slug:        "hello-world",
		Summary:     "summary",
		Status:      StatusPublished,
		Category:    "engineering",
		CreatedAt:   time.Now().UTC().Add(-time.Hour).Round(0),
		UpdatedAt:   time.Now().UTC().Round(0),
		PublishedAt: &publishedAt,
		PublishURL:  "https://example.com/post",
	}
	if err := s.UpdateMeta("hello-world", meta); err != nil {
		t.Fatalf("update meta: %v", err)
	}
	got, err := s.Get("hello-world")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if got.Title != meta.Title || got.Summary != meta.Summary || got.Category != meta.Category || got.PublishURL != meta.PublishURL {
		t.Fatalf("meta fields mismatch\n got: %#v\nwant: %#v", got, meta)
	}
	if !got.CreatedAt.Equal(meta.CreatedAt) || !got.UpdatedAt.Equal(meta.UpdatedAt) {
		t.Fatalf("meta timestamps mismatch\n got: %#v\nwant: %#v", got, meta)
	}
	if got.PublishedAt == nil || !got.PublishedAt.Equal(*meta.PublishedAt) {
		t.Fatalf("published_at mismatch\n got: %#v\nwant: %#v", got, meta)
	}
}

func TestGetMissing(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
func TestHelpersAndParsers(t *testing.T) {
	s := newTestStore(t)
	if s.HeroPath("slug") != filepath.Join(s.ContentDir, "slug", "hero.png") {
		t.Fatalf("unexpected hero path")
	}
	if s.HeroLinkedInPath("slug") != filepath.Join(s.ContentDir, "slug", "hero-linkedin.png") {
		t.Fatalf("unexpected linkedin hero path")
	}
	if _, err := ParseStatus("bad"); err == nil {
		t.Fatalf("expected parse status error")
	}
	if _, err := ParseStatus(string(StatusDraft)); err != nil {
		t.Fatalf("expected valid parse status, got %v", err)
	}
}

func TestStoreErrorPaths(t *testing.T) {
	s := newTestStore(t)
	missing := "missing"
	if _, err := s.ReadArticle(missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for article, got %v", err)
	}
	if _, err := s.ReadSocial(missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for social, got %v", err)
	}
	if _, err := s.ReadQA(missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for qa, got %v", err)
	}
	if err := s.WriteSocial("x", nil); err == nil {
		t.Fatalf("expected nil social error")
	}
	if err := s.WriteQA("x", nil); err == nil {
		t.Fatalf("expected nil qa error")
	}
	if err := s.WriteArticle("x", "body"); err == nil {
		t.Fatalf("expected write article error for missing dir")
	}
	if _, err := s.List(nil); err != nil {
		t.Fatalf("list on missing content dir should not fail: %v", err)
	}
}

func TestUpdateMetaAndListBranches(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "first-post", "First")
	if err := s.UpdateMeta("first-post", nil); err == nil {
		t.Fatalf("expected nil meta error")
	}
	meta, err := s.Get("first-post")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	meta.Slug = "other"
	if err := s.UpdateMeta("first-post", meta); err == nil {
		t.Fatalf("expected slug mismatch error")
	}
	meta.Slug = "first-post"
	meta.Status = "bad"
	if err := s.UpdateMeta("first-post", meta); err == nil {
		t.Fatalf("expected invalid status error")
	}
	if err := os.MkdirAll(filepath.Join(s.ContentDir, "ideas"), 0o755); err != nil {
		t.Fatalf("mkdir ideas: %v", err)
	}
	items, err := s.List(nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("list should ignore non-slug dirs, len=%d err=%v", len(items), err)
	}
}

func TestInvalidTransitionAndDecodeError(t *testing.T) {
	if err := ValidTransition("unknown", StatusDraft, true); err == nil {
		t.Fatalf("expected invalid from status")
	}
	if err := ValidTransition(StatusDraft, "unknown", true); err == nil {
		t.Fatalf("expected invalid to status")
	}
	if err := ValidTransition(StatusDraft, StatusDraft, true); err == nil {
		t.Fatalf("expected same-status transition error")
	}
	s := newTestStore(t)
	mustCreate(t, s, "hello-world", "Hello World")
	if err := os.WriteFile(filepath.Join(s.SlugDir("hello-world"), "social.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write bad social: %v", err)
	}
	if _, err := s.ReadSocial("hello-world"); err == nil {
		t.Fatalf("expected decode error")
	}
}
