package ideas

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/kb"
	"github.com/MikeS071/contentai/internal/llm"
	"github.com/MikeS071/contentai/internal/templates"
)

const (
	defaultIdeaCount = 5
	recentKBLimit    = 12
)

type Outline struct {
	Title             string
	CoreParadox       string
	TransformationArc string
	KeyExamples       []string
	ActionableSteps   []string
	Raw               string
}

type GenerateOptions struct {
	FromKB             bool
	FromConversations  bool
	ConversationSource string
	Count              int
}

type Generator struct {
	ContentDir string
	LLM        llm.LLMClient
	KB         *kb.Store
	Templates  *templates.Engine
	Content    *content.Store
	Now        func() time.Time
}

func NewGenerator(contentDir string, client llm.LLMClient, kbStore *kb.Store, tpl *templates.Engine, contentStore *content.Store) *Generator {
	if kbStore == nil {
		kbStore = kb.NewStore(contentDir)
	}
	if tpl == nil {
		tpl = templates.NewEngine(contentDir)
	}
	if contentStore == nil {
		contentStore = content.NewStore(contentDir)
	}
	return &Generator{
		ContentDir: contentDir,
		LLM:        client,
		KB:         kbStore,
		Templates:  tpl,
		Content:    contentStore,
		Now:        time.Now,
	}
}

func (g *Generator) Generate(ctx context.Context, opts GenerateOptions) ([]Outline, error) {
	if g == nil {
		return nil, fmt.Errorf("generator is nil")
	}
	if g.LLM == nil {
		return nil, fmt.Errorf("llm client is required")
	}

	count := opts.Count
	if count <= 0 {
		count = defaultIdeaCount
	}

	sourceMaterial := ""
	if opts.FromKB {
		kbText, err := g.gatherKBSource(recentKBLimit)
		if err != nil {
			return nil, err
		}
		sourceMaterial = strings.TrimSpace(kbText)
	}

	taskPrompt, err := g.Templates.Get("deep-post-ideas")
	if err != nil {
		return nil, err
	}
	taskPrompt = strings.ReplaceAll(taskPrompt, "Produce exactly 5 outlines.", fmt.Sprintf("Produce exactly %d outlines.", count))

	conversation := ""
	if opts.FromConversations {
		conversation = strings.TrimSpace(opts.ConversationSource)
	}

	ctxPayload, err := llm.AssembleContext(g.ContentDir, llm.WithSource(sourceMaterial), llm.WithConversation(conversation))
	if err != nil {
		return nil, err
	}
	messages := ctxPayload.BuildMessages(taskPrompt, 0)

	resp, err := g.LLM.Complete(ctx, llm.Request{Messages: messages})
	if err != nil {
		return nil, err
	}

	outlines := parseOutlines(resp.Content)
	if len(outlines) > count {
		outlines = outlines[:count]
	}
	if len(outlines) == 0 {
		return nil, fmt.Errorf("no idea outlines returned")
	}
	return outlines, nil
}

func (g *Generator) SaveBatch(outlines []Outline) (string, error) {
	if len(outlines) == 0 {
		return "", fmt.Errorf("outlines are required")
	}
	now := time.Now().UTC()
	if g.Now != nil {
		now = g.Now().UTC()
	}
	date := now.Format("2006-01-02")

	ideasDir := filepath.Join(g.ContentDir, "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		return "", fmt.Errorf("create ideas dir: %w", err)
	}
	path := filepath.Join(ideasDir, date+"-batch.md")

	var b strings.Builder
	b.WriteString("# Idea Batch - ")
	b.WriteString(date)
	b.WriteString("\n\n")

	for i, idea := range outlines {
		b.WriteString(fmt.Sprintf("## Idea %d: %s\n", i+1, strings.TrimSpace(idea.Title)))
		b.WriteString("\n### Core Paradox\n")
		b.WriteString(strings.TrimSpace(idea.CoreParadox))
		b.WriteString("\n\n### Transformation Arc\n")
		b.WriteString(strings.TrimSpace(idea.TransformationArc))
		b.WriteString("\n\n### Key Examples\n")
		for _, ex := range idea.KeyExamples {
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(strings.TrimPrefix(ex, "-")))
			b.WriteString("\n")
		}
		if len(idea.KeyExamples) == 0 {
			b.WriteString("- (none)\n")
		}
		b.WriteString("\n### Actionable Steps\n")
		for _, step := range idea.ActionableSteps {
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(strings.TrimPrefix(step, "-")))
			b.WriteString("\n")
		}
		if len(idea.ActionableSteps) == 0 {
			b.WriteString("- (none)\n")
		}
		b.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(strings.TrimSpace(b.String())+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write idea batch: %w", err)
	}
	return path, nil
}

func (g *Generator) PickAndCreate(in io.Reader, out io.Writer, outlines []Outline) (string, error) {
	if len(outlines) == 0 {
		return "", fmt.Errorf("no outlines to pick from")
	}
	for i, idea := range outlines {
		fmt.Fprintf(out, "%d. %s\n", i+1, idea.Title)
	}
	fmt.Fprint(out, "Pick idea number to develop (0 to skip): ")

	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read pick: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	pick, err := strconv.Atoi(line)
	if err != nil {
		return "", fmt.Errorf("invalid selection %q", line)
	}
	if pick == 0 {
		return "", nil
	}
	if pick < 1 || pick > len(outlines) {
		return "", fmt.Errorf("selection out of range: %d", pick)
	}

	selected := outlines[pick-1]
	slug := uniqueSlug(g.Content, slugify(selected.Title))
	if err := g.Content.Create(slug, selected.Title); err != nil {
		return "", err
	}
	if err := g.Content.WriteArticle(slug, outlineToSeedMarkdown(selected)); err != nil {
		return "", err
	}
	fmt.Fprintf(out, "Created content item: %s\n", g.Content.SlugDir(slug))
	return slug, nil
}

func (g *Generator) gatherKBSource(limit int) (string, error) {
	root := g.ContentDir
	if g.KB != nil && strings.TrimSpace(g.KB.ContentDir) != "" {
		root = g.KB.ContentDir
	}
	kbRoot := filepath.Join(root, "kb")

	files, err := markdownFiles(filepath.Join(kbRoot, "blogs"), true)
	if err != nil {
		return "", err
	}
	notes, err := markdownFiles(filepath.Join(kbRoot, "notes"), false)
	if err != nil {
		return "", err
	}
	files = append(files, notes...)
	if len(files) == 0 {
		return "", nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].mod.After(files[j].mod)
	})
	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}

	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			return "", fmt.Errorf("read kb source %s: %w", f.path, err)
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed == "" {
			continue
		}
		b.WriteString("\n--- SOURCE: ")
		b.WriteString(filepath.Base(f.path))
		b.WriteString(" ---\n")
		b.WriteString(trimmed)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}

type fileInfo struct {
	path string
	mod  time.Time
}

func markdownFiles(root string, recursive bool) ([]fileInfo, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]fileInfo, 0)
	if recursive {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".md" {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			files = append(files, fileInfo{path: path, mod: info.ModTime()})
			return nil
		})
		return files, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, fileInfo{path: filepath.Join(root, entry.Name()), mod: info.ModTime()})
	}
	return files, nil
}
