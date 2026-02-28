package qa

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/llm"
)

type mockLLM struct {
	responses []string
	requests  []llm.Request
	err       error
}

func (m *mockLLM) Complete(_ context.Context, req llm.Request) (*llm.Response, error) {
	m.requests = append(m.requests, req)
	if m.err != nil {
		return nil, m.err
	}
	if len(m.responses) == 0 {
		return &llm.Response{Content: "[]"}, nil
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return &llm.Response{Content: resp}, nil
}

func (m *mockLLM) Stream(_ context.Context, _ llm.Request) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockLLM) Name() string { return "mock" }

func TestCheckNoSecrets(t *testing.T) {
	article := "Token: sk-abc1234567890 and password=hunter2"
	check := CheckNoSecrets(article)
	if check.Passed {
		t.Fatalf("expected no_secrets to fail")
	}
	if len(check.Issues) == 0 {
		t.Fatalf("expected issues for detected secrets")
	}
}

func TestCheckNoSecretsFalsePositive(t *testing.T) {
	article := "This is a general discussion about tokens, passwords, and API best practices."
	check := CheckNoSecrets(article)
	if !check.Passed {
		t.Fatalf("expected no_secrets to pass, got issues: %v", check.Issues)
	}
}

func TestCheckDashCleanup(t *testing.T) {
	article := "This sentence - has an unnecessary dash in the middle."
	check := CheckDashCleanup(article)
	if check.Passed {
		t.Fatalf("expected dash_cleanup to fail")
	}
}

func TestCheckDedup(t *testing.T) {
	article := "Repeat this sentence. Repeat this sentence."
	check := CheckDedup(article)
	if check.Passed {
		t.Fatalf("expected dedup to fail")
	}
}

func TestCheckLength(t *testing.T) {
	short := strings.Repeat("word ", 100)
	if got := CheckLength(short); got.Passed {
		t.Fatalf("expected short content to warn/fail")
	}

	long := strings.Repeat("word ", 1100)
	if got := CheckLength(long); got.Passed {
		t.Fatalf("expected long content to warn/fail")
	}
}

func TestCheckReadingLevel(t *testing.T) {
	article := "Inasmuch as the aforementioned operationalization necessitates multidimensional epistemic calibration across heterogeneous paradigms, stakeholders must meticulously orchestrate implementation vectors."
	check := CheckReadingLevel(article)
	if check.Passed {
		t.Fatalf("expected reading_level to fail for complex sentence")
	}
}

func TestAutoFixProducesDiff(t *testing.T) {
	m := &mockLLM{responses: []string{"Improved text."}}
	check := content.QACheck{Name: "dash_cleanup", Passed: false, Issues: []string{"unnecessary dash"}}

	fixed, diff, fixes, err := AutoFix(context.Background(), m, "gpt-4o-mini", "Original - text.", []content.QACheck{check})
	if err != nil {
		t.Fatalf("AutoFix() error = %v", err)
	}
	if strings.TrimSpace(fixed) != "Improved text." {
		t.Fatalf("unexpected fixed content: %q", fixed)
	}
	if strings.TrimSpace(diff) == "" {
		t.Fatalf("expected diff output")
	}
	if len(fixes["dash_cleanup"]) == 0 {
		t.Fatalf("expected fix entries for dash_cleanup")
	}
}

