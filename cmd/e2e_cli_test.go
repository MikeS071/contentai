package cmd

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

	"github.com/MikeS071/contentai/internal/content"
)

func TestE2ELifecycle(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	restoreWD := setWD(t, workDir)
	defer restoreWD()

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Feed</title><link>https://example.test</link><item><title>One</title><link>https://example.test/one</link><pubDate>Sat, 28 Feb 2026 08:00:00 GMT</pubDate><description>desc</description></item></channel></rss>`))
	}))
	defer rss.Close()

	chatCall := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			chatCall++
			mockChatByCall(w, chatCall)
		case "/v1/images/generations":
			mockImage(t, w)
		case "/publish":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"https://publisher.example/test-article","id":"1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	restoreEnv := setEnvMap(t, map[string]string{
		"CONTENTAI_LLM_API_KEY":    "test-llm-key",
		"CONTENTAI_IMAGE_API_KEY":  "test-image-key",
		"OPENAI_API_KEY":           "test-openai-key",
		"CONTENTAI_LLM_BASE_URL":   api.URL,
		"CONTENTAI_IMAGE_BASE_URL": api.URL,
	})
	defer restoreEnv()

	initInput := strings.Join([]string{
		"Example article body for init wizard.",
		"---END---",
		"",
		"test-llm-key",
		"test-image-key",
		api.URL + "/publish",
		"",
	}, "\n")

	run(t, initInput, "init", "demo")
	run(t, "", "kb", "add-feed", rss.URL+"/feed.xml")
	run(t, "", "kb", "sync")
	run(t, "0\n", "ideas", "--count", "1")
	run(t, "", "new", "test-article", "--from-idea", "1")
	run(t, "", "draft", "test-article")
	run(t, strings.Repeat("y\n", 8), "qa", "test-article", "--auto-fix", "--approve")
	run(t, "", "hero", "test-article")
	run(t, "", "publish", "test-article", "--approve")
	run(t, "s\n", "social", "test-article")
	run(t, "", "schedule", "test-article")

	requireFile(t, filepath.Join(workDir, "content", "blueprint.md"))
	requireFile(t, filepath.Join(workDir, "content", "voice.md"))
	requireGlob(t, filepath.Join(workDir, "content", "kb", "blogs"), "*.md")
	requireGlob(t, filepath.Join(workDir, "content", "ideas"), "*.md")
	requireFile(t, filepath.Join(workDir, "content", "test-article", "article.md"))
	requireFile(t, filepath.Join(workDir, "content", "test-article", "qa.json"))
	requireFile(t, filepath.Join(workDir, "content", "test-article", "hero.png"))
	requireFile(t, filepath.Join(workDir, "content", "test-article", "hero-linkedin.png"))
	requireFile(t, filepath.Join(workDir, "content", "test-article", "social.json"))

	meta, err := content.NewStore(filepath.Join(workDir, "content")).Get("test-article")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if meta.Status != content.StatusScheduled {
		t.Fatalf("status = %q, want %q", meta.Status, content.StatusScheduled)
	}
}

func run(t *testing.T, stdin string, args ...string) {
	t.Helper()
	root := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"--config", "contentai.toml"}, args...))
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("contentai %s failed: %v\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
}

func mockChatByCall(w http.ResponseWriter, call int) {
	reply := "OK"
	switch call {
	case 1:
		reply = "# Blueprint\n\n- Audience trust\n- Practical leverage"
	case 2:
		reply = "Write with directness."
	case 4:
		reply = "## Idea 1\n### Working Title\nTest Article\n\n### Core Paradox\nSimple beats broad.\n\n### Transformation Arc\nNoise to signal.\n\n### Key Examples\n- One specific move.\n\n### Actionable Steps\n- Do one thing."
	case 5:
		reply = "## Extracted Insights\n- One angle."
	case 6:
		reply = "# Test Article\n\nA short, clear draft."
	case 7, 8:
		reply = "[]"
	case 9:
		reply = "# Test Article\n\nA revised short, clear draft."
	case 10:
		reply = "Short signal, clear action. {{ARTICLE_URL}}"
	case 11:
		reply = "Short signal, clear action.\n\n#Content #AI\n\n{{ARTICLE_URL}}"
	}
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(reply)
	_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":` + string(b) + `}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
}

func mockImage(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1792, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1792; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 90, B: 150, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode image: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(buf.Bytes()) + `"}]}`))
}

func setWD(t *testing.T, path string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return func() { _ = os.Chdir(old) }
}

func setEnvMap(t *testing.T, values map[string]string) func() {
	t.Helper()
	old := map[string]*string{}
	for k, v := range values {
		if prev, ok := os.LookupEnv(k); ok {
			prevCopy := prev
			old[k] = &prevCopy
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
			} else {
				_ = os.Setenv(k, *v)
			}
		}
	}
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing %s: %v", path, err)
	}
}

func requireGlob(t *testing.T, dir, pattern string) {
	t.Helper()
	count := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ok, matchErr := filepath.Match(pattern, filepath.Base(path))
		if matchErr != nil {
			return matchErr
		}
		if ok {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walkdir: %v", err)
	}
	if count == 0 {
		t.Fatalf("no %s found in %s", pattern, dir)
	}
}
