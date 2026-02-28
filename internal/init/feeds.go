package initflow

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

type FeedsConfig struct {
	Feeds []string `toml:"feeds"`
}

func SaveFeeds(path string, cfg FeedsConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create feeds dir: %w", err)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal feeds toml: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write feeds file: %w", err)
	}
	return nil
}

func LoadFeeds(path string) (FeedsConfig, error) {
	var cfg FeedsConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read feeds file: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse feeds toml: %w", err)
	}
	return cfg, nil
}
