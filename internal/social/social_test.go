package social

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/content"
)

func TestXPostSendsCorrectPayload(t *testing.T) {
	t.Parallel()

	imgPath := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(imgPath, []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	var (
		mu         sync.Mutex
		uploadAuth string
		tweetAuth  string
		tweetText  string
		mediaID    string
	)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1.1/media/upload.json":
			mu.Lock()
			uploadAuth = r.Header.Get("Authorization")
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"media_id_string":"42"}`))
		case "/2/tweets":
			mu.Lock()
			tweetAuth = r.Header.Get("Authorization")
			mu.Unlock()

			var payload struct {
				Text  string `json:"text"`
				Media struct {
					MediaIDs []string `json:"media_ids"`
				} `json:"media"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode tweet payload: %v", err)
			}
			mu.Lock()
			tweetText = payload.Text
			if len(payload.Media.MediaIDs) > 0 {
				mediaID = payload.Media.MediaIDs[0]
			}
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"100"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	poster := NewXPoster(XPosterConfig{
		APIKey:       "key",
		APISecret:    "secret",
		AccessToken:  "access",
		AccessSecret: "access-secret",
		APIBaseURL:   srv.URL,
		UploadURL:    srv.URL,
		HTTPClient:   srv.Client(),
	})

	_, err := poster.Post(context.Background(), SocialPost{
		Platform:  "x",
		Text:      "hello x",
		ImagePath: imgPath,
	})
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !strings.HasPrefix(uploadAuth, "OAuth ") {
		t.Fatalf("upload auth header missing OAuth: %q", uploadAuth)
	}
	if !strings.HasPrefix(tweetAuth, "OAuth ") {
		t.Fatalf("tweet auth header missing OAuth: %q", tweetAuth)
	}
	if tweetText != "hello x" {
		t.Fatalf("tweet text = %q, want hello x", tweetText)
	}
	if mediaID != "42" {
		t.Fatalf("media id = %q, want 42", mediaID)
	}
}

func TestLinkedInPostSendsCorrectPayload(t *testing.T) {
	t.Parallel()

	var (
		authHeader string
		commentary string
	)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/ugcPosts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		share, _ := payload["specificContent"].(map[string]any)
		ugc, _ := share["com.linkedin.ugc.ShareContent"].(map[string]any)
		commentary = ugc["shareCommentary"].(map[string]any)["text"].(string)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"urn:li:share:123"}`))
	}))
	defer srv.Close()

	poster := NewLinkedInPoster(LinkedInPosterConfig{
		AccessToken: "token-123",
		AuthorURN:   "urn:li:person:abc",
		BaseURL:     srv.URL,
		HTTPClient:  srv.Client(),
	})

	_, err := poster.Post(context.Background(), SocialPost{
		Platform:   "linkedin",
		Text:       "hello linkedin",
		ArticleURL: "https://example.com/a",
	})
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}

	if authHeader != "Bearer token-123" {
		t.Fatalf("authorization = %q", authHeader)
	}
	if commentary != "hello linkedin" {
		t.Fatalf("commentary = %q", commentary)
	}
}

func TestLinkedInPostUploadsImagePath(t *testing.T) {
	t.Parallel()

	imgPath := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(imgPath, []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	var (
		gotUploadAuth string
		gotUploadCT   string
		gotAsset      string
	)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/assets" && r.URL.Query().Get("action") == "registerUpload":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":{"uploadMechanism":{"com.linkedin.digitalmedia.uploading.MediaUploadHttpRequest":{"uploadUrl":"` + srv.URL + `/upload/1"}},"asset":"urn:li:digitalmediaAsset:1"}}`))
		case r.URL.Path == "/upload/1":
			gotUploadAuth = r.Header.Get("Authorization")
			gotUploadCT = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/v2/ugcPosts":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			share := payload["specificContent"].(map[string]any)
			ugc := share["com.linkedin.ugc.ShareContent"].(map[string]any)
			media := ugc["media"].([]any)
			gotAsset = media[0].(map[string]any)["media"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"urn:li:share:456"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer srv.Close()

	poster := NewLinkedInPoster(LinkedInPosterConfig{
		AccessToken: "token-123",
		AuthorURN:   "urn:li:person:abc",
		BaseURL:     srv.URL,
		HTTPClient:  srv.Client(),
	})

	_, err := poster.Post(context.Background(), SocialPost{
		Platform:  "linkedin",
		Text:      "image post",
		ImagePath: imgPath,
	})
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if gotUploadAuth != "Bearer token-123" {
		t.Fatalf("upload auth = %q", gotUploadAuth)
	}
	if gotUploadCT != "image/png" {
		t.Fatalf("upload content-type = %q, want image/png", gotUploadCT)
	}
	if gotAsset != "urn:li:digitalmediaAsset:1" {
		t.Fatalf("asset = %q", gotAsset)
	}
}

