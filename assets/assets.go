// Package assets holds the files built into the app.
package assets

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
)

//go:embed icon.png
var iconPNG []byte

// Icon is the app icon, 512x512.
func Icon() image.Image {
	img, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		panic(err)
	}
	return img
}
