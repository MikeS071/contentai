package hero

import (
	"hash/fnv"
)

type Palette struct {
	Name        string
	Description string
	Colors      []string
	TitleColor  string
	Mood        string
}

var palettes = []Palette{
	{Name: "deep-ocean", Description: "Deep ocean blues and electric cyan accents on dark navy", Colors: []string{"#0B1D3A", "#0F4C5C", "#00A5CF"}, TitleColor: "#E9F6FF", Mood: "flowing, liquid, data-stream aesthetic with circuit-like patterns"},
	{Name: "forge", Description: "Warm amber, burnt orange, and deep charcoal", Colors: []string{"#F97316", "#C96A3D", "#374151"}, TitleColor: "#FFF1E8", Mood: "industrial, forge-like, with glowing embers and metallic textures"},
	{Name: "forest", Description: "Deep forest greens and charcoal with emerald highlights", Colors: []string{"#0E2A1F", "#1F6A46", "#C9A227"}, TitleColor: "#F5F0D8", Mood: "organic, architectural, with geometric shapes and subtle glow"},
	{Name: "nebula", Description: "Rich purple, magenta, and deep indigo on near-black", Colors: []string{"#5B21B6", "#DB2777", "#A855F7"}, TitleColor: "#FFEAFE", Mood: "cosmic, nebula-like, with stardust particles and energy fields"},
	{Name: "steampunk", Description: "Teal, copper, and dark slate with golden accents", Colors: []string{"#0F4C5C", "#C96A3D", "#C9A227"}, TitleColor: "#FFF2DF", Mood: "steampunk-meets-modern, with mechanical precision and warm highlights"},
	{Name: "volcanic", Description: "Crimson red, dark rose, and charcoal with white accents", Colors: []string{"#110B0B", "#7F1D1D", "#DC2626"}, TitleColor: "#FFEAEA", Mood: "bold, dramatic, with sharp angular forms and neon edge lighting"},
	{Name: "arctic", Description: "Silver, ice blue, and pearl white on dark graphite", Colors: []string{"#A5D8FF", "#DFF6FF", "#374151"}, TitleColor: "#FFFFFF", Mood: "crystalline, frozen, with sharp faceted surfaces and cool light refractions"},
	{Name: "circuit", Description: "Lime green, electric yellow, and deep black", Colors: []string{"#0C1A1A", "#22C55E", "#F59E0B"}, TitleColor: "#EFFFF2", Mood: "hacker-terminal aesthetic, with matrix-like energy and digital rain hints"},
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
