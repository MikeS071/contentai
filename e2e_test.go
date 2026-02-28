package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/contentai/cmd"
	"github.com/MikeS071/contentai/internal/content"
)

func TestE2ELifecycle(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	restoreWD := chdir(t, workDir)
	defer restoreWD()

	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feed.xml" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Mock Feed</title>
    <link>https://example.test</link>
    <item>
      <title>Mock Post One</title>
      <link>https://example.test/mock-post-one</link>
      <pubDate>Sat, 28 Feb 2026 08:00:00 GMT</pubDate>
      <description>Practical lessons from the mock feed.</description>
    </item>
  </channel>
</rss>`))
	}))
	defer rssServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			serveMockChat(t, w, r)
		case "/v1/images/generations":
			serveMockImage(t, w, r)
		case "/publish":
			_ = json.NewDecoder(r.Body).Decode(&map[string]any{})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"https://publisher.example/test-article","id":"pub-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	envRestore := withEnv(t, map[string]string{
		"CONTENTAI_LLM_API_KEY":    "test-llm-key",
		"CONTENTAI_IMAGE_API_KEY":  "test-image-key",
		"OPENAI_API_KEY":           "test-openai-key",
		"CONTENTAI_LLM_BASE_URL":   apiServer.URL,
		"CONTENTAI_IMAGE_BASE_URL": apiServer.URL,
	})
	defer envRestore()

	initInput := strings.Join([]string{
		"A compact article that demonstrates voice and structure.",
		"---END---",
		"",
		"test-llm-key",
		"test-image-key",
		apiServer.URL + "/publish",
		"",
	}, "\n")
	runCLI(t, initInput, "init", "demo-project")

	runCLI(t, "", "kb", "add-feed", rssServer.URL+"/feed.xml")
	runCLI(t, "", "kb", "sync")
	runCLI(t, "0\n", "ideas", "--count", "1")
	runCLI(t, "", "new", "test-article", "--from-idea", "1")
	runCLI(t, "", "draft", "test-article")
	runCLI(t, strings.Repeat("y\n", 8), "qa", "test-article", "--auto-fix", "--approve")
	runCLI(t, "", "hero", "test-article")
	runCLI(t, "", "publish", "test-article", "--approve")
	runCLI(t, "s\n", "social", "test-article")
	runCLI(t, "", "schedule", "test-article")

	ensureFile(t, filepath.Join(workDir, "content", "blueprint.md"))
	ensureFile(t, filepath.Join(workDir, "content", "voice.md"))
	ensureAnyMatch(t, filepath.Join(workDir, "content", "kb", "blogs"), "*.md")
	ensureAnyMatch(t, filepath.Join(workDir, "content", "ideas"), "*.md")
	ensureFile(t, filepath.Join(workDir, "content", "test-article", "article.md"))
	ensureFile(t, filepath.Join(workDir, "content", "test-article", "qa.json"))
	ensureFile(t, filepath.Join(workDir, "content", "test-article", "hero.png"))
	ensureFile(t, filepath.Join(workDir, "content", "test-article", "hero-linkedin.png"))
	ensureFile(t, filepath.Join(workDir, "content", "test-article", "social.json"))

	meta, err := content.NewStore(filepath.Join(workDir, "content")).Get("test-article")
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if meta.Status != content.StatusScheduled {
		t.Fatalf("final status = %q, want %q", meta.Status, content.StatusScheduled)
	}
}

func runCLI(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	root := cmd.NewRootCmd()
	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"--config", "contentai.toml"}, args...))
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("command failed: contentai %s\nerror: %v\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), err, out.String(), errOut.String())
	}
	return out.String()
}

func serveMockChat(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("decode chat request: %v", err)
	}
	user := ""
	system := ""
	for _, m := range req.Messages {
		if m.Role == "system" && system == "" {
			system = m.Content
		}
		if m.Role == "user" {
			user = m.Content
		}
	}

	reply := "OK"
	switch {
	case strings.Contains(user, "Respond with OK"):
		reply = "OK"
	case strings.Contains(user, "Example Articles:") && strings.Contains(system, "perspective"):
		reply = "# Blueprint\n\n- Audience trust\n- Practical leverage"
	case strings.Contains(user, "Blueprint:") && strings.Contains(system, "voice"):
		reply = "Write with directness, practical framing, and clear contrast."
	case strings.Contains(user, "Produce exactly 1 outlines"):
		reply = strings.TrimSpace(`## Idea 1
### Working Title
Test Article

### Core Paradox
Move faster by reducing options.

### Transformation Arc
Chaos to focus through constraints.

### Key Examples
- Weekly publishing cadence.

### Actionable Steps
- Commit to one narrow format.`)
	case strings.Contains(user, "Mine insights from this context"):
		reply = "## Extracted Insights\n- Commit to one core lens.\n- Cut optional branches."
	case strings.Contains(user, "Use this full context while writing"):
		reply = "# Test Article\n\nThis draft is intentionally concise and specific.\n\nIt includes a practical example and a clear next step."
	case strings.Contains(user, "Compare the article against the voice profile"):
		reply = "[]"
	case strings.Contains(user, "List claims that sound factual"):
		reply = "[]"
	case strings.Contains(user, "Rewrite the article to resolve only these QA issues"):
		reply = "# Test Article\n\nThis revised draft keeps the original meaning and resolves QA issues with tighter phrasing."
	case strings.Contains(user, "Write only X post copy"):
		reply = "A practical walkthrough from idea to shipped post. {{ARTICLE_URL}}"
	case strings.Contains(user, "Write only LinkedIn copy"):
		reply = "A practical walkthrough from idea to shipped post.\n\nUse this lifecycle to reduce publishing friction.\n\n#ContentOps #AI\n\n{{ARTICLE_URL}}"
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":` + toJSONString(reply) + `}}],"usage":{"prompt_tokens":10,"completion_tokens":20}}`))
}

func serveMockImage(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1792, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1792; x++ {
			img.Set(x, y, color.RGBA{R: 24, G: 90, B: 140, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode mock image: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + b64 + `"}]}`))
}

func toJSONString(v string) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir(%s): %v", dir, err)
	}
	return func() {
		_ = os.Chdir(old)
	}
}

func withEnv(t *testing.T, values map[string]string) func() {
	t.Helper()
	old := make(map[string]*string, len(values))
	for k, v := range values {
		if cur, ok := os.LookupEnv(k); ok {
			curCopy := cur
			old[k] = &curCopy
		} else {
			old[k] = nil
		}
		if err := os.Setenv(k, v); err != nil {
			t.Fatalf("setenv %s: %v", k, err)
		}
	}
	return func() {
		for k, v := range old {
			if v == nil {
				_ = os.Unsetenv(k)
				continue
			}
			_ = os.Setenv(k, *v)
		}
	}
}

func ensureFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing file %s: %v", path, err)
	}
}

func ensureAnyMatch(t *testing.T, dir, pattern string) {
	t.Helper()
	count := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		matched, matchErr := filepath.Match(pattern, filepath.Base(path))
		if matchErr != nil {
			return matchErr
		}
		if matched {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if count == 0 {
		t.Fatalf("expected at least one match in %s for %s", dir, pattern)
	}
}
