package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/contentai/internal/config"
)

const defaultMemdURL = "http://localhost:7457"

// SearchResult represents one memory snippet match.
type SearchResult struct {
	Text   string
	Source string
}

// Reader loads OpenClaw workspace conversation and memory context.
type Reader struct {
	HTTPClient *http.Client
	MemdURL    string
}

// NewReader builds a reader with defaults suitable for CLI usage.
func NewReader(client *http.Client) *Reader {
	if client == nil {
		client = http.DefaultClient
	}
	return &Reader{HTTPClient: client, MemdURL: defaultMemdURL}
}

// ReadConversationHistory reads memory markdown and transcript files from the workspace.
func (r *Reader) ReadConversationHistory(cfg config.OpenClawConfig) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}
	workspace, err := resolveWorkspace(cfg.Workspace)
	if err != nil {
		return "", err
	}
	if workspace == "" {
		return "", nil
	}

	var parts []string

	memoryParts, err := readMarkdownDir(filepath.Join(workspace, "memory"))
	if err != nil {
		return "", err
	}
	if len(memoryParts) > 0 {
		parts = append(parts, strings.Join(memoryParts, "\n\n"))
	}

	if cfg.ChannelHistory {
		transcriptPaths := []string{
			filepath.Join(workspace, "channel-history.md"),
			filepath.Join(workspace, "conversation.md"),
		}
		for _, dir := range []string{"conversations", "transcripts"} {
			items, readErr := readMarkdownDir(filepath.Join(workspace, dir))
			if readErr != nil {
				return "", readErr
			}
			if len(items) > 0 {
				parts = append(parts, strings.Join(items, "\n\n"))
			}
		}
		for _, path := range transcriptPaths {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				if os.IsNotExist(readErr) {
					continue
				}
				return "", fmt.Errorf("read transcript (%s): %w", path, readErr)
			}
			trimmed := strings.TrimSpace(string(data))
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n")), nil
}

// SearchMemory queries memd first, then falls back to scanning workspace memory markdown.
func (r *Reader) SearchMemory(ctx context.Context, cfg config.OpenClawConfig, query string, limit int) ([]SearchResult, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}

	workspace, err := resolveWorkspace(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	if workspace == "" {
		return nil, nil
	}

	memdResults, err := r.searchMemd(ctx, query, limit)
	if err == nil && len(memdResults) > 0 {
		return memdResults, nil
	}
	return grepMemoryFiles(workspace, query, limit)
}

func (r *Reader) searchMemd(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	base := strings.TrimSpace(r.MemdURL)
	if base == "" {
		base = defaultMemdURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse memd url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/search"
	q := u.Query()
	q.Set("q", query)
	q.Set("limit", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()

	client := r.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("memd status: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Results []struct {
			Text    string `json:"text"`
			Snippet string `json:"snippet"`
			Content string `json:"content"`
			Source  string `json:"source"`
			File    string `json:"file"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	out := make([]SearchResult, 0, len(parsed.Results))
	for _, item := range parsed.Results {
		text := firstNonEmpty(item.Text, item.Snippet, item.Content)
		source := firstNonEmpty(item.Source, item.File)
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, SearchResult{Text: strings.TrimSpace(text), Source: strings.TrimSpace(source)})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func grepMemoryFiles(workspace, query string, limit int) ([]SearchResult, error) {
	memoryDir := filepath.Join(workspace, "memory")
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read memory dir (%s): %w", memoryDir, err)
	}
	queryLower := strings.ToLower(query)

	out := make([]SearchResult, 0, limit)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(memoryDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read memory file (%s): %w", path, readErr)
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.Contains(strings.ToLower(trimmed), queryLower) {
				out = append(out, SearchResult{Text: trimmed, Source: path})
				if len(out) >= limit {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

func readMarkdownDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir (%s): %w", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read file (%s): %w", path, readErr)
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts, nil
}

func resolveWorkspace(path string) (string, error) {
	workspace := strings.TrimSpace(path)
	if workspace == "" {
		return "", nil
	}
	if workspace == "~" || strings.HasPrefix(workspace, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		if workspace == "~" {
			return home, nil
		}
		workspace = filepath.Join(home, strings.TrimPrefix(workspace, "~/"))
	}
	return workspace, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
