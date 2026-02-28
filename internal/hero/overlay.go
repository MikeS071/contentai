package hero

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	linkedInWidth  = 1200
	linkedInHeight = 627
)

func OverlayText(src image.Image, title string, palette Palette) (image.Image, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return src, nil
	}

	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	// Dark band increases readability while preserving the generated art.
	overlayHeight := bounds.Dy() / 3
	overlayRect := image.Rect(bounds.Min.X, bounds.Max.Y-overlayHeight, bounds.Max.X, bounds.Max.Y)
	draw.Draw(dst, overlayRect, &image.Uniform{C: color.RGBA{0, 0, 0, 90}}, image.Point{}, draw.Over)

	parsedFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse embedded font: %w", err)
	}
	fontSize := float64(bounds.Dx()) * 0.033
	face, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{Size: fontSize, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, fmt.Errorf("create font face: %w", err)
	}
	defer face.Close()

	textColor := hexToColor(palette.TitleColor)
	d := &font.Drawer{Dst: dst, Face: face, Src: image.NewUniform(textColor)}

	padding := bounds.Dx() / 14
	maxWidth := bounds.Dx() - (padding * 2)
	lines := wrapLines(d, title, maxWidth)

	lineHeight := face.Metrics().Height.Ceil() + (bounds.Dx() / 180)
	startY := bounds.Max.Y - overlayHeight + lineHeight
	for i, line := range lines {
		d.Dot = fixed.P(padding, startY+(i*lineHeight))
		d.DrawString(line)
	}
	return dst, nil
}

func ResizeForLinkedIn(src image.Image) image.Image {
	srcBounds := src.Bounds()
	targetRatio := float64(linkedInWidth) / float64(linkedInHeight)

	cropW := srcBounds.Dx()
	cropH := int(float64(cropW) / targetRatio)
	if cropH > srcBounds.Dy() {
		cropH = srcBounds.Dy()
		cropW = int(float64(cropH) * targetRatio)
	}

	offX := (srcBounds.Dx() - cropW) / 2
	offY := (srcBounds.Dy() - cropH) / 2
	crop := image.Rect(0, 0, cropW, cropH)
	cropped := image.NewRGBA(crop)
	draw.Draw(cropped, crop, src, image.Pt(srcBounds.Min.X+offX, srcBounds.Min.Y+offY), draw.Src)

	out := image.NewRGBA(image.Rect(0, 0, linkedInWidth, linkedInHeight))
	xdraw.CatmullRom.Scale(out, out.Bounds(), cropped, cropped.Bounds(), draw.Over, nil)
	return out
}

func wrapLines(d *font.Drawer, text string, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	lines := make([]string, 0, 3)
	line := words[0]
	for _, w := range words[1:] {
		candidate := line + " " + w
		if d.MeasureString(candidate).Ceil() <= maxWidth {
			line = candidate
			continue
		}
		lines = append(lines, line)
		line = w
	}
	lines = append(lines, line)
	return lines
}

func hexToColor(hex string) color.Color {
	if len(hex) != 7 || hex[0] != '#' {
		return color.White
	}
	var r, g, b uint8
	_, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	if err != nil {
		return color.White
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