func TestPosterFactoryLoadsTokensFromCommands(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Project.Name = "test"
	cfg.Social["x"] = config.SocialPlatformConfig{
		Enabled:         true,
		APIKeyCmd:       "printf x-key",
		APISecretCmd:    "printf x-secret",
		AccessTokenCmd:  "printf x-token",
		AccessSecretCmd: "printf x-access-secret",
	}
	cfg.Social["linkedin"] = config.SocialPlatformConfig{
		Enabled:   true,
		APIKeyCmd: "printf li-token",
		AuthorURN: "urn:li:person:abc",
	}

	xPoster, err := NewPosterForPlatform(cfg, "x")
	if err != nil {
		t.Fatalf("NewPosterForPlatform(x) error = %v", err)
	}
	if _, ok := xPoster.(*XPoster); !ok {
		t.Fatalf("x poster type = %T", xPoster)
	}
	liPoster, err := NewPosterForPlatform(cfg, "linkedin")
	if err != nil {
		t.Fatalf("NewPosterForPlatform(linkedin) error = %v", err)
	}
	if _, ok := liPoster.(*LinkedInPoster); !ok {
		t.Fatalf("linkedin poster type = %T", liPoster)
	}
}

func TestScheduleAddsToCalendar(t *testing.T) {
	t.Parallel()

	calPath := filepath.Join(t.TempDir(), "posting-calendar.json")
	s := NewScheduler(calPath, config.ScheduleConfig{Timezone: "UTC"})

	slot := time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC)
	if err := s.Add("alpha-post", slot); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	cal, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cal.Slots) != 1 {
		t.Fatalf("slots len = %d, want 1", len(cal.Slots))
	}
	if cal.Slots[0].Slug != "alpha-post" {
		t.Fatalf("slot slug = %q", cal.Slots[0].Slug)
	}
}

func TestScheduleNextSlot(t *testing.T) {
	t.Parallel()

	s := NewScheduler(filepath.Join(t.TempDir(), "posting-calendar.json"), config.ScheduleConfig{
		Timezone:    "UTC",
		Days:        []string{"Mon", "Wed"},
		WindowStart: "09:00",
		WindowEnd:   "09:30",
	})

	now := time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC) // Monday after window.
	next, err := s.NextSlot(now)
	if err != nil {
		t.Fatalf("NextSlot() error = %v", err)
	}
	want := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC) // Wednesday start.
	if !next.Equal(want) {
		t.Fatalf("next slot = %s, want %s", next, want)
	}
}

func TestPostCheckFiresDue(t *testing.T) {
	t.Parallel()

	store, scheduler, calendarPath := setupPostingFixtures(t)
	due := time.Date(2026, 2, 28, 8, 0, 0, 0, time.UTC)
	if err := scheduler.Add("alpha-post", due); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := markApproved(calendarPath, "alpha-post"); err != nil {
		t.Fatalf("mark approved: %v", err)
	}

	var calls int
	svc := &PostingService{
		Store:     store,
		Scheduler: scheduler,
		Now:       func() time.Time { return time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC) },
		PosterForPlatform: func(_ context.Context, platform string) (SocialPoster, error) {
			return SocialPosterFunc(func(_ context.Context, post SocialPost) (PostResult, error) {
				calls++
				return PostResult{Platform: platform, URL: "https://example.com/post/1"}, nil
			}), nil
		},
	}

	fired, err := svc.PostDue(context.Background())
	if err != nil {
		t.Fatalf("PostDue() error = %v", err)
	}
	if fired != 1 {
		t.Fatalf("fired = %d, want 1", fired)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 platforms", calls)
	}
}

func TestPostCheckSkipsNotDue(t *testing.T) {
	t.Parallel()

	store, scheduler, calendarPath := setupPostingFixtures(t)
	future := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if err := scheduler.Add("alpha-post", future); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := markApproved(calendarPath, "alpha-post"); err != nil {
		t.Fatalf("mark approved: %v", err)
	}

	called := false
	svc := &PostingService{
		Store:     store,
		Scheduler: scheduler,
		Now:       func() time.Time { return time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC) },
		PosterForPlatform: func(_ context.Context, _ string) (SocialPoster, error) {
			called = true
			return SocialPosterFunc(func(_ context.Context, _ SocialPost) (PostResult, error) {
				called = true
				return PostResult{}, nil
			}), nil
		},
	}

	fired, err := svc.PostDue(context.Background())
	if err != nil {
		t.Fatalf("PostDue() error = %v", err)
	}
	if fired != 0 {
		t.Fatalf("fired = %d, want 0", fired)
	}
	if called {
		t.Fatalf("poster called for future slot")
	}
}

