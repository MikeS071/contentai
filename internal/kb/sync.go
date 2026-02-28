package kb

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) syncOne(ctx context.Context, feed *Feed) (int, error) {
	feedCtx, cancel := context.WithTimeout(ctx, feedTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(feedCtx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	parsed, err := ParseFeed(body, feed.URL)
	if err != nil {
		return 0, err
	}
	if feed.Title == "" && parsed.Title != "" {
		feed.Title = parsed.Title
	}

	feedDir := filepath.Join(s.blogsDir(), feed.DirName)
	if err := os.MkdirAll(feedDir, 0o755); err != nil {
		return 0, fmt.Errorf("create feed dir: %w", err)
	}
	existing, err := existingPostURLs(feedDir)
	if err != nil {
		return 0, err
	}

	newPosts := 0
	latestPost := feed.LastPostDate
	for _, item := range parsed.Items {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		if _, ok := existing[item.URL]; ok {
			continue
		}
		if err := writePostMarkdown(feedDir, *feed, item, s.nowUTC()); err != nil {
			return newPosts, err
		}
		existing[item.URL] = struct{}{}
		newPosts++
		if item.PublishedAt.After(latestPost) {
			latestPost = item.PublishedAt
		}
	}

	feed.LastSyncedAt = s.nowUTC()
	if latestPost.After(feed.LastPostDate) {
		feed.LastPostDate = latestPost
	}
	if err := s.writeFeedMeta(*feed); err != nil {
		return newPosts, err
	}
	return newPosts, nil
}

func (s *Store) writeFeedMeta(feed Feed) error {
	feedDir := filepath.Join(s.blogsDir(), feed.DirName)
	if err := os.MkdirAll(feedDir, 0o755); err != nil {
		return fmt.Errorf("create feed dir: %w", err)
	}
	content, err := json.MarshalIndent(feed, "", "  ")
	if err != nil {
		return fmt.Errorf("encode feed metadata: %w", err)
	}
	content = append(content, '\n')
	path := filepath.Join(feedDir, "feed.json")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write feed metadata: %w", err)
	}
	return nil
}

func (s *Store) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: feedTimeout}
}

func existingPostURLs(feedDir string) (map[string]struct{}, error) {
	matches, err := filepath.Glob(filepath.Join(feedDir, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("scan feed posts: %w", err)
	}
	urls := make(map[string]struct{}, len(matches))
	for _, path := range matches {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read existing post: %w", err)
		}
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "url: ") {
				urlValue := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "url: ")), "\"")
				if urlValue != "" {
					urls[urlValue] = struct{}{}
				}
				break
			}
		}
	}
	return urls, nil
}

func writePostMarkdown(feedDir string, feed Feed, item FeedItem, now time.Time) error {
	pub := item.PublishedAt
	if pub.IsZero() {
		pub = now
	}
	datePart := pub.UTC().Format("2006-01-02")
	slug := slugify(item.Title)
	if slug == "" {
		slug = "post"
	}

	filename := filepath.Join(feedDir, fmt.Sprintf("%s-%s.md", datePart, slug))
	for i := 2; ; i++ {
		if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
			break
		}
		filename = filepath.Join(feedDir, fmt.Sprintf("%s-%s-%d.md", datePart, slug, i))
	}

	body := strings.TrimSpace(item.Content)
	if body == "" {
		body = item.Title
	}
	markdown := fmt.Sprintf("---\ntitle: \"%s\"\nurl: \"%s\"\ndate: \"%s\"\nfeed: \"%s\"\n---\n\n%s\n",
		yamlEscape(item.Title),
		yamlEscape(item.URL),
		pub.UTC().Format(time.RFC3339),
		yamlEscape(feed.URL),
		body,
	)
	if err := os.WriteFile(filename, []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("write markdown post: %w", err)
	}
	return nil
}

func normalizeURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("feed URL is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid feed URL: %s", raw)
	}
	u.Fragment = ""
	return u.String(), nil
}

func feedDirName(feedURL string) string {
	u, err := url.Parse(feedURL)
	host := "feed"
	if err == nil && u.Host != "" {
		host = strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	}
	h := sha1.Sum([]byte(feedURL))
	hash := hex.EncodeToString(h[:])[:10]
	return fmt.Sprintf("%s-%s", slugify(host), hash)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func yamlEscape(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), `"`, `\"`)
}
