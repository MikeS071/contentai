package kb

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SearchResult struct {
	Title      string
	Snippet    string
	SourcePath string
	Score      int
}

func Search(kbDir, query string, limit int) ([]SearchResult, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	paths := make([]string, 0)
	err := filepath.WalkDir(kbDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk kb dir: %w", err)
	}

	results := make([]SearchResult, 0)
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", path, err)
		}
		title, body := extractTitleAndBody(string(content), path)
		titleLower := strings.ToLower(title)
		bodyLower := strings.ToLower(body)

		titleHits := strings.Count(titleLower, q)
		bodyHits := strings.Count(bodyLower, q)
		score := titleHits*3 + bodyHits
		if score <= 0 {
			continue
		}

		results = append(results, SearchResult{
			Title:      title,
			Snippet:    buildSnippet(body, q),
			SourcePath: path,
			Score:      score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Title != results[j].Title {
			return results[i].Title < results[j].Title
		}
		return results[i].SourcePath < results[j].SourcePath
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func extractTitleAndBody(content, path string) (string, string) {
	title := ""
	body := content

	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "\n---\n", 2)
		if len(parts) == 2 {
			for _, line := range strings.Split(parts[0], "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(strings.ToLower(line), "title:") {
					title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
					title = strings.Trim(title, "\"")
					break
				}
			}
			body = parts[1]
		}
	}
	if title == "" {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				title = strings.TrimSpace(strings.TrimLeft(line, "#"))
				break
			}
		}
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return title, body
}

func buildSnippet(body, queryLower string) string {
	clean := strings.Join(strings.Fields(body), " ")
	if clean == "" {
		return ""
	}
	lower := strings.ToLower(clean)
	idx := strings.Index(lower, queryLower)
	if idx < 0 {
		if len(clean) > 160 {
			return clean[:160] + "..."
		}
		return clean
	}
	start := idx - 60
	if start < 0 {
		start = 0
	}
	end := idx + len(queryLower) + 60
	if end > len(clean) {
		end = len(clean)
	}
	snippet := clean[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(clean) {
		snippet += "..."
	}
	return snippet
}
