package draft

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/contentai/internal/content"
)

type Context struct {
	VoiceProfile string
	Blueprint    string
	Examples     []string
	Meta         *content.Meta
	Article      string
	Source       string
	Conversation string
}

func AssembleContext(store *content.Store, slug, sourcePath, conversation string) (*Context, error) {
	if store == nil {
		return nil, fmt.Errorf("content store is required")
	}
	if err := content.ValidateSlug(slug); err != nil {
		return nil, err
	}

	voicePath := filepath.Join(store.ContentDir, "voice.md")
	voice, err := os.ReadFile(voicePath)
	if err != nil {
		return nil, fmt.Errorf("read voice profile (%s): %w", voicePath, err)
	}

	blueprintPath := filepath.Join(store.ContentDir, "blueprint.md")
	blueprint, err := os.ReadFile(blueprintPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read blueprint (%s): %w", blueprintPath, err)
	}

	examples, err := readExamples(filepath.Join(store.ContentDir, "examples"))
	if err != nil {
		return nil, err
	}

	meta, err := store.Get(slug)
	if err != nil {
		return nil, err
	}

	article, err := store.ReadArticle(slug)
	if err != nil {
		return nil, err
	}

	source := ""
	if strings.TrimSpace(sourcePath) != "" {
		raw, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read source file (%s): %w", sourcePath, err)
		}
		source = strings.TrimSpace(string(raw))
	}

	return &Context{
		VoiceProfile: strings.TrimSpace(string(voice)),
		Blueprint:    strings.TrimSpace(string(blueprint)),
		Examples:     examples,
		Meta:         meta,
		Article:      strings.TrimSpace(article),
		Source:       source,
		Conversation: strings.TrimSpace(conversation),
	}, nil
}

func (c *Context) CoreIdeas() string {
	text := strings.TrimSpace(c.Blueprint)
	if text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	idx := strings.Index(lower, "core ideas")
	if idx < 0 {
		return text
	}
	start := strings.LastIndex(text[:idx], "\n")
	if start < 0 {
		start = 0
	}
	section := strings.TrimSpace(text[start:])
	if section == "" {
		return text
	}
	return section
}

func (c *Context) Outline() string {
	var b strings.Builder
	if c.Meta != nil {
		b.WriteString("Title: ")
		b.WriteString(strings.TrimSpace(c.Meta.Title))
		if strings.TrimSpace(c.Meta.Summary) != "" {
			b.WriteString("\nSummary: ")
			b.WriteString(strings.TrimSpace(c.Meta.Summary))
		}
		if strings.TrimSpace(c.Meta.Category) != "" {
			b.WriteString("\nTopic: ")
			b.WriteString(strings.TrimSpace(c.Meta.Category))
		}
	}
	if strings.TrimSpace(c.Article) != "" {
		b.WriteString("\n\nExisting Draft:\n")
		b.WriteString(strings.TrimSpace(c.Article))
	}
	return strings.TrimSpace(b.String())
}

func (c *Context) FullText() string {
	var b strings.Builder
	b.WriteString("VOICE PROFILE:\n")
	b.WriteString(strings.TrimSpace(c.VoiceProfile))

	if strings.TrimSpace(c.Blueprint) != "" {
		b.WriteString("\n\nBLUEPRINT:\n")
		b.WriteString(strings.TrimSpace(c.Blueprint))
	}

	if c.Meta != nil {
		b.WriteString("\n\nMETA:\n")
		b.WriteString("Title: ")
		b.WriteString(strings.TrimSpace(c.Meta.Title))
		b.WriteString("\nSlug: ")
		b.WriteString(strings.TrimSpace(c.Meta.Slug))
		if strings.TrimSpace(c.Meta.Summary) != "" {
			b.WriteString("\nSummary: ")
			b.WriteString(strings.TrimSpace(c.Meta.Summary))
		}
		if strings.TrimSpace(c.Meta.Category) != "" {
			b.WriteString("\nTopic: ")
			b.WriteString(strings.TrimSpace(c.Meta.Category))
		}
		b.WriteString("\nStatus: ")
		b.WriteString(string(c.Meta.Status))
	}

	if strings.TrimSpace(c.Article) != "" {
		b.WriteString("\n\nEXISTING ARTICLE DRAFT:\n")
		b.WriteString(strings.TrimSpace(c.Article))
	}

	if len(c.Examples) > 0 {
		b.WriteString("\n\nEXAMPLES:\n")
		for i, ex := range c.Examples {
			b.WriteString(fmt.Sprintf("\n--- Example %d ---\n", i+1))
			b.WriteString(strings.TrimSpace(ex))
		}
	}

	if strings.TrimSpace(c.Source) != "" {
		b.WriteString("\n\nSOURCE MATERIAL:\n")
		b.WriteString(strings.TrimSpace(c.Source))
	}

	if strings.TrimSpace(c.Conversation) != "" {
		b.WriteString("\n\nCONVERSATION HISTORY:\n")
		b.WriteString(strings.TrimSpace(c.Conversation))
	}

	return strings.TrimSpace(b.String())
}

func readExamples(examplesDir string) ([]string, error) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read examples dir (%s): %w", examplesDir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	examples := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(examplesDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read example (%s): %w", path, err)
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed != "" {
			examples = append(examples, trimmed)
		}
	}
	return examples, nil
}
