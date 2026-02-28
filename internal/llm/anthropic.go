package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type anthropicClient struct {
	model   string
	apiKey  string
	baseURL string
	http    *http.Client
}

func newAnthropicClient(model, apiKey, baseURL string) *anthropicClient {
	return &anthropicClient{
		model:   model,
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *anthropicClient) Name() string { return "anthropic" }

func (c *anthropicClient) Complete(ctx context.Context, req Request) (*Response, error) {
	system, messages := splitAnthropicMessages(req.Messages)
	payload := map[string]any{
		"model":       c.pickModel(req.Model),
		"system":      system,
		"messages":    messages,
		"temperature": req.Temperature,
		"max_tokens":  max(1, req.MaxTokens),
		"stream":      false,
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		body, status, err := c.doJSON(ctx, payload)
		if err != nil {
			return nil, err
		}
		if status == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("anthropic rate limited: %s", parseAPIError(body))
			time.Sleep(backoff(attempt))
			continue
		}
		if status == http.StatusUnauthorized {
			return nil, fmt.Errorf("anthropic auth error: %s", parseAPIError(body))
		}
		if status >= 400 {
			return nil, fmt.Errorf("anthropic request failed (%d): %s", status, parseAPIError(body))
		}

		var out struct {
			StopReason string `json:"stop_reason"`
			Usage      struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("decode anthropic response: %w", err)
		}
		parts := make([]string, 0, len(out.Content))
		for _, c := range out.Content {
			if c.Type == "text" && c.Text != "" {
				parts = append(parts, c.Text)
			}
		}
		if len(parts) == 0 {
			return nil, errors.New("anthropic response had no text content")
		}
		return &Response{
			Content:      strings.Join(parts, ""),
			TokensIn:     out.Usage.InputTokens,
			TokensOut:    out.Usage.OutputTokens,
			FinishReason: out.StopReason,
		}, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("anthropic request failed")
}

func (c *anthropicClient) Stream(ctx context.Context, req Request) (<-chan StreamChunk, error) {
	system, messages := splitAnthropicMessages(req.Messages)
	payload := map[string]any{
		"model":       c.pickModel(req.Model),
		"system":      system,
		"messages":    messages,
		"temperature": req.Temperature,
		"max_tokens":  max(1, req.MaxTokens),
		"stream":      true,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}
	hreq.Header.Set("x-api-key", c.apiKey)
	hreq.Header.Set("anthropic-version", "2023-06-01")
	hreq.Header.Set("Content-Type", "application/json")

	hresp, err := c.http.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("anthropic stream request failed: %w", err)
	}
	if hresp.StatusCode >= 400 {
		defer hresp.Body.Close()
		body, _ := io.ReadAll(hresp.Body)
		if hresp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("anthropic rate limited: %s", parseAPIError(body))
		}
		if hresp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("anthropic auth error: %s", parseAPIError(body))
		}
		return nil, fmt.Errorf("anthropic stream failed (%d): %s", hresp.StatusCode, parseAPIError(body))
	}

	chunks := make(chan StreamChunk)
	go func() {
		defer close(chunks)
		defer hresp.Body.Close()

		s := bufio.NewScanner(hresp.Body)
		s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			var evt struct {
				Type  string `json:"type"`
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &evt); err != nil {
				chunks <- StreamChunk{Error: fmt.Errorf("parse anthropic stream event: %w", err)}
				return
			}
			switch evt.Type {
			case "content_block_delta":
				if evt.Delta.Text != "" {
					chunks <- StreamChunk{Content: evt.Delta.Text}
				}
			case "message_stop":
				chunks <- StreamChunk{Done: true}
				return
			}
		}
		if err := s.Err(); err != nil {
			chunks <- StreamChunk{Error: err}
		}
	}()

	return chunks, nil
}

func splitAnthropicMessages(msgs []Message) (string, []map[string]string) {
	var systemParts []string
	out := make([]map[string]string, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				systemParts = append(systemParts, m.Content)
			}
		case "user", "assistant":
			out = append(out, map[string]string{"role": m.Role, "content": m.Content})
		}
	}
	return strings.Join(systemParts, "\n\n"), out
}

func (c *anthropicClient) doJSON(ctx context.Context, payload map[string]any) ([]byte, int, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal anthropic request: %w", err)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return nil, 0, fmt.Errorf("create anthropic request: %w", err)
	}
	hreq.Header.Set("x-api-key", c.apiKey)
	hreq.Header.Set("anthropic-version", "2023-06-01")
	hreq.Header.Set("Content-Type", "application/json")

	hresp, err := c.http.Do(hreq)
	if err != nil {
		return nil, 0, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer hresp.Body.Close()
	body, err := io.ReadAll(hresp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read anthropic response: %w", err)
	}
	return body, hresp.StatusCode, nil
}

func (c *anthropicClient) pickModel(model string) string {
	if strings.TrimSpace(model) != "" {
		return model
	}
	return c.model
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
