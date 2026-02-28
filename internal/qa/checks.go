package qa

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/MikeS071/contentai/internal/content"
)

var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{name: "openai_key", re: regexp.MustCompile(`\bsk-[A-Za-z0-9]{10,}\b`)},
	{name: "aws_access_key", re: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{name: "password_assignment", re: regexp.MustCompile(`(?i)\b(password|passwd|pwd)\s*[:=]\s*\S+`)},
	{name: "token_assignment", re: regexp.MustCompile(`(?i)\btoken\s*[:=]\s*\S+`)},
	{name: "api_key_assignment", re: regexp.MustCompile(`(?i)\bapi[_-]?key\s*[:=]\s*\S+`)},
}

var dashPattern = regexp.MustCompile(`\b\w+\s[-–—]\s\w+\b`)

func CheckNoSecrets(article string) content.QACheck {
	issues := make([]string, 0)
	seen := map[string]struct{}{}
	for _, p := range secretPatterns {
		matches := p.re.FindAllString(article, -1)
		for _, m := range matches {
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			issues = append(issues, fmt.Sprintf("possible secret (%s): %s", p.name, m))
		}
	}
	sort.Strings(issues)
	return content.QACheck{Name: "no_secrets", Passed: len(issues) == 0, Issues: issues}
}

func CheckDashCleanup(article string) content.QACheck {
	matches := dashPattern.FindAllString(article, -1)
	issues := make([]string, 0, len(matches))
	for _, m := range matches {
		issues = append(issues, "unnecessary mid-sentence dash: "+m)
	}
	return content.QACheck{Name: "dash_cleanup", Passed: len(issues) == 0, Issues: issues}
}

func CheckDedup(article string) content.QACheck {
	sentences := splitSentences(article)
	seen := map[string]int{}
	issues := make([]string, 0)
	for i, s := range sentences {
		norm := normalizeSentence(s)
		if norm == "" {
			continue
		}
		if first, ok := seen[norm]; ok {
			issues = append(issues, fmt.Sprintf("repeated sentence at #%d and #%d: %q", first+1, i+1, strings.TrimSpace(s)))
			continue
		}
		seen[norm] = i
	}
	return content.QACheck{Name: "dedup", Passed: len(issues) == 0, Issues: issues}
}

func CheckReadingLevel(article string) content.QACheck {
	sentences := splitSentences(article)
	issues := make([]string, 0)
	for _, s := range sentences {
		words := strings.Fields(s)
		if len(words) == 0 {
			continue
		}
		if len(words) > 30 {
			issues = append(issues, "very long sentence: "+truncate(strings.TrimSpace(s), 140))
			continue
		}
		longWords := 0
		for _, w := range words {
			clean := strings.Trim(strings.ToLower(w), ",.;:!?()[]{}\"'")
			if len(clean) >= 12 {
				longWords++
			}
		}
		if longWords >= 3 {
			issues = append(issues, "overly complex wording: "+truncate(strings.TrimSpace(s), 140))
		}
	}
	return content.QACheck{Name: "reading_level", Passed: len(issues) == 0, Issues: issues}
}

func CheckLength(article string) content.QACheck {
	count := len(strings.Fields(article))
	if count < 500 {
		return content.QACheck{Name: "length", Passed: false, Issues: []string{fmt.Sprintf("word count %d is below target range (500-1000)", count)}}
	}
	if count > 1000 {
		return content.QACheck{Name: "length", Passed: false, Issues: []string{fmt.Sprintf("word count %d is above target range (500-1000)", count)}}
	}
	return content.QACheck{Name: "length", Passed: true}
}

func checkLLMOutput(name, raw string) content.QACheck {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "pass") || trimmed == "[]" || trimmed == "{}" {
		return content.QACheck{Name: name, Passed: true}
	}

	var arr []string
	if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return content.QACheck{Name: name, Passed: len(out) == 0, Issues: out}
	}

	lines := strings.Split(trimmed, "\n")
	issues := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if line != "" {
			issues = append(issues, line)
		}
	}
	if len(issues) == 0 {
		issues = []string{trimmed}
	}
	return content.QACheck{Name: name, Passed: false, Issues: issues}
}

func splitSentences(text string) []string {
	replacer := strings.NewReplacer("\n", " ", "\t", " ")
	text = strings.TrimSpace(replacer.Replace(text))
	if text == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeSentence(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(",", "", ";", "", ":", "", "!", "", "?", "", "\"", "", "'", "", "(", "", ")", "").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
