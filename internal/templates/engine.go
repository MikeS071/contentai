package templates

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

type Engine struct {
	contentDir string
}

type ExportReport struct {
	Exported []string
	Skipped  []string
}

func NewEngine(contentDir string) *Engine {
	return &Engine{contentDir: contentDir}
}

func (e *Engine) Get(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}

	localPath := e.localPath(name)
	data, err := os.ReadFile(localPath)
	if err == nil {
		return string(data), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read local template %q: %w", localPath, err)
	}

	data, err = e.readEmbedded(name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (e *Engine) GetWithVars(name string, vars map[string]interface{}) (string, error) {
	raw, err := e.Get(name)
	if err != nil {
		return "", err
	}

	tpl, err := template.New(name).Option("missingkey=error").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", name, err)
	}

	var out bytes.Buffer
	if err := tpl.Execute(&out, vars); err != nil {
		return "", fmt.Errorf("execute template %q: %w", name, err)
	}
	return out.String(), nil
}

func (e *Engine) List() []string {
	entries, err := fs.ReadDir(embeddedTemplates, "templates")
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		names = append(names, strings.TrimSuffix(name, ".md"))
	}
	sort.Strings(names)
	return names
}

func (e *Engine) Export(destDir string) error {
	_, err := e.export(destDir, false)
	return err
}

func (e *Engine) ExportWithForce(destDir string, force bool) (ExportReport, error) {
	return e.export(destDir, force)
}

func (e *Engine) IsOverridden(name string) bool {
	if err := validateName(name); err != nil {
		return false
	}
	_, err := os.Stat(e.localPath(name))
	return err == nil
}

func (e *Engine) export(destDir string, force bool) (ExportReport, error) {
	report := ExportReport{}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return report, fmt.Errorf("create export dir %q: %w", destDir, err)
	}

	for _, name := range e.List() {
		target := filepath.Join(destDir, name+".md")
		if _, err := os.Stat(target); err == nil && !force {
			report.Skipped = append(report.Skipped, name)
			continue
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return report, fmt.Errorf("stat %q: %w", target, err)
		}

		data, err := e.readEmbedded(name)
		if err != nil {
			return report, err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return report, fmt.Errorf("write %q: %w", target, err)
		}
		report.Exported = append(report.Exported, name)
	}

	return report, nil
}

func (e *Engine) localPath(name string) string {
	return filepath.Join(e.contentDir, "templates", name+".md")
}

func (e *Engine) readEmbedded(name string) ([]byte, error) {
	path := filepath.Join("templates", name+".md")
	data, err := embeddedTemplates.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("template %q not found", name)
		}
		return nil, fmt.Errorf("read embedded template %q: %w", name, err)
	}
	return data, nil
}

func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("template name is required")
	}
	if strings.Contains(name, string(filepath.Separator)) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid template name %q", name)
	}
	return nil
}
