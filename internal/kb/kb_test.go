package kb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	s := NewStore(filepath.Join(root, "content"))
	s.Now = func() time.Time {
		return time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	}
	return s
}

func TestAddFeed(t *testing.T) {
	s := newTestStore(t)

	feed, err := s.AddFeed("https://example.com/feed.xml", "Example")
	if err != nil {
		t.Fatalf("AddFeed() error = %v", err)
	}
	if feed.URL != "https://example.com/feed.xml" {
		t.Fatalf("feed.URL = %q", feed.URL)
	}

	feeds, err := s.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() error = %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("len(feeds) = %d, want 1", len(feeds))
	}
}

func TestAddFeedDuplicate(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddFeed("https://example.com/feed.xml", "Example"); err != nil {
		t.Fatalf("AddFeed() first error = %v", err)
	}
	if _, err := s.AddFeed("https://example.com/feed.xml", "Example"); err == nil {
		t.Fatalf("AddFeed() duplicate error = nil")
	}
}

func TestListFeeds(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddFeed("https://a.example/feed.xml", "A"); err != nil {
		t.Fatalf("AddFeed A error = %v", err)
	}
	if _, err := s.AddFeed("https://b.example/feed.xml", "B"); err != nil {
		t.Fatalf("AddFeed B error = %v", err)
	}

	feeds, err := s.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() error = %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("len(feeds) = %d, want 2", len(feeds))
	}
}

func TestRemoveFeed(t *testing.T) {
	s := newTestStore(t)
	feed, err := s.AddFeed("https://example.com/feed.xml", "Example")
	if err != nil {
		t.Fatalf("AddFeed() error = %v", err)
	}

	blogDir := filepath.Join(s.blogsDir(), feed.DirName)
	if err := os.MkdirAll(blogDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := s.RemoveFeed("https://example.com/feed.xml", true); err != nil {
		t.Fatalf("RemoveFeed() error = %v", err)
	}
	feeds, err := s.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() error = %v", err)
	}
	if len(feeds) != 0 {
		t.Fatalf("len(feeds) = %d, want 0", len(feeds))
	}
	if _, err := os.Stat(blogDir); !os.IsNotExist(err) {
		t.Fatalf("blogDir should be removed")
	}
}

func TestParseRSS(t *testing.T) {
	parsed, err := ParseFeed([]byte(sampleRSS), "https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("ParseFeed(RSS) error = %v", err)
	}
	if parsed.Title != "Example RSS" {
		t.Fatalf("parsed.Title = %q", parsed.Title)
	}
	if len(parsed.Items) != 1 {
		t.Fatalf("len(parsed.Items) = %d, want 1", len(parsed.Items))
	}
	if parsed.Items[0].Title != "Hello RSS" {
		t.Fatalf("item title = %q", parsed.Items[0].Title)
	}
}

func TestParseAtom(t *testing.T) {
	parsed, err := ParseFeed([]byte(sampleAtom), "https://example.com/atom.xml")
	if err != nil {
		t.Fatalf("ParseFeed(Atom) error = %v", err)
	}
	if parsed.Title != "Example Atom" {
		t.Fatalf("parsed.Title = %q", parsed.Title)
	}
	if len(parsed.Items) != 1 {
		t.Fatalf("len(parsed.Items) = %d, want 1", len(parsed.Items))
	}
	if parsed.Items[0].URL != "https://example.com/atom-post" {
		t.Fatalf("item URL = %q", parsed.Items[0].URL)
	}
}

func TestHTMLToMarkdown(t *testing.T) {
	got := HTMLToMarkdown("<p>Hello &amp; <b>world</b><br/>now</p>")
	want := "Hello & world\nnow"
	if got != want {
		t.Fatalf("HTMLToMarkdown() = %q, want %q", got, want)
	}
}

