package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type HTTPConfig struct {
	URL             string
	FieldMap        map[string]string
	AuthHeader      string
	AuthToken       string
	AuthPrefix      string
	ResponseURLPath string
	Client          *http.Client
}

type HTTPPublisher struct {
	cfg HTTPConfig
}

func NewHTTPPublisher(cfg HTTPConfig) *HTTPPublisher {
	return &HTTPPublisher{cfg: cfg}
}

func (p *HTTPPublisher) Publish(ctx context.Context, item PublishItem) (PublishResult, error) {
	if strings.TrimSpace(p.cfg.URL) == "" {
		return PublishResult{}, fmt.Errorf("publish URL is required")
	}

	payload := p.mapFields(item)
	body, err := json.Marshal(payload)
	if err != nil {
		return PublishResult{}, fmt.Errorf("marshal publish payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return PublishResult{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	authHeader := strings.TrimSpace(p.cfg.AuthHeader)
	if authHeader == "" {
		authHeader = "Authorization"
	}
	if strings.TrimSpace(p.cfg.AuthToken) != "" {
		req.Header.Set(authHeader, p.cfg.AuthPrefix+p.cfg.AuthToken)
	}

	client := p.cfg.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return PublishResult{}, fmt.Errorf("publish request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PublishResult{}, fmt.Errorf("read publish response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PublishResult{}, fmt.Errorf("publish request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if len(bytes.TrimSpace(respBody)) == 0 {
		return PublishResult{}, nil
	}

	var parsed any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return PublishResult{}, fmt.Errorf("decode publish response: %w", err)
	}

	urlPath := strings.TrimSpace(p.cfg.ResponseURLPath)
	if urlPath == "" {
		urlPath = "url"
	}

	result := PublishResult{URL: asString(getByPath(parsed, urlPath)), ID: asString(getByPath(parsed, "id"))}
	return result, nil
}

func (p *HTTPPublisher) mapFields(item PublishItem) map[string]any {
	fieldMap := p.cfg.FieldMap
	if len(fieldMap) == 0 {
		fieldMap = map[string]string{
			"title":      "title",
			"slug":       "slug",
			"content":    "content",
			"summary":    "summary",
			"image_url":  "image_url",
			"image_path": "image_path",
		}
	}

	payload := make(map[string]any, len(fieldMap))
	for target, source := range fieldMap {
		payload[target] = itemField(item, source)
	}
	return payload
}

func itemField(item PublishItem, name string) any {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "title":
		return item.Title
	case "slug":
		return item.Slug
	case "content", "body":
		return item.Content
	case "summary":
		return item.Summary
	case "image_url", "imageurl":
		return item.ImageURL
	case "image_path", "imagepath":
		return item.ImagePath
	case "meta":
		return item.Meta
	default:
		if item.Meta != nil {
			if v, ok := item.Meta[name]; ok {
				return v
			}
		}
		return nil
	}
}

func getByPath(value any, path string) any {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	curr := value
	for _, part := range strings.Split(path, ".") {
		obj, ok := curr.(map[string]any)
		if !ok {
			return nil
		}
		next, ok := obj[part]
		if !ok {
			return nil
		}
		curr = next
	}
	return curr
}

func asString(v any) string {
	switch tv := v.(type) {
	case string:
		return tv
	case float64:
		return strconv.FormatFloat(tv, 'f', -1, 64)
	case int:
		return strconv.Itoa(tv)
	default:
		return ""
	}
}
