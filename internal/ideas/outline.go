package ideas

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/MikeS071/contentai/internal/content"
)

func parseOutlines(raw string) []Outline {
	sections := splitByIdea(raw)
	out := make([]Outline, 0, len(sections))
	for _, sec := range sections {
		idea := Outline{
			Title:             parseSectionValue(sec, "Working Title"),
			CoreParadox:       parseSectionValue(sec, "Core Paradox"),
			TransformationArc: parseSectionValue(sec, "Transformation Arc"),
			KeyExamples:       parseSectionList(sec, "Key Examples"),
			ActionableSteps:   parseSectionList(sec, "Actionable Steps"),
			Raw:               strings.TrimSpace(sec),
		}
		if idea.Title != "" {
			out = append(out, idea)
		}
	}
	return out
}

func splitByIdea(raw string) []string {
	lines := strings.Split(raw, "\n")
	chunks := make([]string, 0)
	var current []string
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## Idea") {
			if len(current) > 0 {
				chunks = append(chunks, strings.TrimSpace(strings.Join(current, "\n")))
			}
			current = []string{line}
			continue
		}
		if len(current) > 0 {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.TrimSpace(strings.Join(current, "\n")))
	}
	return chunks
}

func parseSectionValue(section, heading string) string {
	lines := strings.Split(section, "\n")
	capture := false
	parts := make([]string, 0)
	target := "### " + heading
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == target {
			capture = true
			continue
		}
		if capture && strings.HasPrefix(trimmed, "### ") {
			break
		}
		if capture && trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func parseSectionList(section, heading string) []string {
	lines := strings.Split(section, "\n")
	capture := false
	parts := make([]string, 0)
	target := "### " + heading
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == target {
			capture = true
			continue
		}
		if capture && strings.HasPrefix(trimmed, "### ") {
			break
		}
		if capture && strings.HasPrefix(trimmed, "- ") {
			parts = append(parts, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}
	return parts
}

func outlineToSeedMarkdown(o Outline) string {
	var b strings.Builder
	b.WriteString("## ")
	b.WriteString(strings.TrimSpace(o.Title))
	b.WriteString("\n\n### Core Paradox\n")
	b.WriteString(strings.TrimSpace(o.CoreParadox))
	b.WriteString("\n\n### Transformation Arc\n")
	b.WriteString(strings.TrimSpace(o.TransformationArc))
	b.WriteString("\n\n### Key Examples\n")
	for _, ex := range o.KeyExamples {
		b.WriteString("- ")
		b.WriteString(ex)
		b.WriteString("\n")
	}
	if len(o.KeyExamples) == 0 {
		b.WriteString("- (add examples)\n")
	}
	b.WriteString("\n### Actionable Steps\n")
	for _, step := range o.ActionableSteps {
		b.WriteString("- ")
		b.WriteString(step)
		b.WriteString("\n")
	}
	if len(o.ActionableSteps) == 0 {
		b.WriteString("- (add steps)\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func uniqueSlug(store *content.Store, base string) string {
	if len(base) < 3 {
		base = "idea-post"
	}
	if !store.Exists(base) {
		return base
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !store.Exists(candidate) {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UTC().Unix())
}

func slugify(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 100 {
		slug = strings.Trim(slug[:100], "-")
	}
	if len(slug) < 3 {
		return "idea-post"
	}
	return slug
}
