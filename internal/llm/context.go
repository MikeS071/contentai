package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ContentContext struct {
	VoiceProfile string
	Blueprint    string
	Examples     []string
	Source       string
	Conversation string
	CustomRules  string
}

type assembleOptions struct {
	source       string
	conversation string
	customRules  string
}

type ContextOption func(*assembleOptions)

func WithSource(source string) ContextOption {
	return func(o *assembleOptions) { o.source = source }
}

func WithConversation(conversation string) ContextOption {
	return func(o *assembleOptions) { o.conversation = conversation }
}

func WithCustomRules(rules string) ContextOption {
	return func(o *assembleOptions) { o.customRules = rules }
}

func AssembleContext(contentDir string, opts ...ContextOption) (*ContentContext, error) {
	cfg := &assembleOptions{}
	for _, opt := range opts {
		opt(cfg)
	}

	voicePath := filepath.Join(contentDir, "voice.md")
	voice, err := os.ReadFile(voicePath)
	if err != nil {
		return nil, fmt.Errorf("read voice profile (%s): %w", voicePath, err)
	}

	bpPath := filepath.Join(contentDir, "blueprint.md")
	blueprint, err := os.ReadFile(bpPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read blueprint (%s): %w", bpPath, err)
	}

	examples, err := readExamples(filepath.Join(contentDir, "examples"))
	if err != nil {
		return nil, err
	}

	return &ContentContext{
		VoiceProfile: strings.TrimSpace(string(voice)),
		Blueprint:    strings.TrimSpace(string(blueprint)),
		Examples:     examples,
		Source:       strings.TrimSpace(cfg.source),
		Conversation: strings.TrimSpace(cfg.conversation),
		CustomRules:  strings.TrimSpace(cfg.customRules),
	}, nil
}

func (cc *ContentContext) BuildMessages(taskPrompt string, maxTokens int) []Message {
	sys := buildSystemMessage(cc.VoiceProfile, cc.Blueprint)
	examples := append([]string(nil), cc.Examples...)
	conversation := cc.Conversation

	for {
		user := buildUserMessage(taskPrompt, cc.Source, conversation, cc.CustomRules, examples)
		total := estimateTokens(sys) + estimateTokens(user)
		if maxTokens <= 0 || total <= maxTokens {
			return []Message{{Role: "system", Content: sys}, {Role: "user", Content: user}}
		}

		if len(examples) > 0 {
			examples = examples[1:]
			continue
		}

		if conversation != "" {
			next := truncateToTokens(conversation, maxTokens/5)
			if next == conversation {
				conversation = ""
			} else {
				conversation = next
			}
			continue
		}

		user = buildUserMessage(taskPrompt, truncateToTokens(cc.Source, maxTokens/3), "", cc.CustomRules, nil)
		return []Message{{Role: "system", Content: sys}, {Role: "user", Content: user}}
	}
}

func buildSystemMessage(voice, blueprint string) string {
	bpCore := extractBlueprintCoreIdeas(blueprint)
	var b strings.Builder
	b.WriteString("VOICE PROFILE (ALWAYS APPLY):\n")
	b.WriteString(strings.TrimSpace(voice))
	if bpCore != "" {
		b.WriteString("\n\nBLUEPRINT CORE IDEAS:\n")
		b.WriteString(bpCore)
	}
	return b.String()
}

func buildUserMessage(taskPrompt, source, conversation, customRules string, examples []string) string {
	var b strings.Builder
	b.WriteString("TASK:\n")
	b.WriteString(strings.TrimSpace(taskPrompt))

	if strings.TrimSpace(source) != "" {
		b.WriteString("\n\nSOURCE MATERIAL:\n")
		b.WriteString(strings.TrimSpace(source))
	}
	if strings.TrimSpace(conversation) != "" {
		b.WriteString("\n\nCONVERSATION HISTORY:\n")
		b.WriteString(strings.TrimSpace(conversation))
	}
	if strings.TrimSpace(customRules) != "" {
		b.WriteString("\n\nCUSTOM RULES:\n")
		b.WriteString(strings.TrimSpace(customRules))
	}
	if len(examples) > 0 {
		b.WriteString("\n\nEXAMPLES:\n")
		for i, ex := range examples {
			b.WriteString(fmt.Sprintf("\n--- Example %d ---\n", i+1))
			b.WriteString(strings.TrimSpace(ex))
		}
	}
	return b.String()
}

func extractBlueprintCoreIdeas(blueprint string) string {
	if strings.TrimSpace(blueprint) == "" {
		return ""
	}
	lower := strings.ToLower(blueprint)
	idx := strings.Index(lower, "core ideas")
	if idx < 0 {
		return strings.TrimSpace(blueprint)
	}
	start := strings.LastIndex(blueprint[:idx], "\n")
	if start < 0 {
		start = 0
	}
	section := strings.TrimSpace(blueprint[start:])
	if section == "" {
		return strings.TrimSpace(blueprint)
	}
	return section
}

func readExamples(examplesDir string) ([]string, error) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read examples dir (%s): %w", examplesDir, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		path := filepath.Join(examplesDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read example (%s): %w", path, err)
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out, nil
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len([]rune(s)) / 4) + 1
}

func truncateToTokens(s string, tokenLimit int) string {
	if tokenLimit <= 0 {
		return ""
	}
	limit := tokenLimit * 4
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	start := len(r) - limit
	if start < 0 {
		start = 0
	}
	return strings.TrimSpace(string(r[start:]))
}
