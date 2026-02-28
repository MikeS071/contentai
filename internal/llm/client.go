package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	defaultOpenAIBaseURL    = "https://api.openai.com"
	defaultAnthropicBaseURL = "https://api.anthropic.com"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Messages    []Message
	Model       string
	Temperature float64
	MaxTokens   int
	Stream      bool
}

type Response struct {
	Content      string
	TokensIn     int
	TokensOut    int
	FinishReason string
}

type StreamChunk struct {
	Content string
	Done    bool
	Error   error
}

type LLMClient interface {
	Complete(ctx context.Context, req Request) (*Response, error)
	Stream(ctx context.Context, req Request) (<-chan StreamChunk, error)
	Name() string
}

func NewClient(provider, model, apiKey, baseURL string) (LLMClient, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("api key is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("model is required")
	}

	switch provider {
	case "openai":
		if baseURL == "" {
			baseURL = defaultOpenAIBaseURL
		}
		return newOpenAIClient(model, apiKey, baseURL), nil
	case "anthropic":
		if baseURL == "" {
			baseURL = defaultAnthropicBaseURL
		}
		return newAnthropicClient(model, apiKey, baseURL), nil
	default:
		return nil, fmt.Errorf("unknown llm provider: %s", provider)
	}
}
