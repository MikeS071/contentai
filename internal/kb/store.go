package kb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	feedTimeout = 30 * time.Second
)

type Store struct {
	ContentDir string
	HTTPClient *http.Client
	Now        func() time.Time
}

type Feed struct {
	Title        string    `toml:"title" json:"title"`
	URL          string    `toml:"url" json:"url"`
	DirName      string    `toml:"dir_name" json:"dir_name"`
	AddedAt      time.Time `toml:"added_at" json:"added_at"`
	LastSyncedAt time.Time `toml:"last_synced_at" json:"last_synced_at"`
	LastPostDate time.Time `toml:"last_post_date" json:"last_post_date"`
}

type SyncReport struct {
	Feeds       int
	SyncedFeeds int
	NewPosts    int
	Errors      []string
}

type feedRegistry struct {
	Feeds []Feed `toml:"feeds"`
}

func NewStore(contentDir string) *Store {
	return &Store{
		ContentDir: contentDir,
		HTTPClient: &http.Client{Timeout: feedTimeout},
		Now:        time.Now,
	}
}

func (s *Store) AddFeed(rawURL, title string) (Feed, error) {
	if err := s.ensureLayout(); err != nil {
		return Feed{}, err
	}
	feedURL, err := normalizeURL(rawURL)
	if err != nil {
		return Feed{}, err
	}

	reg, err := s.loadRegistry()
	if err != nil {
		return Feed{}, err
	}
	for _, existing := range reg.Feeds {
		if strings.EqualFold(existing.URL, feedURL) {
			return Feed{}, fmt.Errorf("feed already exists: %s", feedURL)
		}
	}

	now := s.nowUTC()
	feed := Feed{
		Title:   strings.TrimSpace(title),
		URL:     feedURL,
		DirName: feedDirName(feedURL),
		AddedAt: now,
	}
	if feed.Title == "" {
		feed.Title = feedURL
	}

	reg.Feeds = append(reg.Feeds, feed)
	if err := s.saveRegistry(reg); err != nil {
		return Feed{}, err
	}
	if err := s.writeFeedMeta(feed); err != nil {
		return Feed{}, err
	}
	return feed, nil
}

func (s *Store) ListFeeds() ([]Feed, error) {
	if err := s.ensureLayout(); err != nil {
		return nil, err
	}
	reg, err := s.loadRegistry()
	if err != nil {
		return nil, err
	}
	feeds := append([]Feed(nil), reg.Feeds...)
	sort.Slice(feeds, func(i, j int) bool {
		return feeds[i].AddedAt.Before(feeds[j].AddedAt)
	})
	return feeds, nil
}

func (s *Store) RemoveFeed(rawURL string, deleteContent bool) error {
	if err := s.ensureLayout(); err != nil {
		return err
	}
	feedURL, err := normalizeURL(rawURL)
	if err != nil {
		return err
	}

	reg, err := s.loadRegistry()
	if err != nil {
		return err
	}
	filtered := make([]Feed, 0, len(reg.Feeds))
	var removed *Feed
	for i := range reg.Feeds {
		if strings.EqualFold(reg.Feeds[i].URL, feedURL) {
			f := reg.Feeds[i]
			removed = &f
			continue
		}
		filtered = append(filtered, reg.Feeds[i])
	}
	if removed == nil {
		return fmt.Errorf("feed not found: %s", feedURL)
	}
	reg.Feeds = filtered
	if err := s.saveRegistry(reg); err != nil {
		return err
	}
	if deleteContent {
		if err := os.RemoveAll(filepath.Join(s.blogsDir(), removed.DirName)); err != nil {
			return fmt.Errorf("remove feed content: %w", err)
		}
	}
	return nil
}

func (s *Store) ImportOPML(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read OPML: %w", err)
	}
	feeds, err := ParseOPML(content)
	if err != nil {
		return 0, err
	}

	added := 0
	for _, f := range feeds {
		if _, err := s.AddFeed(f.URL, f.Title); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return added, err
		}
		added++
	}
	return added, nil
}

func (s *Store) AddNote(path string) (string, error) {
	if err := s.ensureLayout(); err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read note: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name := slugify(base)
	if name == "" {
		name = "note"
	}
	dst := filepath.Join(s.notesDir(), name+".md")
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		return "", fmt.Errorf("write note: %w", err)
	}
	return dst, nil
}

func (s *Store) Sync(ctx context.Context) (SyncReport, error) {
	if err := s.ensureLayout(); err != nil {
		return SyncReport{}, err
	}
	reg, err := s.loadRegistry()
	if err != nil {
		return SyncReport{}, err
	}

	report := SyncReport{Feeds: len(reg.Feeds)}
	updated := false
	for i := range reg.Feeds {
		n, err := s.syncOne(ctx, &reg.Feeds[i])
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", reg.Feeds[i].URL, err))
			continue
		}
		report.NewPosts += n
		report.SyncedFeeds++
		updated = true
	}
	if updated {
		if err := s.saveRegistry(reg); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (s *Store) kbDir() string {
	return filepath.Join(s.ContentDir, "kb")
}

func (s *Store) blogsDir() string {
	return filepath.Join(s.kbDir(), "blogs")
}

func (s *Store) notesDir() string {
	return filepath.Join(s.kbDir(), "notes")
}

func (s *Store) feedsPath() string {
	return filepath.Join(s.kbDir(), "feeds.toml")
}

func (s *Store) nowUTC() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s *Store) ensureLayout() error {
	if err := os.MkdirAll(s.blogsDir(), 0o755); err != nil {
		return fmt.Errorf("create blogs dir: %w", err)
	}
	if err := os.MkdirAll(s.notesDir(), 0o755); err != nil {
		return fmt.Errorf("create notes dir: %w", err)
	}
	if _, err := os.Stat(s.feedsPath()); errors.Is(err, os.ErrNotExist) {
		return s.saveRegistry(feedRegistry{Feeds: []Feed{}})
	} else if err != nil {
		return fmt.Errorf("stat feeds registry: %w", err)
	}
	return nil
}

func (s *Store) loadRegistry() (feedRegistry, error) {
	content, err := os.ReadFile(s.feedsPath())
	if err != nil {
		return feedRegistry{}, fmt.Errorf("read feeds registry: %w", err)
	}
	var reg feedRegistry
	if len(strings.TrimSpace(string(content))) == 0 {
		return feedRegistry{Feeds: []Feed{}}, nil
	}
	if err := toml.Unmarshal(content, &reg); err != nil {
		return feedRegistry{}, fmt.Errorf("parse feeds registry: %w", err)
	}
	return reg, nil
}

func (s *Store) saveRegistry(reg feedRegistry) error {
	content, err := toml.Marshal(reg)
	if err != nil {
		return fmt.Errorf("encode feeds registry: %w", err)
	}
	if err := os.WriteFile(s.feedsPath(), content, 0o644); err != nil {
		return fmt.Errorf("write feeds registry: %w", err)
	}
	return nil
}
