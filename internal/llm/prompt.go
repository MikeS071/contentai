package llm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func LoadPrompt(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt (%s): %w", path, err)
	}
	return string(data), nil
}

func RenderPrompt(tmpl string, vars map[string]string) string {
	out := tmpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func LoadAndRenderPrompt(path string, vars map[string]string) (string, error) {
	tmpl, err := LoadPrompt(path)
	if err != nil {
		return "", err
	}
	return RenderPrompt(tmpl, vars), nil
}

var (
	apiKeyMu    sync.Mutex
	apiKeyCache = map[string]string{}
)

func ResolveAPIKey(cmd string) (string, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", fmt.Errorf("api key command is required")
	}

	apiKeyMu.Lock()
	if key, ok := apiKeyCache[cmd]; ok {
		apiKeyMu.Unlock()
		return key, nil
	}
	apiKeyMu.Unlock()

	c := exec.Command("bash", "-lc", cmd)
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("run api key command: %w", err)
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return "", fmt.Errorf("api key command returned empty value")
	}

	apiKeyMu.Lock()
	apiKeyCache[cmd] = key
	apiKeyMu.Unlock()
	return key, nil
}

func clearAPIKeyCache() {
	apiKeyMu.Lock()
	defer apiKeyMu.Unlock()
	apiKeyCache = map[string]string{}
}
