package hero

import (
	"hash/fnv"
)

type Palette struct {
	Name        string
	Description string
	Colors      []string
	TitleColor  string
}

var palettes = []Palette{
	{Name: "deep-ocean", Description: "Deep ocean tones using navy and teal with electric cyan accents.", Colors: []string{"#0B1D3A", "#0F4C5C", "#00A5CF"}, TitleColor: "#E9F6FF"},
	{Name: "forest", Description: "Forest composition in layered greens with gold highlights.", Colors: []string{"#0E2A1F", "#1F6A46", "#C9A227"}, TitleColor: "#F5F0D8"},
	{Name: "sunset", Description: "Sunset palette blending orange and purple gradients with warm contrast.", Colors: []string{"#F97316", "#6D28D9", "#F59E0B"}, TitleColor: "#FFF1E8"},
	{Name: "arctic", Description: "Arctic mood with ice blue layers and crisp white highlights.", Colors: []string{"#A5D8FF", "#DFF6FF", "#FFFFFF"}, TitleColor: "#FFFFFF"},
	{Name: "volcanic", Description: "Volcanic atmosphere featuring black forms and red lava accents.", Colors: []string{"#110B0B", "#7F1D1D", "#DC2626"}, TitleColor: "#FFEAEA"},
	{Name: "desert", Description: "Desert tones using sand and terracotta with heat-haze gradients.", Colors: []string{"#D6B38A", "#C96A3D", "#8F4A2A"}, TitleColor: "#FFF2DF"},
	{Name: "nebula", Description: "Nebula-inspired scene with purple clouds and pink luminous streaks.", Colors: []string{"#5B21B6", "#DB2777", "#A855F7"}, TitleColor: "#FFEAFE"},
	{Name: "circuit", Description: "Circuit-style geometry in neon green over dark graphite fields.", Colors: []string{"#0C1A1A", "#22C55E", "#374151"}, TitleColor: "#EFFFF2"},
}

func Palettes() []Palette {
	out := make([]Palette, len(palettes))
	copy(out, palettes)
	return out
}

func PaletteForSlug(slug string) Palette {
	if len(palettes) == 0 {
		return Palette{}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug))
	idx := int(h.Sum32() % uint32(len(palettes)))
	return palettes[idx]
}