func TestSyncNewPosts(t *testing.T) {
	s := newTestStore(t)
	server := newFeedServer(t, sampleRSS)
	defer server.Close()

	if _, err := s.AddFeed(server.URL+"/feed.xml", "Feed"); err != nil {
		t.Fatalf("AddFeed() error = %v", err)
	}

	report, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if report.NewPosts != 1 {
		t.Fatalf("report.NewPosts = %d, want 1", report.NewPosts)
	}
	files, err := filepath.Glob(filepath.Join(s.blogsDir(), "*", "*.md"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "title: \"Hello RSS\"") {
		t.Fatalf("expected frontmatter title in markdown")
	}
}

func TestSyncSkipsExisting(t *testing.T) {
	s := newTestStore(t)
	server := newFeedServer(t, sampleRSS)
	defer server.Close()

	if _, err := s.AddFeed(server.URL+"/feed.xml", "Feed"); err != nil {
		t.Fatalf("AddFeed() error = %v", err)
	}
	if _, err := s.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync() error = %v", err)
	}
	report, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if report.NewPosts != 0 {
		t.Fatalf("report.NewPosts = %d, want 0", report.NewPosts)
	}
}

func TestSyncUpdatesTimestamp(t *testing.T) {
	s := newTestStore(t)
	server := newFeedServer(t, sampleRSS)
	defer server.Close()

	if _, err := s.AddFeed(server.URL+"/feed.xml", "Feed"); err != nil {
		t.Fatalf("AddFeed() error = %v", err)
	}
	if _, err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	feeds, err := s.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() error = %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("len(feeds) = %d, want 1", len(feeds))
	}
	if feeds[0].LastSyncedAt.IsZero() {
		t.Fatalf("LastSyncedAt should be set")
	}
}

func TestOPMLImport(t *testing.T) {
	s := newTestStore(t)
	opmlPath := filepath.Join(t.TempDir(), "feeds.opml")
	if err := os.WriteFile(opmlPath, []byte(sampleOPML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	count, err := s.ImportOPML(opmlPath)
	if err != nil {
		t.Fatalf("ImportOPML() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	feeds, err := s.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() error = %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("len(feeds) = %d, want 2", len(feeds))
	}
}

func TestAddNote(t *testing.T) {
	s := newTestStore(t)
	src := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(src, []byte("hello note"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	dst, err := s.AddNote(src)
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "hello note" {
		t.Fatalf("note content mismatch")
	}
}

func TestSearchKeyword(t *testing.T) {
	s := newTestStore(t)
	notePath := filepath.Join(s.notesDir(), "note.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(notePath, []byte("# Deep Work\n\nFocus and topic clarity."), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	results, err := Search(s.kbDir(), "clarity", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
}

func TestSearchNoResults(t *testing.T) {
	s := newTestStore(t)
	results, err := Search(s.kbDir(), "does-not-exist", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}

func TestSearchLimit(t *testing.T) {
	s := newTestStore(t)
	if err := os.MkdirAll(s.notesDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for i := range 3 {
		path := filepath.Join(s.notesDir(), "note-"+string(rune('a'+i))+".md")
		if err := os.WriteFile(path, []byte("topic appears here"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
	results, err := Search(s.kbDir(), "topic", 2)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
}

func newFeedServer(t *testing.T, body string) *testServer {
	t.Helper()
	return newTestServer(t, body)
}

type testServer struct {
	URL string
	srv *httptest.Server
}

func (s *testServer) Close() {
	s.srv.Close()
}

func newTestServer(t *testing.T, body string) *testServer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	return &testServer{
		URL: srv.URL,
		srv: srv,
	}
}

const sampleRSS = `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Example RSS</title>
    <link>https://example.com/</link>
    <item>
      <title>Hello RSS</title>
      <link>/post-1</link>
      <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
      <description><![CDATA[<p>Hello &amp; <strong>RSS</strong></p>]]></description>
    </item>
  </channel>
</rss>`

const sampleAtom = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom</title>
  <link href="https://example.com/"/>
  <entry>
    <title>Hello Atom</title>
    <link href="/atom-post"/>
    <updated>2006-01-02T15:04:05Z</updated>
    <summary type="html">&lt;p&gt;Atom &amp;amp; Stuff&lt;/p&gt;</summary>
  </entry>
</feed>`

const sampleOPML = `<?xml version="1.0" encoding="UTF-8"?>
<opml version="1.0">
  <body>
    <outline text="Feed A" title="Feed A" type="rss" xmlUrl="https://a.example/feed.xml" />
    <outline text="Feed B" title="Feed B" type="rss" xmlUrl="https://b.example/feed.xml" />
  </body>
</opml>`
