package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	var openClaw bool
	var workspace string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install workspace integration assets",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !openClaw {
				return fmt.Errorf("no install target selected (use --openclaw)")
			}

			resolvedWorkspace := strings.TrimSpace(workspace)
			if resolvedWorkspace == "" {
				resolvedWorkspace = filepath.Join(userHomeDir(), ".openclaw", "workspace")
			}
			installer := &openClawInstaller{
				workspace: resolvedWorkspace,
				skillSrc:  filepath.Join("skill"),
				stdout:    cmd.OutOrStdout(),
			}
			return installer.Install()
		},
	}

	cmd.Flags().BoolVar(&openClaw, "openclaw", false, "Install OpenClaw workspace integration")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Override OpenClaw workspace path")
	return cmd
}

type openClawInstaller struct {
	workspace string
	skillSrc  string
	stdout    io.Writer
}

func (i *openClawInstaller) Install() error {
	workspace := strings.TrimSpace(i.workspace)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	dstSkillDir := filepath.Join(workspace, "skills", "contentai")
	if err := copyDir(i.skillSrc, dstSkillDir); err != nil {
		return err
	}

	snippets := []struct {
		src string
		dst string
	}{
		{src: filepath.Join(i.skillSrc, "AGENTS-SNIPPET.md"), dst: filepath.Join(workspace, "AGENTS.md")},
		{src: filepath.Join(i.skillSrc, "TOOLS-SNIPPET.md"), dst: filepath.Join(workspace, "TOOLS.md")},
		{src: filepath.Join(i.skillSrc, "MEMORY-ENTRY.md"), dst: filepath.Join(workspace, "MEMORY.md")},
	}
	for _, s := range snippets {
		if err := appendFileIfMissing(s.dst, s.src); err != nil {
			return err
		}
	}

	if i.stdout != nil {
		fmt.Fprintf(i.stdout, "OpenClaw integration installed in %s\n", workspace)
	}
	return nil
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("read skill source dir (%s): %w", src, err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create skill destination dir (%s): %w", dst, err)
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file (%s): %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination dir (%s): %w", filepath.Dir(dst), err)
	}
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination file (%s): %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return nil
}

func appendFileIfMissing(targetPath, snippetPath string) error {
	snippetBytes, err := os.ReadFile(snippetPath)
	if err != nil {
		return fmt.Errorf("read snippet (%s): %w", snippetPath, err)
	}
	snippet := strings.TrimSpace(string(snippetBytes))
	if snippet == "" {
		return nil
	}

	targetBytes, err := os.ReadFile(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read target (%s): %w", targetPath, err)
	}
	target := string(targetBytes)
	if strings.Contains(target, snippet) {
		return nil
	}

	var b strings.Builder
	trimmedTarget := strings.TrimRight(target, "\n")
	if trimmedTarget != "" {
		b.WriteString(trimmedTarget)
		b.WriteString("\n\n")
	}
	b.WriteString(snippet)
	b.WriteString("\n")

	if err := os.WriteFile(targetPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write target (%s): %w", targetPath, err)
	}
	return nil
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
