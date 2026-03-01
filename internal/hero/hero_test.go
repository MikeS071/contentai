package hero

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/templates"
)

type mockImageGenerator struct {
	img image.Image
}

func (m *mockImageGenerator) Generate(_ context.Context, _, _, _ string) (image.Image, error) {
	return m.img, nil
}

func solidImage(w, h int, c color.Color) image.Image {
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rgba.Set(x, y, c)
		}
	}
	return rgba
}

func newTestGenerator(t *testing.T) (*Generator, *content.Store, string) {
	t.Helper()
	contentDir := t.TempDir()
	store := content.NewStore(contentDir)
	tpl := templates.NewEngine(contentDir)
	gen := NewGenerator(contentDir, &mockImageGenerator{img: solidImage(1792, 1024, color.RGBA{10, 20, 30, 255})}, tpl, store)
	return gen, store, contentDir
}

func TestPaletteRotation(t *testing.T) {
	a := PaletteForSlug("same-slug")
	b := PaletteForSlug("same-slug")
	if a.Name != b.Name {
		t.Fatalf("same slug should map to same palette: %q != %q", a.Name, b.Name)
	}
}

func TestPaletteDistribution(t *testing.T) {
	seen := map[string]bool{}
	slugs := []string{"alpha-post", "beta-post", "gamma-post", "delta-post", "epsilon-post", "zeta-post", "eta-post", "theta-post", "iota-post", "kappa-post", "lambda-post", "mu-post"}
	for _, slug := range slugs {
		seen[PaletteForSlug(slug).Name] = true
	}
	if len(seen) < 4 {
		t.Fatalf("expected a spread across palettes, got %d unique", len(seen))
	}
}

func TestPalettes(t *testing.T) {
	if len(Palettes()) != 8 {
		t.Fatalf("expected 8 palettes")
	}
}

func TestBuildPrompt(t *testing.T) {
	gen, _, _ := newTestGenerator(t)
	palette := PaletteForSlug("prompt-slug")
	prompt, err := gen.BuildPrompt("My Test Title", []string{"strategy", "systems"}, palette)
	if err != nil {
		t.Fatalf("BuildPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "strategy") {
		t.Fatalf("prompt missing title: %s", prompt)
	}
	if !strings.Contains(prompt, palette.Description) {
		t.Fatalf("prompt missing palette description: %s", prompt)
	}
	if !strings.Contains(prompt, "strategy") || !strings.Contains(prompt, "systems") {
		t.Fatalf("prompt missing themes: %s", prompt)
	}
}

func TestBuildPromptFallbackTopic(t *testing.T) {
	gen, _, _ := newTestGenerator(t)
	palette := PaletteForSlug("prompt-fallback")
	prompt, err := gen.BuildPrompt("Title Fallback", nil, palette)
	if err != nil {
		t.Fatalf("BuildPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Title Fallback") {
		t.Fatalf("expected title fallback in topic: %s", prompt)
	}
}

func TestOverlayText(t *testing.T) {
	src := solidImage(1792, 1024, color.Black)
	out, err := OverlayText(src, "Overlay Title", PaletteForSlug("overlay"))
	if err != nil {
		t.Fatalf("OverlayText() error = %v", err)
	}
	if out.Bounds().Dx() != 1792 || out.Bounds().Dy() != 1024 {
		t.Fatalf("unexpected output dimensions: %dx%d", out.Bounds().Dx(), out.Bounds().Dy())
	}

	rgba := image.NewRGBA(out.Bounds())
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			rgba.Set(x, y, out.At(x, y))
		}
	}

	nonBlack := false
	for y := 0; y < rgba.Bounds().Dy() && !nonBlack; y++ {
		for x := 0; x < rgba.Bounds().Dx(); x++ {
			r, g, b, _ := rgba.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 {
				nonBlack = true
				break
			}
		}
	}
	if !nonBlack {
		t.Fatalf("expected overlay to modify pixels")
	}
}

func TestLinkedInResize(t *testing.T) {
	src := solidImage(1792, 1024, color.White)
	out := ResizeForLinkedIn(src)
	if out.Bounds().Dx() != 1200 || out.Bounds().Dy() != 627 {
		t.Fatalf("unexpected linkedIn dimensions: %dx%d", out.Bounds().Dx(), out.Bounds().Dy())
	}
}

func TestSavesImages(t *testing.T) {
	gen, store, _ := newTestGenerator(t)
	if err := store.Create("hero-slug", "Hero Title"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.ContentDir, "blueprint.md"), []byte("- theme one\n- theme two\n"), 0o644); err != nil {
		t.Fatalf("write blueprint: %v", err)
	}

	if err := gen.Generate(context.Background(), "hero-slug", GenerateOptions{}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if _, err := os.Stat(store.HeroPath("hero-slug")); err != nil {
		t.Fatalf("hero.png should exist: %v", err)
	}
	if _, err := os.Stat(store.HeroLinkedInPath("hero-slug")); err != nil {
		t.Fatalf("hero-linkedin.png should exist: %v", err)
	}
}

