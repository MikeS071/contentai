package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

var (
	validLLMProviders    = map[string]struct{}{"openai": {}, "anthropic": {}, "custom": {}}
	validImageProviders  = map[string]struct{}{"openai": {}, "stability": {}}
	validPublishTypes    = map[string]struct{}{"http": {}, "static": {}}
	validStaticFormats   = map[string]struct{}{"markdown": {}, "html": {}}
	validScheduleDays    = map[string]struct{}{"Mon": {}, "Tue": {}, "Wed": {}, "Thu": {}, "Fri": {}, "Sat": {}, "Sun": {}}
	defaultScheduleDays  = []string{"Mon", "Tue", "Wed", "Thu", "Fri"}
	defaultWindowStart   = "09:00"
	defaultWindowEnd     = "09:30"
	defaultScheduleTZ    = "UTC"
	defaultContentDir    = "content"
	defaultImageSize     = "1792x1024"
	defaultAuthHeader    = "Authorization"
	defaultAuthPrefix    = "Bearer "
	defaultPublishFormat = "markdown"
)

type Config struct {
	Project  ProjectConfig `toml:"project"`
	LLM      LLMConfig     `toml:"llm"`
	LLMDraft *LLMOverride
	LLMQA    *LLMOverride
	Images   ImagesConfig   `toml:"images"`
	Publish  PublishConfig  `toml:"publish"`
	Social   SocialConfig   `toml:"social"`
	Schedule ScheduleConfig `toml:"schedule"`
	OpenClaw OpenClawConfig `toml:"openclaw"`
	QA       QAConfig       `toml:"qa"`
}

type ProjectConfig struct {
	Name               string `toml:"name"`
	ContentDir         string `toml:"content_dir"`
	DefaultPublishGate bool   `toml:"default_publish_gate"`
	DefaultSocialGate  bool   `toml:"default_social_gate"`
	QAGate             bool   `toml:"qa_gate"`
}

type LLMConfig struct {
	Provider  string `toml:"provider"`
	Model     string `toml:"model"`
	APIKeyCmd string `toml:"api_key_cmd"`
	BaseURL   string `toml:"base_url"`
}

type LLMOverride struct {
	Model       string  `toml:"model"`
	Temperature float64 `toml:"temperature"`
}

type ImagesConfig struct {
	Provider     string `toml:"provider"`
	Model        string `toml:"model"`
	APIKeyCmd    string `toml:"api_key_cmd"`
	Size         string `toml:"size"`
	TitleOverlay bool   `toml:"title_overlay"`
}

type PublishConfig struct {
	Type            string              `toml:"type"`
	URL             string              `toml:"url"`
	AuthHeader      string              `toml:"auth_header"`
	APIKeyCmd       string              `toml:"api_key_cmd"`
	AuthCmd         string              `toml:"auth_cmd"`
	AuthPrefix      string              `toml:"auth_prefix"`
	FieldMap        map[string]string   `toml:"field_map"`
	ResponseURLPath string              `toml:"response_url_path"`
	Static          StaticPublishConfig `toml:"static"`
}

type StaticPublishConfig struct {
	OutputDir string `toml:"output_dir"`
	Format    string `toml:"format"`
}

type SocialConfig map[string]SocialPlatformConfig

type SocialPlatformConfig struct {
	Enabled         bool   `toml:"enabled"`
	APIKeyCmd       string `toml:"api_key_cmd"`
	APISecretCmd    string `toml:"api_secret_cmd"`
	AccessTokenCmd  string `toml:"access_token_cmd"`
	AccessSecretCmd string `toml:"access_secret_cmd"`
	AuthorURN       string `toml:"author_urn"`
	Handle          string `toml:"handle"`
	PasswordCmd     string `toml:"password_cmd"`
}

type ScheduleConfig struct {
	Timezone    string   `toml:"timezone"`
	Days        []string `toml:"days"`
	WindowStart string   `toml:"window_start"`
	WindowEnd   string   `toml:"window_end"`
}

type OpenClawConfig struct {
	Enabled        bool   `toml:"enabled"`
	Workspace      string `toml:"workspace"`
	ChannelHistory bool   `toml:"channel_history"`
	MemorySearch   bool   `toml:"memory_search"`
}

type QAConfig struct {
	RulesFile string `toml:"rules_file"`
	AutoFix   bool   `toml:"auto_fix"`
}

type fileConfig struct {
	Project  ProjectConfig  `toml:"project"`
	LLM      fileLLMConfig  `toml:"llm"`
	Images   ImagesConfig   `toml:"images"`
	Publish  PublishConfig  `toml:"publish"`
	Social   SocialConfig   `toml:"social"`
	Schedule ScheduleConfig `toml:"schedule"`
	OpenClaw OpenClawConfig `toml:"openclaw"`
	QA       QAConfig       `toml:"qa"`
}

