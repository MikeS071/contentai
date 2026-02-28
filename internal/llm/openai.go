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

type openAIClient struct {
	model   string
	apiKey  string
	baseURL string
	http    *http.Client
}

func newOpenAIClient(model, apiKey, baseURL string) *openAIClient {
	return &openAIClient{
		model:   model,
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *openAIClient) Name() string { return "openai" }

func (c *openAIClient) Complete(ctx context.Context, req Request) (*Response, error) {
	payload := map[string]any{
		"model":       c.pickModel(req.Model),
		"messages":    req.Messages,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"stream":      false,
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		respBody, status, err := c.doJSON(ctx, payload)
		if err != nil {
			return nil, err
		}
		if status == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("openai rate limited: %s", parseAPIError(respBody))
			time.Sleep(backoff(attempt))
			continue
		}
		if status == http.StatusUnauthorized {
			return nil, fmt.Errorf("openai auth error: %s", parseAPIError(respBody))
		}
		if status >= 400 {
			return nil, fmt.Errorf("openai request failed (%d): %s", status, parseAPIError(respBody))
		}

		var out struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil {
			return nil, fmt.Errorf("decode openai response: %w", err)
		}
		if len(out.Choices) == 0 {
			return nil, errors.New("openai response has no choices")
		}
		return &Response{
			Content:      out.Choices[0].Message.Content,
			TokensIn:     out.Usage.PromptTokens,
			TokensOut:    out.Usage.CompletionTokens,
			FinishReason: out.Choices[0].FinishReason,
		}, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("openai request failed")
}

func (c *openAIClient) Stream(ctx context.Context, req Request) (<-chan StreamChunk, error) {
	payload := map[string]any{
		"model":       c.pickModel(req.Model),
		"messages":    req.Messages,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"stream":      true,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	hreq.Header.Set("Authorization", "Bearer "+c.apiKey)
	hreq.Header.Set("Content-Type", "application/json")

	hresp, err := c.http.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("openai stream request: %w", err)
	}
	if hresp.StatusCode >= 400 {
		defer hresp.Body.Close()
		body, _ := io.ReadAll(hresp.Body)
		if hresp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("openai rate limited: %s", parseAPIError(body))
		}
		if hresp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("openai auth error: %s", parseAPIError(body))
		}
		return nil, fmt.Errorf("openai stream failed (%d): %s", hresp.StatusCode, parseAPIError(body))
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
			if data == "[DONE]" {
				chunks <- StreamChunk{Done: true}
				return
			}

			var evt struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &evt); err != nil {
				chunks <- StreamChunk{Error: fmt.Errorf("parse openai stream event: %w", err)}
				return
			}
			if len(evt.Choices) == 0 {
				continue
			}
			if txt := evt.Choices[0].Delta.Content; txt != "" {
				chunks <- StreamChunk{Content: txt}
			}
		}
		if err := s.Err(); err != nil {
			chunks <- StreamChunk{Error: err}
		}
	}()

	return chunks, nil
}

func (c *openAIClient) doJSON(ctx context.Context, payload map[string]any) ([]byte, int, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal openai request: %w", err)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, 0, fmt.Errorf("create openai request: %w", err)
	}
	hreq.Header.Set("Authorization", "Bearer "+c.apiKey)
	hreq.Header.Set("Content-Type", "application/json")

	hresp, err := c.http.Do(hreq)
	if err != nil {
		return nil, 0, fmt.Errorf("openai request failed: %w", err)
	}
	defer hresp.Body.Close()

	body, err := io.ReadAll(hresp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read openai response: %w", err)
	}
	return body, hresp.StatusCode, nil
}

func (c *openAIClient) pickModel(model string) string {
	if strings.TrimSpace(model) != "" {
		return model
	}
	return c.model
}

func parseAPIError(body []byte) string {
	var v struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &v); err == nil && v.Error.Message != "" {
		return v.Error.Message
	}
	if strings.TrimSpace(string(body)) == "" {
		return "empty error body"
	}
	return strings.TrimSpace(string(body))
}

func backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	return time.Duration(100*(attempt+1)) * time.Millisecond
}
