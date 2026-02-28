package publish

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/content"
)

type capturePublisher struct {
	calls int
	item  PublishItem
	res   PublishResult
	err   error
}

func (p *capturePublisher) Publish(_ context.Context, item PublishItem) (PublishResult, error) {
	p.calls++
	p.item = item
	if p.err != nil {
		return PublishResult{}, p.err
	}
	return p.res, nil
}

func newPublishTestStore(t *testing.T) *content.Store {
	t.Helper()
	s := content.NewStore(t.TempDir())
	if err := s.Create("hello-world", "Hello World"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.WriteArticle("hello-world", "# Heading\n\nBody"); err != nil {
		t.Fatalf("write article: %v", err)
	}
	meta, err := s.Get("hello-world")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	meta.Status = content.StatusQAPassed
	meta.Summary = "summary"
	if err := s.UpdateMeta("hello-world", meta); err != nil {
		t.Fatalf("update meta: %v", err)
	}
	return s
}

func TestHTTPPublish(t *testing.T) {
	var (
		gotMethod string
		gotBody   map[string]any
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"url":"https://site.example/posts/hello-world"},"id":"42"}`))
	}))
	t.Cleanup(ts.Close)

	p := NewHTTPPublisher(HTTPConfig{
		URL:             ts.URL,
		FieldMap:        map[string]string{"title": "title", "body": "content", "slug": "slug"},
		ResponseURLPath: "data.url",
	})

	res, err := p.Publish(context.Background(), PublishItem{Title: "Hello", Slug: "hello-world", Content: "Body"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotBody["title"] != "Hello" || gotBody["body"] != "Body" || gotBody["slug"] != "hello-world" {
		t.Fatalf("unexpected payload: %#v", gotBody)
	}
	if res.URL != "https://site.example/posts/hello-world" {
		t.Fatalf("url = %q", res.URL)
	}
}

func TestHTTPFieldMapping(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"url":"https://site.example/posts/1"}`))
	}))
	t.Cleanup(ts.Close)

	p := NewHTTPPublisher(HTTPConfig{
		URL:             ts.URL,
		FieldMap:        map[string]string{"headline": "title", "article_markdown": "content", "permalink": "slug"},
		ResponseURLPath: "url",
	})

	_, err := p.Publish(context.Background(), PublishItem{Title: "T", Slug: "s", Content: "C"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotBody["headline"] != "T" || gotBody["article_markdown"] != "C" || gotBody["permalink"] != "s" {
		t.Fatalf("unexpected mapped payload: %#v", gotBody)
	}
}

func TestHTTPAuth(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"url":"https://site.example/posts/1"}`))
	}))
	t.Cleanup(ts.Close)

	p := NewHTTPPublisher(HTTPConfig{
		URL:             ts.URL,
		AuthHeader:      "Authorization",
		AuthToken:       "secret",
		AuthPrefix:      "Bearer ",
		ResponseURLPath: "url",
	})

	_, err := p.Publish(context.Background(), PublishItem{Title: "T", Slug: "s", Content: "C"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("auth header = %q", gotAuth)
	}
}

func TestStaticPublish(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "hero.png")
	if err := os.WriteFile(imgPath, []byte("image"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	p := NewStaticPublisher(StaticConfig{OutputDir: filepath.Join(dir, "dist")})
	res, err := p.Publish(context.Background(), PublishItem{
		Title:     "Hello",
		Slug:      "hello-world",
		Content:   "# Heading\n\nBody",
		ImagePath: imgPath,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	indexPath := filepath.Join(dir, "dist", "hello-world", "index.html")
	body, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(body), "Heading") {
		t.Fatalf("index.html missing content: %s", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist", "hello-world", "hero.png")); err != nil {
		t.Fatalf("expected copied image: %v", err)
	}
	if res.URL == "" {
		t.Fatalf("expected static publish URL")
	}
}

func TestPublishRequiresApprove(t *testing.T) {
	store := newPublishTestStore(t)
	pub := &capturePublisher{}
	svc := NewService(store, pub, ServiceConfig{RequireApprove: true, QAGate: true})

	out, err := svc.PublishSlug(context.Background(), "hello-world", PublishOptions{})
	if err != nil {
		t.Fatalf("publish slug: %v", err)
	}
	if !out.DryRun {
		t.Fatalf("expected dry-run when not approved")
	}
	if pub.calls != 0 {
		t.Fatalf("publish should not be called")
	}
}

func TestPublishRequiresQAPassed(t *testing.T) {
	store := newPublishTestStore(t)
	meta, err := store.Get("hello-world")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	meta.Status = content.StatusDraft
	if err := store.UpdateMeta("hello-world", meta); err != nil {
		t.Fatalf("update meta: %v", err)
	}

	pub := &capturePublisher{}
	svc := NewService(store, pub, ServiceConfig{RequireApprove: true, QAGate: true})

	_, err = svc.PublishSlug(context.Background(), "hello-world", PublishOptions{Approve: true})
	if err == nil {
		t.Fatalf("expected qa gate error")
	}
}

func TestPublishUpdatesStatus(t *testing.T) {
	store := newPublishTestStore(t)
	pub := &capturePublisher{res: PublishResult{URL: "https://site.example/posts/hello-world", ID: "42"}}
	svc := NewService(store, pub, ServiceConfig{RequireApprove: true, QAGate: true})

	out, err := svc.PublishSlug(context.Background(), "hello-world", PublishOptions{Approve: true})
	if err != nil {
		t.Fatalf("publish slug: %v", err)
	}
	if out.Result.URL == "" {
		t.Fatalf("expected publish result URL")
	}

	meta, err := store.Get("hello-world")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if meta.Status != content.StatusPublished {
		t.Fatalf("status = %q, want %q", meta.Status, content.StatusPublished)
	}
	if meta.PublishURL != "https://site.example/posts/hello-world" {
		t.Fatalf("publish_url = %q", meta.PublishURL)
	}
	if meta.PublishedAt == nil {
		t.Fatalf("published_at should be set")
	}
}

func TestDryRun(t *testing.T) {
	store := newPublishTestStore(t)
	pub := &capturePublisher{}
	svc := NewService(store, pub, ServiceConfig{RequireApprove: true, QAGate: true})

	out, err := svc.PublishSlug(context.Background(), "hello-world", PublishOptions{DryRun: true, Approve: true})
	if err != nil {
		t.Fatalf("publish slug: %v", err)
	}
	if !out.DryRun {
		t.Fatalf("expected dry-run true")
	}
	if !strings.Contains(string(out.PayloadJSON), "hello-world") {
		t.Fatalf("expected payload to include slug: %s", out.PayloadJSON)
	}
	if pub.calls != 0 {
		t.Fatalf("publish should not be called in dry-run")
	}
}

func TestNewPublisherFromConfig(t *testing.T) {
	httpPub, err := NewPublisherFromConfig(config.PublishConfig{
		Type: "http",
		URL:  "https://example.com",
	})
	if err != nil {
		t.Fatalf("http publisher: %v", err)
	}
	if _, ok := httpPub.(*HTTPPublisher); !ok {
		t.Fatalf("expected HTTPPublisher, got %T", httpPub)
	}

	staticPub, err := NewPublisherFromConfig(config.PublishConfig{
		Type: "static",
		Static: config.StaticPublishConfig{
			OutputDir: t.TempDir(),
		},
	})
	if err != nil {
		t.Fatalf("static publisher: %v", err)
	}
	if _, ok := staticPub.(*StaticPublisher); !ok {
		t.Fatalf("expected StaticPublisher, got %T", staticPub)
	}
}

func TestNewPublisherFromConfigErrors(t *testing.T) {
	if _, err := NewPublisherFromConfig(config.PublishConfig{Type: "unknown"}); err == nil {
		t.Fatalf("expected unsupported type error")
	}
	if _, err := NewPublisherFromConfig(config.PublishConfig{
		Type:      "http",
		APIKeyCmd: "command-that-does-not-exist-12345",
	}); err == nil {
		t.Fatalf("expected auth command error")
	}
}

func TestHTTPPublishErrors(t *testing.T) {
	if _, err := NewHTTPPublisher(HTTPConfig{}).Publish(context.Background(), PublishItem{}); err == nil {
		t.Fatalf("expected missing URL error")
	}

	tsFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	t.Cleanup(tsFail.Close)
	if _, err := NewHTTPPublisher(HTTPConfig{URL: tsFail.URL}).Publish(context.Background(), PublishItem{}); err == nil {
		t.Fatalf("expected non-2xx error")
	}

	tsBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	t.Cleanup(tsBadJSON.Close)
	if _, err := NewHTTPPublisher(HTTPConfig{URL: tsBadJSON.URL}).Publish(context.Background(), PublishItem{}); err == nil {
		t.Fatalf("expected bad json error")
	}
}

func TestHelpers(t *testing.T) {
	item := PublishItem{
		Title:    "T",
		Slug:     "s",
		Content:  "C",
		Summary:  "S",
		ImageURL: "http://img",
		Meta: map[string]any{
			"count":  1,
			"nested": map[string]any{"url": "https://example.com"},
		},
	}
	if got := itemField(item, "title"); got != "T" {
		t.Fatalf("title field = %#v", got)
	}
	if got := itemField(item, "meta"); got == nil {
		t.Fatalf("meta field should not be nil")
	}
	if got := itemField(item, "count"); got != 1 {
		t.Fatalf("meta lookup = %#v", got)
	}
	if got := getByPath(item.Meta, "nested.url"); got != "https://example.com" {
		t.Fatalf("path lookup = %#v", got)
	}
	if got := asString(float64(42)); got != "42" {
		t.Fatalf("float as string = %q", got)
	}
	if got := asString(7); got != "7" {
		t.Fatalf("int as string = %q", got)
	}
}
