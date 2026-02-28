package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewClientOpenAI(t *testing.T) {
	c, err := NewClient("openai", "gpt-4o", "key", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Name() != "openai" {
		t.Fatalf("unexpected client name: %s", c.Name())
	}
}

func TestNewClientAnthropic(t *testing.T) {
	c, err := NewClient("anthropic", "claude-3-5-sonnet-latest", "key", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Name() != "anthropic" {
		t.Fatalf("unexpected client name: %s", c.Name())
	}
}

func TestNewClientInvalid(t *testing.T) {
	_, err := NewClient("nope", "model", "key", "")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestOpenAIComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"gpt-4o"`) {
			t.Fatalf("expected model in request body, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"abc",
			"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"hello world"}}],
			"usage":{"prompt_tokens":11,"completion_tokens":7}
		}`))
	}))
	defer server.Close()

	c, err := NewClient("openai", "gpt-4o", "test-key", server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := c.Complete(context.Background(), Request{
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Temperature: 0.2,
		MaxTokens:   100,
	})
	if err != nil {
		t.Fatalf("complete error: %v", err)
	}
	if resp.Content != "hello world" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if resp.TokensIn != 11 || resp.TokensOut != 7 {
		t.Fatalf("unexpected tokens in/out: %+v", resp)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("unexpected finish reason: %q", resp.FinishReason)
	}
}

func TestOpenAIStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	c, err := NewClient("openai", "gpt-4o", "test-key", server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ch, err := c.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var parts []string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream chunk error: %v", chunk.Error)
		}
		if chunk.Content != "" {
			parts = append(parts, chunk.Content)
		}
	}

	if got := strings.Join(parts, ""); got != "hello" {
		t.Fatalf("unexpected stream content: %q", got)
	}
}

func TestOpenAIRateLimitRetry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer server.Close()

	c, err := NewClient("openai", "gpt-4o", "test-key", server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := c.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("complete error: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestAnthropicRateLimitRetry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"type":"message",
			"stop_reason":"end_turn",
			"usage":{"input_tokens":2,"output_tokens":1},
			"content":[{"type":"text","text":"ok"}]
		}`))
	}))
	defer server.Close()

	c, err := NewClient("anthropic", "claude-3-5-sonnet-latest", "test-key", server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	resp, err := c.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 10})
	if err != nil {
		t.Fatalf("complete error: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestAnthropicComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("unexpected key header: %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			t.Fatal("expected anthropic-version header")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"system":"you are helpful"`) {
			t.Fatalf("expected system prompt in request body, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"type":"message",
			"stop_reason":"end_turn",
			"usage":{"input_tokens":5,"output_tokens":3},
			"content":[{"type":"text","text":"anthropic says hi"}]
		}`))
	}))
	defer server.Close()

	c, err := NewClient("anthropic", "claude-3-5-sonnet-latest", "test-key", server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := c.Complete(context.Background(), Request{Messages: []Message{{Role: "system", Content: "you are helpful"}, {Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("complete error: %v", err)
	}
	if resp.Content != "anthropic says hi" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if resp.TokensIn != 5 || resp.TokensOut != 3 {
		t.Fatalf("unexpected token usage: %+v", resp)
	}
	if resp.FinishReason != "end_turn" {
		t.Fatalf("unexpected finish reason: %s", resp.FinishReason)
	}
}

func TestOpenAIStreamHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()

	c, err := NewClient("openai", "gpt-4o", "test-key", server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = c.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "auth error") {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestAnthropicStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"anth\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"ropic\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	c, err := NewClient("anthropic", "claude-3-5-sonnet-latest", "test-key", server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ch, err := c.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 10})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var got []string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream chunk error: %v", chunk.Error)
		}
		if chunk.Content != "" {
			got = append(got, chunk.Content)
		}
	}
	if strings.Join(got, "") != "anthropic" {
		t.Fatalf("unexpected stream output: %q", strings.Join(got, ""))
	}
}

func TestOpenAICompleteAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()

	c, err := NewClient("openai", "gpt-4o", "test-key", server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = c.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "auth error") {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestAssembleContext(t *testing.T) {
	dir := t.TempDir()
	contentDir := filepath.Join(dir, "content")
	if err := os.MkdirAll(filepath.Join(contentDir, "examples"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWrite(t, filepath.Join(contentDir, "voice.md"), "voice")
	mustWrite(t, filepath.Join(contentDir, "blueprint.md"), "# Blueprint\n## Core Ideas\n- keep it tight")
	mustWrite(t, filepath.Join(contentDir, "examples", "a.md"), "example a")
	mustWrite(t, filepath.Join(contentDir, "examples", "b.md"), "example b")

	cc, err := AssembleContext(contentDir, WithSource("source"), WithConversation("history"), WithCustomRules("rules"))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if cc.VoiceProfile != "voice" {
		t.Fatalf("unexpected voice: %q", cc.VoiceProfile)
	}
	if !strings.Contains(cc.Blueprint, "Core Ideas") {
		t.Fatalf("expected blueprint loaded")
	}
	if len(cc.Examples) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(cc.Examples))
	}
	if cc.Source != "source" || cc.Conversation != "history" || cc.CustomRules != "rules" {
		t.Fatalf("unexpected optional fields: %+v", cc)
	}
}

func TestAssembleContextMissingVoice(t *testing.T) {
	dir := t.TempDir()
	contentDir := filepath.Join(dir, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWrite(t, filepath.Join(contentDir, "blueprint.md"), "bp")

	_, err := AssembleContext(contentDir)
	if err == nil {
		t.Fatal("expected missing voice error")
	}
}

func TestBuildMessages(t *testing.T) {
	cc := &ContentContext{
		VoiceProfile: "voice profile",
		Blueprint:    "bp core",
		Examples:     []string{"ex1", "ex2"},
		Source:       "source",
	}

	msgs := cc.BuildMessages("write a draft", 10_000)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "voice profile") || !strings.Contains(msgs[0].Content, "bp core") {
		t.Fatalf("unexpected system message: %+v", msgs[0])
	}
	if msgs[1].Role != "user" || !strings.Contains(msgs[1].Content, "write a draft") || !strings.Contains(msgs[1].Content, "source") {
		t.Fatalf("unexpected user message: %+v", msgs[1])
	}
	if !strings.Contains(msgs[1].Content, "ex1") || !strings.Contains(msgs[1].Content, "ex2") {
		t.Fatalf("expected examples in user message")
	}
}

func TestBuildMessagesTruncation(t *testing.T) {
	cc := &ContentContext{
		VoiceProfile: strings.Repeat("v", 60),
		Blueprint:    strings.Repeat("b", 60),
		Examples: []string{
			"oldest-" + strings.Repeat("1", 80),
			"middle-" + strings.Repeat("2", 80),
			"newest-" + strings.Repeat("3", 80),
		},
		Conversation: strings.Repeat("c", 160),
		Source:       "src",
	}

	msgs := cc.BuildMessages("task", 70)
	joined := msgs[0].Content + msgs[1].Content
	if !strings.Contains(joined, strings.Repeat("v", 60)) {
		t.Fatal("voice profile must be preserved")
	}
	if strings.Contains(joined, "oldest-") {
		t.Fatal("oldest example should be dropped first")
	}
	if strings.Contains(joined, strings.Repeat("c", 160)) {
		t.Fatal("conversation should be truncated when needed")
	}
}
