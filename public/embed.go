package public

import "embed"

//go:embed *.tmpl
var Templates embed.FS

//go:embed all:static
var Static embed.FS
