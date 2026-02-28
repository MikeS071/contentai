package hero

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/templates"
	xdraw "golang.org/x/image/draw"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com"
	defaultImageModel    = "gpt-image-1"
	defaultImageSize     = "1792x1024"
)

type ImageGenerator interface {
	Generate(ctx context.Context, prompt, model, size string) (image.Image, error)
}

type GenerateOptions struct {
	Regenerate bool
	NoTitle    bool
}

type Generator struct {
	ContentDir   string
	ImageGen     ImageGenerator
	Templates    *templates.Engine
	Store        *content.Store
	Model        string
	Size         string
	TitleOverlay bool
	Now          func() time.Time
}

func NewGenerator(contentDir string, imageGen ImageGenerator, tpl *templates.Engine, store *content.Store) *Generator {
	if tpl == nil {
		tpl = templates.NewEngine(contentDir)
	}
	if store == nil {
		store = content.NewStore(contentDir)
	}
	return &Generator{
		ContentDir:   contentDir,
		ImageGen:     imageGen,
		Templates:    tpl,
		Store:        store,
		Model:        defaultImageModel,
		Size:         defaultImageSize,
		TitleOverlay: true,
		Now:          time.Now,
	}
}

func (g *Generator) Generate(ctx context.Context, slug string, opts GenerateOptions) error {
	if g == nil {
		return errors.New("generator is nil")
	}
	if g.ImageGen == nil {
		return errors.New("image generator is required")
	}

	meta, err := g.Store.Get(slug)
	if err != nil {
		return err
	}

	heroPath := g.Store.HeroPath(slug)
	if !opts.Regenerate {
		if _, err := os.Stat(heroPath); err == nil {
			return fmt.Errorf("hero image already exists for %q; use --regenerate", slug)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat hero image: %w", err)
		}
	}

	palette := PaletteForSlug(slug)
	themes := readBlueprintThemes(g.ContentDir)
	prompt, err := g.BuildPrompt(meta.Title, themes, palette)
	if err != nil {
		return err
	}

	base, err := g.ImageGen.Generate(ctx, prompt, g.Model, g.Size)
	if err != nil {
		return err
	}
	base = ensureBaseSize(base)

	if g.TitleOverlay && !opts.NoTitle {
		overlaid, err := OverlayText(base, meta.Title, palette)
		if err != nil {
			return err
		}
		base = overlaid
	}

	if err := writePNG(heroPath, base); err != nil {
		return err
	}
	if err := writePNG(g.Store.HeroLinkedInPath(slug), ResizeForLinkedIn(base)); err != nil {
		return err
	}

	meta.HeroImage = filepath.Base(heroPath)
	meta.HeroLinkedInImage = filepath.Base(g.Store.HeroLinkedInPath(slug))
	now := time.Now().UTC()
	if g.Now != nil {
		now = g.Now().UTC()
	}
	meta.UpdatedAt = now
	return g.Store.UpdateMeta(slug, meta)
}

func (g *Generator) BuildPrompt(title string, themes []string, palette Palette) (string, error) {
	topic := strings.TrimSpace(strings.Join(themes, ", "))
	if topic == "" {
		topic = strings.TrimSpace(title)
	}
	return g.Templates.GetWithVars("hero-prompt", map[string]interface{}{
		"Topic":              topic,
		"Title":              strings.TrimSpace(title),
		"PaletteDescription": palette.Description,
	})
}

func readBlueprintThemes(contentDir string) []string {
	body, err := os.ReadFile(filepath.Join(contentDir, "blueprint.md"))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(body), "\n")
	out := make([]string, 0, 6)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if value == "" {
			continue
		}
		out = append(out, value)
		if len(out) == 6 {
			break
		}
	}
	return out
}

func ensureBaseSize(src image.Image) image.Image {
	const w = 1792
	const h = 1024
	if src.Bounds().Dx() == w && src.Bounds().Dy() == h {
		return src
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(out, out.Bounds(), src, src.Bounds(), draw.Over, nil)
	return out
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create image directory: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	return nil
}

type OpenAIImageGenerator struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewOpenAIImageGenerator(apiKey, baseURL string) *OpenAIImageGenerator {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &OpenAIImageGenerator{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 90 * time.Second},
	}
}

func (g *OpenAIImageGenerator) Generate(ctx context.Context, prompt, model, size string) (image.Image, error) {
	if strings.TrimSpace(g.apiKey) == "" {
		return nil, errors.New("openai image api key is required")
	}
	if strings.TrimSpace(model) == "" {
		model = defaultImageModel
	}
	if strings.TrimSpace(size) == "" {
		size = defaultImageSize
	}

	payload := map[string]any{
		"model":   model,
		"prompt":  prompt,
		"size":    size,
		"n":       1,
		"quality": "high",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal image request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create image request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai image request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read image response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai image request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode image response: %w", err)
	}
	if len(out.Data) == 0 {
		return nil, errors.New("image response had no data")
	}

	if strings.TrimSpace(out.Data[0].B64JSON) != "" {
		decoded, err := base64.StdEncoding.DecodeString(out.Data[0].B64JSON)
		if err != nil {
			return nil, fmt.Errorf("decode base64 image: %w", err)
		}
		img, _, err := image.Decode(bytes.NewReader(decoded))
		if err != nil {
			return nil, fmt.Errorf("decode image data: %w", err)
		}
		return img, nil
	}

	if strings.TrimSpace(out.Data[0].URL) != "" {
		return g.fetchImageURL(ctx, out.Data[0].URL)
	}

	return nil, errors.New("image response missing b64_json and url")
}

func (g *OpenAIImageGenerator) fetchImageURL(ctx context.Context, url string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create image download request: %w", err)
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download image failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode downloaded image: %w", err)
	}
	return img, nil
}
