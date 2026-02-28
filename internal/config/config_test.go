package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "contentai.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, `
[project]
name = "contentai-demo"
content_dir = "articles"
default_publish_gate = false
default_social_gate = false
qa_gate = true

[llm]
provider = "openai"
model = "gpt-4o"
api_key_cmd = "echo llm-key"
base_url = "https://api.example.com"

[llm.draft]
model = "gpt-4o"
temperature = 0.8

[llm.qa]
model = "gpt-4o-mini"
temperature = 0.3

[images]
provider = "openai"
model = "dall-e-3"
api_key_cmd = "echo img-key"
size = "1024x1024"
title_overlay = false

[publish]
type = "http"
url = "https://cms.example.com/publish"
auth_header = "X-API-Key"
auth_cmd = "echo publish-key"
auth_prefix = "Token "
field_map = { title = "title", slug = "slug", content = "body" }

[publish.static]
output_dir = "dist"
format = "html"

[social.x]
enabled = true
api_key_cmd = "echo social-key"

[schedule]
timezone = "Australia/Melbourne"
days = ["Mon", "Tue", "Wed"]
window_start = "10:00"
window_end = "11:00"

[openclaw]
enabled = true
workspace = "~/.openclaw/workspace"
channel_history = true
memory_search = false

[qa]
rules_file = "qa-rules.toml"
auto_fix = false
`)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Project.Name != "contentai-demo" {
		t.Fatalf("Project.Name = %q, want %q", cfg.Project.Name, "contentai-demo")
	}
	if cfg.LLM.Provider != "openai" {
		t.Fatalf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "openai")
	}
	if cfg.LLMDraft == nil || cfg.LLMDraft.Model != "gpt-4o" {
		t.Fatalf("LLMDraft not parsed correctly: %#v", cfg.LLMDraft)
	}
	if cfg.LLMQA == nil || cfg.LLMQA.Model != "gpt-4o-mini" {
		t.Fatalf("LLMQA not parsed correctly: %#v", cfg.LLMQA)
	}
	if cfg.Publish.Static.OutputDir != "dist" || cfg.Publish.Static.Format != "html" {
		t.Fatalf("Publish.Static not parsed correctly: %#v", cfg.Publish.Static)
	}
	platform, ok := cfg.Social["x"]
	if !ok || !platform.Enabled {
		t.Fatalf("Social[x] not parsed correctly: %#v", cfg.Social)
	}
	if cfg.Schedule.Timezone != "Australia/Melbourne" {
		t.Fatalf("Schedule.Timezone = %q, want %q", cfg.Schedule.Timezone, "Australia/Melbourne")
	}
	if cfg.QA.AutoFix {
		t.Fatalf("QA.AutoFix = %v, want false", cfg.QA.AutoFix)
	}
}

func TestLoadMinimalConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, `
[project]
name = "minimal"
`)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Project.Name != "minimal" {
		t.Fatalf("Project.Name = %q, want %q", cfg.Project.Name, "minimal")
	}
	if cfg.Project.ContentDir != "content" {
		t.Fatalf("Project.ContentDir = %q, want %q", cfg.Project.ContentDir, "content")
	}
	if !cfg.Project.DefaultPublishGate || !cfg.Project.DefaultSocialGate || !cfg.Project.QAGate {
		t.Fatalf("project defaults not applied: %#v", cfg.Project)
	}
	if cfg.Images.Size != "1792x1024" || !cfg.Images.TitleOverlay {
		t.Fatalf("images defaults not applied: %#v", cfg.Images)
	}
	if cfg.Publish.AuthHeader != "Authorization" || cfg.Publish.AuthPrefix != "Bearer " {
		t.Fatalf("publish defaults not applied: %#v", cfg.Publish)
	}
	if cfg.Schedule.Timezone != "UTC" {
		t.Fatalf("schedule timezone default = %q, want %q", cfg.Schedule.Timezone, "UTC")
	}
	if len(cfg.Schedule.Days) != 5 {
		t.Fatalf("schedule days default len = %d, want 5", len(cfg.Schedule.Days))
	}
	if !cfg.QA.AutoFix {
		t.Fatalf("qa default auto_fix = %v, want true", cfg.QA.AutoFix)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	cfgPath := writeTempConfig(t, `
