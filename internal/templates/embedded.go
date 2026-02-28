package templates

import "embed"

//go:embed templates/*.md
var embeddedTemplates embed.FS