func TestMetaUpdated(t *testing.T) {
	gen, store, _ := newTestGenerator(t)
	if err := store.Create("meta-slug", "Meta Hero"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.ContentDir, "blueprint.md"), []byte("- one\n"), 0o644); err != nil {
		t.Fatalf("write blueprint: %v", err)
	}

	if err := gen.Generate(context.Background(), "meta-slug", GenerateOptions{}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	meta, err := store.Get("meta-slug")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if meta.HeroImage == "" || meta.HeroLinkedInImage == "" {
		t.Fatalf("expected hero paths in meta, got %#v", meta)
	}
}

func TestRegenerate(t *testing.T) {
	gen, store, _ := newTestGenerator(t)
	if err := store.Create("regen-slug", "Regenerate Hero"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.ContentDir, "blueprint.md"), []byte("- one\n"), 0o644); err != nil {
		t.Fatalf("write blueprint: %v", err)
	}

	if err := gen.Generate(context.Background(), "regen-slug", GenerateOptions{NoTitle: true}); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	first, err := os.ReadFile(store.HeroPath("regen-slug"))
	if err != nil {
		t.Fatalf("read first image: %v", err)
	}

	gen.ImageGen = &mockImageGenerator{img: solidImage(1792, 1024, color.RGBA{220, 20, 20, 255})}
	if err := gen.Generate(context.Background(), "regen-slug", GenerateOptions{Regenerate: true, NoTitle: true}); err != nil {
		t.Fatalf("regenerate Generate() error = %v", err)
	}
	second, err := os.ReadFile(store.HeroPath("regen-slug"))
	if err != nil {
		t.Fatalf("read second image: %v", err)
	}

	if bytes.Equal(first, second) {
		t.Fatalf("expected regenerated image to overwrite existing file")
	}
}

func TestGenerateRejectsWhenImageExistsWithoutRegenerate(t *testing.T) {
	gen, store, _ := newTestGenerator(t)
	if err := store.Create("regen-block", "Regenerate Hero"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.ContentDir, "blueprint.md"), []byte("- one\n"), 0o644); err != nil {
		t.Fatalf("write blueprint: %v", err)
	}

	if err := gen.Generate(context.Background(), "regen-block", GenerateOptions{NoTitle: true}); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	err := gen.Generate(context.Background(), "regen-block", GenerateOptions{NoTitle: true})
	if err == nil || !strings.Contains(err.Error(), "--regenerate") {
		t.Fatalf("expected regenerate guidance error, got %v", err)
	}
}

func TestOpenAIImageGeneratorB64(t *testing.T) {
	raw := image.NewRGBA(image.Rect(0, 0, 100, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 100; x++ {
			raw.Set(x, y, color.RGBA{100, 120, 140, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, raw); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["model"] != "gpt-image-1" {
			t.Fatalf("expected model in payload")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + b64 + `"}]}`))
	}))
	defer srv.Close()

	gen := NewOpenAIImageGenerator("test-key", srv.URL)
	img, err := gen.Generate(context.Background(), "prompt", "gpt-image-1", "1792x1024")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 50 {
		t.Fatalf("decoded dimensions mismatch: %v", img.Bounds())
	}
}

func TestOpenAIImageGeneratorURLFallback(t *testing.T) {
	raw := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			raw.Set(x, y, color.RGBA{120, 30, 200, 255})
		}
	}
	var imgBuf bytes.Buffer
	if err := png.Encode(&imgBuf, raw); err != nil {
		t.Fatalf("encode test png: %v", err)
	}

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"url":"` + srv.URL + `/image.png"}]}`))
		case "/image.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imgBuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	gen := NewOpenAIImageGenerator("test-key", srv.URL)
	img, err := gen.Generate(context.Background(), "prompt", "gpt-image-1", "1792x1024")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if img.Bounds().Dx() != 80 || img.Bounds().Dy() != 40 {
		t.Fatalf("decoded dimensions mismatch: %v", img.Bounds())
	}
}

func TestOpenAIImageGeneratorErrors(t *testing.T) {
	gen := NewOpenAIImageGenerator("", "http://invalid")
	if _, err := gen.Generate(context.Background(), "p", "", ""); err == nil {
		t.Fatalf("expected api key error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	gen = NewOpenAIImageGenerator("test", srv.URL)
	if _, err := gen.Generate(context.Background(), "p", "", ""); err == nil {
		t.Fatalf("expected request failure")
	}
}