func TestNoAutoPost(t *testing.T) {
	t.Parallel()

	store, scheduler, _ := setupPostingFixtures(t)
	svc := &PostingService{Store: store, Scheduler: scheduler}

	if _, err := svc.PostSlug(context.Background(), "alpha-post", false); err == nil {
		t.Fatalf("expected --confirm error")
	} else if !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostUpdatesCalendar(t *testing.T) {
	t.Parallel()

	store, scheduler, calendarPath := setupPostingFixtures(t)
	due := time.Date(2026, 2, 28, 8, 0, 0, 0, time.UTC)
	if err := scheduler.Add("alpha-post", due); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := markApproved(calendarPath, "alpha-post"); err != nil {
		t.Fatalf("mark approved: %v", err)
	}

	svc := &PostingService{
		Store:     store,
		Scheduler: scheduler,
		Now:       func() time.Time { return time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC) },
		PosterForPlatform: func(_ context.Context, platform string) (SocialPoster, error) {
			return SocialPosterFunc(func(_ context.Context, _ SocialPost) (PostResult, error) {
				return PostResult{Platform: platform, URL: "https://example.com/post/1"}, nil
			}), nil
		},
	}

	if _, err := svc.PostDue(context.Background()); err != nil {
		t.Fatalf("PostDue() error = %v", err)
	}

	cal, err := scheduler.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cal.Slots) != 1 {
		t.Fatalf("slots len = %d, want 1", len(cal.Slots))
	}
	if cal.Slots[0].Status != SlotStatusPosted {
		t.Fatalf("status = %q, want posted", cal.Slots[0].Status)
	}
	if cal.Slots[0].PostedAt == nil {
		t.Fatalf("posted_at should be set")
	}
}

func TestPostSlugUpdatesSocialAndMeta(t *testing.T) {
	t.Parallel()

	store, scheduler, _ := setupPostingFixtures(t)
	svc := &PostingService{
		Store:     store,
		Scheduler: scheduler,
		PosterForPlatform: func(_ context.Context, platform string) (SocialPoster, error) {
			return SocialPosterFunc(func(_ context.Context, _ SocialPost) (PostResult, error) {
				return PostResult{Platform: platform, URL: "https://example.com/" + platform + "/1"}, nil
			}), nil
		},
	}

	results, err := svc.PostSlug(context.Background(), "alpha-post", true)
	if err != nil {
		t.Fatalf("PostSlug() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	socialData, err := store.ReadSocial("alpha-post")
	if err != nil {
		t.Fatalf("ReadSocial() error = %v", err)
	}
	if socialData.XPostURL == "" || socialData.LinkedInURL == "" {
		t.Fatalf("post urls should be set: %#v", socialData)
	}
	meta, err := store.Get("alpha-post")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if meta.Status != content.StatusPosted {
		t.Fatalf("meta status = %q, want posted", meta.Status)
	}
}

func setupPostingFixtures(t *testing.T) (*content.Store, *Scheduler, string) {
	t.Helper()

	contentDir := t.TempDir()
	store := content.NewStore(contentDir)
	if err := store.Create("alpha-post", "Alpha Post"); err != nil {
		t.Fatalf("create content: %v", err)
	}
	meta, err := store.Get("alpha-post")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	meta.Status = content.StatusScheduled
	meta.PublishURL = "https://example.com/alpha"
	if err := store.UpdateMeta("alpha-post", meta); err != nil {
		t.Fatalf("update meta: %v", err)
	}
	if err := store.WriteSocial("alpha-post", &content.SocialJSON{
		XText:        "x copy",
		LinkedInText: "li copy",
	}); err != nil {
		t.Fatalf("write social: %v", err)
	}

	calendarPath := filepath.Join(contentDir, "posting-calendar.json")
	scheduler := NewScheduler(calendarPath, config.ScheduleConfig{Timezone: "UTC"})
	return store, scheduler, calendarPath
}

func markApproved(path, slug string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cal PostingCalendar
	if err := json.Unmarshal(data, &cal); err != nil {
		return err
	}
	for i := range cal.Slots {
		if cal.Slots[i].Slug == slug {
			cal.Slots[i].Approved = true
		}
	}
	updated, err := json.MarshalIndent(cal, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	return os.WriteFile(path, updated, 0o644)
}