[project
name = "bad"
`)

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("Load() error = nil, want file read error")
	}
}

func TestValidateRequiredFields(t *testing.T) {
	cfg := Default()
	cfg.Project.Name = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want required field error")
	}
}

func TestValidateNilConfig(t *testing.T) {
	var cfg *Config
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want nil config error")
	}
}

func TestDefaults(t *testing.T) {
	cfg := Default()

	if cfg.Project.ContentDir != "content" {
		t.Fatalf("Project.ContentDir = %q, want %q", cfg.Project.ContentDir, "content")
	}
	if !cfg.Project.DefaultPublishGate || !cfg.Project.DefaultSocialGate || !cfg.Project.QAGate {
		t.Fatalf("project defaults incorrect: %#v", cfg.Project)
	}
	if cfg.Images.Size != "1792x1024" || !cfg.Images.TitleOverlay {
		t.Fatalf("images defaults incorrect: %#v", cfg.Images)
	}
	if cfg.Publish.AuthHeader != "Authorization" || cfg.Publish.AuthPrefix != "Bearer " {
		t.Fatalf("publish defaults incorrect: %#v", cfg.Publish)
	}
	if cfg.Schedule.Timezone != "UTC" || cfg.Schedule.WindowStart != "09:00" || cfg.Schedule.WindowEnd != "09:30" {
		t.Fatalf("schedule defaults incorrect: %#v", cfg.Schedule)
	}
	if !cfg.QA.AutoFix {
		t.Fatalf("QA.AutoFix = %v, want true", cfg.QA.AutoFix)
	}
}

func TestExecuteKeyCmd(t *testing.T) {
	got, err := ExecuteKeyCmd("echo test-key")
	if err != nil {
		t.Fatalf("ExecuteKeyCmd() error = %v", err)
	}
	if got != "test-key" {
		t.Fatalf("ExecuteKeyCmd() = %q, want %q", got, "test-key")
	}
}

func TestExecuteKeyCmdEmpty(t *testing.T) {
	if _, err := ExecuteKeyCmd("   "); err == nil {
		t.Fatal("ExecuteKeyCmd() error = nil, want empty command error")
	}
}

func TestExecuteKeyCmdFailure(t *testing.T) {
	if _, err := ExecuteKeyCmd("command-that-does-not-exist-12345"); err == nil {
		t.Fatal("ExecuteKeyCmd() error = nil, want command failure")
	}
}

func TestValidateProvider(t *testing.T) {
	cfg := Default()
	cfg.Project.Name = "valid-name"
	cfg.LLM.Provider = "bogus"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid provider error")
	}
}

func TestValidateImagesProvider(t *testing.T) {
	cfg := Default()
	cfg.Project.Name = "valid-name"
	cfg.Images.Provider = "invalid-provider"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid image provider error")
	}
}

func TestValidatePublishType(t *testing.T) {
	cfg := Default()
	cfg.Project.Name = "valid-name"
	cfg.Publish.Type = "ftp"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid publish type error")
	}
}

func TestValidateStaticFormat(t *testing.T) {
	cfg := Default()
	cfg.Project.Name = "valid-name"
	cfg.Publish.Static.Format = "txt"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid static format error")
	}
}

func TestValidateScheduleDays(t *testing.T) {
	cfg := Default()
	cfg.Project.Name = "valid-name"
	cfg.Schedule.Days = []string{"Mon", "Funday"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid day error")
	}
}

func TestValidateScheduleWindow(t *testing.T) {
	cfg := Default()
	cfg.Project.Name = "valid-name"
	cfg.Schedule.WindowStart = "9am"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid time format error")
	}
}
