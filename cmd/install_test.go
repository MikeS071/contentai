package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAppends(t *testing.T) {
	workspace := t.TempDir()
	skillDir := t.TempDir()

	mustWriteInstallFile(t, filepath.Join(skillDir, "SKILL.md"), "name: contentai")
	mustWriteInstallFile(t, filepath.Join(skillDir, "AGENTS-SNIPPET.md"), "## ContentAI Agent Snippet")
	mustWriteInstallFile(t, filepath.Join(skillDir, "TOOLS-SNIPPET.md"), "## ContentAI Tool Snippet")
	mustWriteInstallFile(t, filepath.Join(skillDir, "MEMORY-ENTRY.md"), "## ContentAI Memory Entry")

	installer := &openClawInstaller{
		workspace: workspace,
		skillSrc:  skillDir,
	}
	if err := installer.Install(); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	agents := mustReadInstallFile(t, filepath.Join(workspace, "AGENTS.md"))
	if !strings.Contains(agents, "ContentAI Agent Snippet") {
		t.Fatalf("AGENTS.md missing snippet: %q", agents)
	}
	tools := mustReadInstallFile(t, filepath.Join(workspace, "TOOLS.md"))
	if !strings.Contains(tools, "ContentAI Tool Snippet") {
		t.Fatalf("TOOLS.md missing snippet: %q", tools)
	}
	memory := mustReadInstallFile(t, filepath.Join(workspace, "MEMORY.md"))
	if !strings.Contains(memory, "ContentAI Memory Entry") {
		t.Fatalf("MEMORY.md missing snippet: %q", memory)
	}
	if _, err := os.Stat(filepath.Join(workspace, "skills", "contentai", "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md not copied: %v", err)
	}
}

func TestInstallIdempotent(t *testing.T) {
	workspace := t.TempDir()
	skillDir := t.TempDir()

	mustWriteInstallFile(t, filepath.Join(skillDir, "SKILL.md"), "name: contentai")
	mustWriteInstallFile(t, filepath.Join(skillDir, "AGENTS-SNIPPET.md"), "## ContentAI Agent Snippet")
	mustWriteInstallFile(t, filepath.Join(skillDir, "TOOLS-SNIPPET.md"), "## ContentAI Tool Snippet")
	mustWriteInstallFile(t, filepath.Join(skillDir, "MEMORY-ENTRY.md"), "## ContentAI Memory Entry")

	installer := &openClawInstaller{workspace: workspace, skillSrc: skillDir}
	if err := installer.Install(); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if err := installer.Install(); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}

	agents := mustReadInstallFile(t, filepath.Join(workspace, "AGENTS.md"))
	if strings.Count(agents, "## ContentAI Agent Snippet") != 1 {
		t.Fatalf("AGENTS.md duplicated snippet: %q", agents)
	}
	tools := mustReadInstallFile(t, filepath.Join(workspace, "TOOLS.md"))
	if strings.Count(tools, "## ContentAI Tool Snippet") != 1 {
		t.Fatalf("TOOLS.md duplicated snippet: %q", tools)
	}
	memory := mustReadInstallFile(t, filepath.Join(workspace, "MEMORY.md"))
	if strings.Count(memory, "## ContentAI Memory Entry") != 1 {
		t.Fatalf("MEMORY.md duplicated snippet: %q", memory)
	}
}

func mustWriteInstallFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustReadInstallFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
