package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/contentai/internal/content"
)

func RunNew(args []string, stdout io.Writer, stderr io.Writer, store *content.Store) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(stderr)

	title := fs.String("title", "", "content title")
	fromIdea := fs.Int("from-idea", 0, "seed article from idea index in latest batch")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("usage: contentai new <slug> [--title \"...\"] [--from-idea N]")
	}

	slug := fs.Arg(0)
	if *title == "" {
		*title = slug
	}

	if err := store.Create(slug, *title); err != nil {
		return err
	}

	if *fromIdea > 0 {
		outline, err := loadOutlineFromLatestBatch(store.ContentDir, *fromIdea)
		if err != nil {
			return err
		}
		if err := store.WriteArticle(slug, outline); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(stdout, "Created content item: %s\n", store.SlugDir(slug))
	return err
}

func loadOutlineFromLatestBatch(contentDir string, index int) (string, error) {
	ideasDir := filepath.Join(contentDir, "ideas")
	entries, err := os.ReadDir(ideasDir)
	if err != nil {
		return "", fmt.Errorf("read ideas dir: %w", err)
	}
	if len(entries) == 0 {
		return "", errors.New("no idea batches found")
	}

	type candidate struct {
		path string
		mod  int64
	}
	files := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		files = append(files, candidate{
			path: filepath.Join(ideasDir, entry.Name()),
			mod:  info.ModTime().UnixNano(),
		})
	}
	if len(files) == 0 {
		return "", errors.New("no idea batch files found")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod > files[j].mod })

	contentBytes, err := os.ReadFile(files[0].path)
	if err != nil {
		return "", fmt.Errorf("read latest idea batch: %w", err)
	}
	sections := splitMarkdownSections(string(contentBytes))
	if index < 1 || index > len(sections) {
		return "", fmt.Errorf("outline index %d out of range (1-%d)", index, len(sections))
	}
	return sections[index-1], nil
}

func splitMarkdownSections(doc string) []string {
	lines := strings.Split(doc, "\n")
	sections := make([]string, 0)
	var current []string
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if len(current) > 0 {
				sections = append(sections, strings.TrimSpace(strings.Join(current, "\n")))
			}
			current = []string{line}
			continue
		}
		if len(current) > 0 {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		sections = append(sections, strings.TrimSpace(strings.Join(current, "\n")))
	}
	return sections
}