type fileLLMConfig struct {
	Provider  string       `toml:"provider"`
	Model     string       `toml:"model"`
	APIKeyCmd string       `toml:"api_key_cmd"`
	BaseURL   string       `toml:"base_url"`
	Draft     *LLMOverride `toml:"draft"`
	QA        *LLMOverride `toml:"qa"`
}

func Default() *Config {
	return &Config{
		Project: ProjectConfig{
			ContentDir:         defaultContentDir,
			DefaultPublishGate: true,
			DefaultSocialGate:  true,
			QAGate:             true,
		},
		Images: ImagesConfig{
			Size:         defaultImageSize,
			TitleOverlay: true,
		},
		Publish: PublishConfig{
			AuthHeader: defaultAuthHeader,
			AuthPrefix: defaultAuthPrefix,
			Static: StaticPublishConfig{
				Format: defaultPublishFormat,
			},
		},
		Social: make(SocialConfig),
		Schedule: ScheduleConfig{
			Timezone:    defaultScheduleTZ,
			Days:        append([]string(nil), defaultScheduleDays...),
			WindowStart: defaultWindowStart,
			WindowEnd:   defaultWindowEnd,
		},
		QA: QAConfig{AutoFix: true},
	}
}

func Load(path string) (*Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	parsed := defaultFileConfig()
	if err := toml.Unmarshal(contents, &parsed); err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}

	cfg := &Config{
		Project: parsed.Project,
		LLM: LLMConfig{
			Provider:  parsed.LLM.Provider,
			Model:     parsed.LLM.Model,
			APIKeyCmd: parsed.LLM.APIKeyCmd,
			BaseURL:   parsed.LLM.BaseURL,
		},
		LLMDraft: parsed.LLM.Draft,
		LLMQA:    parsed.LLM.QA,
		Images:   parsed.Images,
		Publish:  parsed.Publish,
		Social:   parsed.Social,
		Schedule: parsed.Schedule,
		OpenClaw: parsed.OpenClaw,
		QA:       parsed.QA,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}

	if strings.TrimSpace(c.Project.Name) == "" {
		return errors.New("project.name is required")
	}

	if c.LLM.Provider != "" {
		if _, ok := validLLMProviders[c.LLM.Provider]; !ok {
			return fmt.Errorf("llm.provider must be one of: openai, anthropic, custom")
		}
	}

	if c.Images.Provider != "" {
		if _, ok := validImageProviders[c.Images.Provider]; !ok {
			return fmt.Errorf("images.provider must be one of: openai, stability")
		}
	}

	if c.Publish.Type != "" {
		if _, ok := validPublishTypes[c.Publish.Type]; !ok {
			return fmt.Errorf("publish.type must be one of: http, static")
		}
	}

	if c.Publish.Static.Format != "" {
		if _, ok := validStaticFormats[c.Publish.Static.Format]; !ok {
			return fmt.Errorf("publish.static.format must be one of: markdown, html")
		}
	}

	if err := validateSchedule(c.Schedule); err != nil {
		return err
	}

	return nil
}

func ExecuteKeyCmd(cmd string) (string, error) {
	trimmedCmd := strings.TrimSpace(cmd)
	if trimmedCmd == "" {
		return "", errors.New("command cannot be empty")
	}

	output, err := exec.Command("sh", "-c", trimmedCmd).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("execute command %q: %w", trimmedCmd, err)
	}

	return strings.TrimSpace(string(output)), nil
}

func defaultFileConfig() fileConfig {
	defaults := Default()
	return fileConfig{
		Project: defaults.Project,
		LLM: fileLLMConfig{
			Provider:  defaults.LLM.Provider,
			Model:     defaults.LLM.Model,
			APIKeyCmd: defaults.LLM.APIKeyCmd,
			BaseURL:   defaults.LLM.BaseURL,
			Draft:     defaults.LLMDraft,
			QA:        defaults.LLMQA,
		},
		Images:   defaults.Images,
		Publish:  defaults.Publish,
		Social:   defaults.Social,
		Schedule: defaults.Schedule,
		OpenClaw: defaults.OpenClaw,
		QA:       defaults.QA,
	}
}

func validateSchedule(schedule ScheduleConfig) error {
	for _, day := range schedule.Days {
		if _, ok := validScheduleDays[day]; !ok {
			return fmt.Errorf("schedule.days contains invalid day %q", day)
		}
	}

	if schedule.WindowStart != "" {
		if _, err := time.Parse("15:04", schedule.WindowStart); err != nil {
			return fmt.Errorf("schedule.window_start must be HH:MM")
		}
	}
	if schedule.WindowEnd != "" {
		if _, err := time.Parse("15:04", schedule.WindowEnd); err != nil {
			return fmt.Errorf("schedule.window_end must be HH:MM")
		}
	}

	return nil
}
