package publish

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticPublishErrors(t *testing.T) {
	if _, err := NewStaticPublisher(StaticConfig{OutputDir: t.TempDir()}).Publish(context.Background(), PublishItem{}); err == nil {
		t.Fatalf("expected missing slug error")
	}
	if _, err := NewStaticPublisher(StaticConfig{}).Publish(context.Background(), PublishItem{Slug: "slug"}); err == nil {
		t.Fatalf("expected missing output dir error")
	}
	if _, err := NewStaticPublisher(StaticConfig{OutputDir: t.TempDir()}).Publish(context.Background(), PublishItem{
		Slug:      "slug",
		ImagePath: filepath.Join(t.TempDir(), "missing.png"),
	}); err == nil {
		t.Fatalf("expected image copy error")
	}
}

func TestPublishSlugErrorPaths(t *testing.T) {
	svc := NewService(nil, nil, ServiceConfig{})
	if _, err := svc.PublishSlug(context.Background(), "slug", PublishOptions{}); err == nil {
		t.Fatalf("expected uninitialized service error")
	}

	store := newPublishTestStore(t)
	svc = NewService(store, nil, ServiceConfig{RequireApprove: false, QAGate: true})
	if _, err := svc.PublishSlug(context.Background(), "", PublishOptions{}); err == nil {
		t.Fatalf("expected empty slug error")
	}
	if _, err := svc.PublishSlug(context.Background(), "hello-world", PublishOptions{Approve: true}); err == nil {
		t.Fatalf("expected missing publisher error")
	}
}

func TestPublishSlugUpdateMetaFailure(t *testing.T) {
	store := newPublishTestStore(t)
	pub := &capturePublisher{res: PublishResult{URL: "https://example.com"}}
	svc := NewService(store, pub, ServiceConfig{RequireApprove: true, QAGate: true})

	metaPath := filepath.Join(store.SlugDir("hello-world"), "meta.json")
	if err := os.Chmod(metaPath, 0o444); err != nil {
		t.Fatalf("chmod meta.json: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(metaPath, 0o644) })

	_, err := svc.PublishSlug(context.Background(), "hello-world", PublishOptions{Approve: true})
	if err == nil {
		t.Fatalf("expected update meta failure")
	}
	if !errors.Is(err, os.ErrPermission) && !strings.Contains(err.Error(), "write meta.json") {
		t.Fatalf("unexpected error: %v", err)
	}
}
