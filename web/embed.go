// Package web embeds the viewer's static files.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// Static holds the viewer files (index.html at the root).
var Static, _ = fs.Sub(files, "static")