func TestApproveUpdatesStatus(t *testing.T) {
	store, contentDir := newQAStore(t)
	writeQAFile(t, filepath.Join(contentDir, "voice.md"), "Write with directness")
	writeQAFile(t, filepath.Join(store.SlugDir("alpha-post"), "article.md"), strings.Repeat("plain words ", 550))

	engine := &Engine{Store: store, ContentDir: contentDir, LLM: &mockLLM{}, Model: "gpt-4o-mini"}
	if _, err := engine.Run(context.Background(), RunOptions{Slug: "alpha-post", Approve: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	meta, err := store.Get("alpha-post")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if meta.Status != content.StatusQAPassed {
		t.Fatalf("status = %q, want %q", meta.Status, content.StatusQAPassed)
	}
}

func TestQAJsonOutput(t *testing.T) {
	store, contentDir := newQAStore(t)
	writeQAFile(t, filepath.Join(contentDir, "voice.md"), "Write with directness")
	writeQAFile(t, filepath.Join(store.SlugDir("alpha-post"), "article.md"), strings.Repeat("plain words ", 550))

	engine := &Engine{Store: store, ContentDir: contentDir, LLM: &mockLLM{}, Model: "gpt-4o-mini"}
	result, err := engine.Run(context.Background(), RunOptions{Slug: "alpha-post"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil || result.QA == nil {
		t.Fatalf("expected non-nil qa result")
	}
	if len(result.QA.Checks) != 7 {
		t.Fatalf("expected 7 checks, got %d", len(result.QA.Checks))
	}

	stored, err := store.ReadQA("alpha-post")
	if err != nil {
		t.Fatalf("ReadQA() error = %v", err)
	}
	if len(stored.Checks) != 7 {
		t.Fatalf("expected 7 checks in qa.json, got %d", len(stored.Checks))
	}
}

func TestNoAutoPostWithoutApproval(t *testing.T) {
	store, contentDir := newQAStore(t)
	writeQAFile(t, filepath.Join(contentDir, "voice.md"), "Write with directness")
	writeQAFile(t, filepath.Join(store.SlugDir("alpha-post"), "article.md"), strings.Repeat("plain words ", 550))

	engine := &Engine{Store: store, ContentDir: contentDir, LLM: &mockLLM{}, Model: "gpt-4o-mini"}
	if _, err := engine.Run(context.Background(), RunOptions{Slug: "alpha-post", Approve: false}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	meta, err := store.Get("alpha-post")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if meta.Status == content.StatusQAPassed {
		t.Fatalf("status should not auto-advance to qa_passed without --approve")
	}
	if err := content.ValidTransition(meta.Status, content.StatusPublished, true); err == nil {
		t.Fatalf("publish should remain blocked by qa gate without approval")
	}
}

func TestEngineRequiresDependencies(t *testing.T) {
	engine := &Engine{}
	if _, err := engine.Run(context.Background(), RunOptions{Slug: "alpha"}); err == nil {
		t.Fatalf("expected dependency validation error")
	}

	store, contentDir := newQAStore(t)
	writeQAFile(t, filepath.Join(contentDir, "voice.md"), "voice")
	writeQAFile(t, filepath.Join(store.SlugDir("alpha-post"), "article.md"), "body")

	engine = &Engine{Store: store, ContentDir: contentDir}
	if _, err := engine.Run(context.Background(), RunOptions{Slug: "alpha-post"}); err == nil {
		t.Fatalf("expected llm dependency validation error")
	}
}

func TestEngineMissingSlug(t *testing.T) {
	store, contentDir := newQAStore(t)
	engine := &Engine{Store: store, ContentDir: contentDir, LLM: &mockLLM{}}
	if _, err := engine.Run(context.Background(), RunOptions{}); err == nil {
		t.Fatalf("expected slug error")
	}
}

func TestEngineMissingContentFiles(t *testing.T) {
	store, contentDir := newQAStore(t)
	engine := &Engine{Store: store, ContentDir: contentDir, LLM: &mockLLM{}}
	if _, err := engine.Run(context.Background(), RunOptions{Slug: "alpha-post"}); err == nil {
		t.Fatalf("expected missing voice/article error")
	}
}

func TestAutoFixNoFailures(t *testing.T) {
	fixed, diff, fixes, err := AutoFix(context.Background(), &mockLLM{}, "gpt-4o-mini", "already good", []content.QACheck{{Name: "length", Passed: true}})
	if err != nil {
		t.Fatalf("AutoFix() error = %v", err)
	}
	if fixed != "already good" {
		t.Fatalf("fixed = %q, want unchanged", fixed)
	}
	if diff != "" {
		t.Fatalf("diff = %q, want empty", diff)
	}
	if len(fixes) != 0 {
		t.Fatalf("fixes = %#v, want empty", fixes)
	}
}

func TestCheckLLMOutputVariants(t *testing.T) {
	t.Run("pass-like responses", func(t *testing.T) {
		for _, raw := range []string{"[]", "{}", "PASS", ""} {
			check := checkLLMOutput("accuracy", raw)
			if !check.Passed {
				t.Fatalf("expected %q to pass, got issues: %v", raw, check.Issues)
			}
		}
	})

	t.Run("json issues", func(t *testing.T) {
		check := checkLLMOutput("accuracy", `["claim one", "claim two"]`)
		if check.Passed || len(check.Issues) != 2 {
			t.Fatalf("expected 2 issues, got %+v", check)
		}
	})

	t.Run("line issues", func(t *testing.T) {
		check := checkLLMOutput("voice_consistency", "- robotic tone\n- generic hook")
		if check.Passed || len(check.Issues) != 2 {
			t.Fatalf("expected line issues, got %+v", check)
		}
	})
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 3); got != "abc" {
		t.Fatalf("truncate(<=3) = %q, want %q", got, "abc")
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("truncate(short) = %q, want unchanged", got)
	}
}

func TestRunAutoFixAttachesFixes(t *testing.T) {
	store, contentDir := newQAStore(t)
	writeQAFile(t, filepath.Join(contentDir, "voice.md"), "Write with directness")
	writeQAFile(t, filepath.Join(store.SlugDir("alpha-post"), "article.md"), "This sentence - has an unnecessary dash.")

	m := &mockLLM{responses: []string{"[]", "[]", "This sentence has an unnecessary dash."}}
	engine := &Engine{Store: store, ContentDir: contentDir, LLM: m, Model: "gpt-4o-mini"}
	result, err := engine.Run(context.Background(), RunOptions{Slug: "alpha-post", AutoFix: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.TrimSpace(result.Diff) == "" {
		t.Fatalf("expected non-empty diff")
	}
	foundFix := false
	for _, c := range result.QA.Checks {
		if c.Name == "dash_cleanup" && len(c.Fixes) > 0 {
			foundFix = true
		}
	}
	if !foundFix {
		t.Fatalf("expected attached fixes for dash_cleanup")
	}
}

func TestRunLLMCheckError(t *testing.T) {
	store, contentDir := newQAStore(t)
	writeQAFile(t, filepath.Join(contentDir, "voice.md"), "voice")
	writeQAFile(t, filepath.Join(store.SlugDir("alpha-post"), "article.md"), strings.Repeat("plain words ", 550))

	engine := &Engine{Store: store, ContentDir: contentDir, LLM: &mockLLM{err: context.DeadlineExceeded}}
	if _, err := engine.Run(context.Background(), RunOptions{Slug: "alpha-post"}); err == nil {
		t.Fatalf("expected llm check error")
	}
}

func newQAStore(t *testing.T) (*content.Store, string) {
	t.Helper()
	contentDir := filepath.Join(t.TempDir(), "content")
	store := content.NewStore(contentDir)
	if err := store.Create("alpha-post", "alpha post"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return store, contentDir
}

func writeQAFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
