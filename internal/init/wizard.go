package initflow

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/llm"
	toml "github.com/pelletier/go-toml/v2"
)

const (
	maxArticles      = 5
	inlineEndToken   = "---END---"
	defaultLLMModel  = "gpt-4o-mini"
	defaultLLMVendor = "openai"
)

type templateGetter interface {
	Get(name string) (string, error)
}

type Wizard struct {
	Stdin   io.Reader
	Stdout  io.Writer
	WorkDir string
	Project string

	LLM        llm.LLMClient
	Templates  templateGetter
	APIKeyTest bool
}

func NewWizard(stdin io.Reader, stdout io.Writer, workDir, project string, client llm.LLMClient, engine templateGetter) *Wizard {
	return &Wizard{
		Stdin:      stdin,
		Stdout:     stdout,
		WorkDir:    workDir,
		Project:    strings.TrimSpace(project),
		LLM:        client,
		Templates:  engine,
		APIKeyTest: true,
	}
}

func (w *Wizard) Run(ctx context.Context) error {
	if w.Stdin == nil || w.Stdout == nil {
		return errors.New("stdin and stdout are required")
	}
	if strings.TrimSpace(w.WorkDir) == "" {
		return errors.New("workdir is required")
	}
	if w.LLM == nil {
		return errors.New("llm client is required")
	}
	if w.Templates == nil {
		return errors.New("template engine is required")
	}

	scanner := bufio.NewScanner(w.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	cfg, err := w.loadOrDefaultConfig()
	if err != nil {
		return err
	}
	if err := ensureWorkspace(cfg.Project.ContentDir, w.WorkDir); err != nil {
		return err
	}

	examplesDir := filepath.Join(w.WorkDir, cfg.Project.ContentDir, "examples")
	examples, err := w.collectExamples(scanner, examplesDir)
	if err != nil {
		return err
	}

	blueprint, err := w.runPerspectiveArchitect(ctx, cfg, examples)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(w.WorkDir, cfg.Project.ContentDir, "blueprint.md"), []byte(strings.TrimSpace(blueprint)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write blueprint.md: %w", err)
	}

	voice, err := w.generateVoiceProfile(ctx, cfg, examples, blueprint)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(w.WorkDir, cfg.Project.ContentDir, "voice.md"), []byte(strings.TrimSpace(voice)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write voice.md: %w", err)
	}

	if err := w.configureIntegrations(ctx, scanner, cfg); err != nil {
		return err
	}
	if err := w.saveConfig(cfg); err != nil {
		return err
	}

	feeds, err := w.collectFeeds(scanner)
	if err != nil {
		return err
	}
	if err := SaveFeeds(filepath.Join(w.WorkDir, cfg.Project.ContentDir, "kb", "feeds.toml"), FeedsConfig{Feeds: feeds}); err != nil {
		return err
	}
	return nil
}

func (w *Wizard) loadOrDefaultConfig() (*config.Config, error) {
	cfgPath := filepath.Join(w.WorkDir, "contentai.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = config.Default()
	}

	if strings.TrimSpace(cfg.Project.Name) == "" {
		if w.Project != "" {
			cfg.Project.Name = w.Project
		} else {
			cfg.Project.Name = "contentai-project"
		}
	}
	if strings.TrimSpace(cfg.Project.ContentDir) == "" {
		cfg.Project.ContentDir = "content"
	}
	if strings.TrimSpace(cfg.LLM.Provider) == "" {
		cfg.LLM.Provider = defaultLLMVendor
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		cfg.LLM.Model = defaultLLMModel
	}
	if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
		cfg.LLM.BaseURL = strings.TrimSpace(os.Getenv("CONTENTAI_LLM_BASE_URL"))
	}
	return cfg, nil
}

func ensureWorkspace(contentDir, workDir string) error {
	dirs := []string{
		filepath.Join(workDir, contentDir),
		filepath.Join(workDir, contentDir, "examples"),
		filepath.Join(workDir, contentDir, "kb"),
		filepath.Join(workDir, contentDir, "kb", "blogs"),
		filepath.Join(workDir, contentDir, "kb", "notes"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}
	return nil
}

func (w *Wizard) collectExamples(scanner *bufio.Scanner, examplesDir string) ([]string, error) {
	fmt.Fprintln(w.Stdout, "Paste 2-3 of your best articles (or paths to markdown files):")
	existing, nextNum, err := loadExistingExamples(examplesDir)
	if err != nil {
		return nil, err
	}

	articles := append([]string(nil), existing...)
	for len(articles) < maxArticles {
		fmt.Fprint(w.Stdout, "> ")
		line, ok := scanLine(scanner)
		if !ok {
			break
		}
		entry := strings.TrimSpace(line)
		if entry == "" {
			break
		}

		article, err := readEntry(scanner, entry)
		if err != nil {
			return nil, err
		}
		article = strings.TrimSpace(article)
		if article == "" {
			continue
		}

		if len(articles) >= maxArticles {
			return nil, fmt.Errorf("maximum %d articles allowed", maxArticles)
		}
		filename := filepath.Join(examplesDir, fmt.Sprintf("%d.md", nextNum))
		nextNum++
		if err := os.WriteFile(filename, []byte(article+"\n"), 0o644); err != nil {
			return nil, fmt.Errorf("write example article: %w", err)
		}
		articles = append(articles, article)
	}

	if len(articles) < 1 {
		return nil, errors.New("at least one example article is required")
	}
	return articles, nil
}

func loadExistingExamples(dir string) ([]string, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 1, nil
		}
		return nil, 1, fmt.Errorf("read examples dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	maxN := 0
	articles := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".md")
		if n, err := strconv.Atoi(base); err == nil && n > maxN {
			maxN = n
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, 1, fmt.Errorf("read example %q: %w", e.Name(), err)
		}
		if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
			articles = append(articles, trimmed)
		}
	}
	return articles, maxN + 1, nil
}

