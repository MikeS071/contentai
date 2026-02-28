package publish

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type StaticConfig struct {
	OutputDir string
}

type StaticPublisher struct {
	cfg StaticConfig
}

func NewStaticPublisher(cfg StaticConfig) *StaticPublisher {
	return &StaticPublisher{cfg: cfg}
}

func (p *StaticPublisher) Publish(_ context.Context, item PublishItem) (PublishResult, error) {
	if strings.TrimSpace(item.Slug) == "" {
		return PublishResult{}, fmt.Errorf("slug is required")
	}
	if strings.TrimSpace(p.cfg.OutputDir) == "" {
		return PublishResult{}, fmt.Errorf("static output directory is required")
	}

	dir := filepath.Join(p.cfg.OutputDir, item.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return PublishResult{}, fmt.Errorf("create output dir: %w", err)
	}

	rendered := renderMarkdownHTML(item.Title, item.Content)
	indexPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(indexPath, []byte(rendered), 0o644); err != nil {
		return PublishResult{}, fmt.Errorf("write index.html: %w", err)
	}

	if strings.TrimSpace(item.ImagePath) != "" {
		dst := filepath.Join(dir, filepath.Base(item.ImagePath))
		if err := copyFile(item.ImagePath, dst); err != nil {
			return PublishResult{}, fmt.Errorf("copy image: %w", err)
		}
	}

	return PublishResult{URL: indexPath, ID: item.Slug}, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func renderMarkdownHTML(title, markdown string) string {
	var article bytes.Buffer
	lines := strings.Split(markdown, "\n")
	paragraph := make([]string, 0)
	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		article.WriteString("<p>")
		article.WriteString(html.EscapeString(strings.Join(paragraph, " ")))
		article.WriteString("</p>\n")
		paragraph = paragraph[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flushParagraph()
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			flushParagraph()
			article.WriteString("<h1>")
			article.WriteString(html.EscapeString(strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))))
			article.WriteString("</h1>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			flushParagraph()
			article.WriteString("<h2>")
			article.WriteString(html.EscapeString(strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))))
			article.WriteString("</h2>\n")
			continue
		}
		paragraph = append(paragraph, trimmed)
	}
	flushParagraph()

	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>%s</title>
  <style>
    body { margin: 0; font-family: Georgia, serif; background: #fafafa; color: #1f2937; }
    main { max-width: 760px; margin: 2rem auto; padding: 0 1rem 3rem; line-height: 1.65; }
    h1, h2 { line-height: 1.25; }
  </style>
</head>
<body>
  <main>
%s  </main>
</body>
</html>
`, html.EscapeString(title), article.String())
}