func readEntry(scanner *bufio.Scanner, entry string) (string, error) {
	if st, err := os.Stat(entry); err == nil && !st.IsDir() {
		data, readErr := os.ReadFile(entry)
		if readErr != nil {
			return "", fmt.Errorf("read article path %q: %w", entry, readErr)
		}
		return string(data), nil
	}

	lines := []string{entry}
	for {
		line, ok := scanLine(scanner)
		if !ok {
			return "", errors.New("inline article missing ---END--- delimiter")
		}
		if strings.TrimSpace(line) == inlineEndToken {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func (w *Wizard) runPerspectiveArchitect(ctx context.Context, cfg *config.Config, examples []string) (string, error) {
	prompt, err := w.Templates.Get("perspective-architect")
	if err != nil {
		return "", fmt.Errorf("load perspective-architect template: %w", err)
	}
	user := "Example Articles:\n\n" + strings.Join(examples, "\n\n---\n\n")
	resp, err := w.LLM.Complete(ctx, llm.Request{
		Model: cfg.LLM.Model,
		Messages: []llm.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", fmt.Errorf("run perspective architect: %w", err)
	}
	return resp.Content, nil
}

func (w *Wizard) generateVoiceProfile(ctx context.Context, cfg *config.Config, examples []string, blueprint string) (string, error) {
	prompt, err := w.Templates.Get("voice-extractor")
	if err != nil {
		return "", fmt.Errorf("load voice-extractor template: %w", err)
	}
	user := "Blueprint:\n\n" + strings.TrimSpace(blueprint) + "\n\nExample Articles:\n\n" + strings.Join(examples, "\n\n---\n\n")
	resp, err := w.LLM.Complete(ctx, llm.Request{
		Model: cfg.LLM.Model,
		Messages: []llm.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", fmt.Errorf("run voice extractor: %w", err)
	}
	return resp.Content, nil
}

func (w *Wizard) configureIntegrations(ctx context.Context, scanner *bufio.Scanner, cfg *config.Config) error {
	fmt.Fprint(w.Stdout, "LLM API key: ")
	llmKey, ok := scanLine(scanner)
	if !ok {
		return errors.New("missing LLM API key input")
	}
	llmKey = strings.TrimSpace(llmKey)
	if llmKey != "" {
		if err := w.validateLLMKey(ctx, cfg, llmKey); err != nil {
			return err
		}
		os.Setenv("CONTENTAI_LLM_API_KEY", llmKey)
		cfg.LLM.APIKeyCmd = `printf %s "$CONTENTAI_LLM_API_KEY"`
	}

	fmt.Fprint(w.Stdout, "Image API key (optional): ")
	imgKey, ok := scanLine(scanner)
	if !ok {
		return errors.New("missing image API key input")
	}
	imgKey = strings.TrimSpace(imgKey)
	if imgKey != "" {
		os.Setenv("CONTENTAI_IMAGE_API_KEY", imgKey)
		cfg.Images.APIKeyCmd = `printf %s "$CONTENTAI_IMAGE_API_KEY"`
		if strings.TrimSpace(cfg.Images.Provider) == "" {
			cfg.Images.Provider = "openai"
		}
		if strings.TrimSpace(cfg.Images.Model) == "" {
			cfg.Images.Model = "gpt-image-1"
		}
	}

	fmt.Fprint(w.Stdout, "Publish endpoint URL (optional): ")
	publishURL, ok := scanLine(scanner)
	if !ok {
		return errors.New("missing publish endpoint input")
	}
	publishURL = strings.TrimSpace(publishURL)
	if publishURL != "" {
		cfg.Publish.Type = "http"
		cfg.Publish.URL = publishURL
	}
	return nil
}

func (w *Wizard) validateLLMKey(ctx context.Context, cfg *config.Config, _ string) error {
	if !w.APIKeyTest {
		return nil
	}
	_, err := w.LLM.Complete(ctx, llm.Request{
		Model: cfg.LLM.Model,
		Messages: []llm.Message{
			{Role: "user", Content: "Respond with OK."},
		},
		MaxTokens: 8,
	})
	if err != nil {
		return fmt.Errorf("validate llm api key: %w", err)
	}
	return nil
}

func (w *Wizard) saveConfig(cfg *config.Config) error {
	path := filepath.Join(w.WorkDir, "contentai.toml")
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (w *Wizard) collectFeeds(scanner *bufio.Scanner) ([]string, error) {
	fmt.Fprintln(w.Stdout, "Add RSS feeds of blogs you follow (one per line, blank to finish):")
	feeds := []string{}
	for {
		fmt.Fprint(w.Stdout, "> ")
		line, ok := scanLine(scanner)
		if !ok {
			break
		}
		feed := strings.TrimSpace(line)
		if feed == "" {
			break
		}
		feeds = append(feeds, feed)
	}
	return feeds, nil
}

func scanLine(scanner *bufio.Scanner) (string, bool) {
	if !scanner.Scan() {
		return "", false
	}
	return scanner.Text(), true
}
