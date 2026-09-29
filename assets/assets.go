// Package assets holds the files built into the app: the icon, and the
// example project.
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

//go:embed example/chasers.json
var exampleProject []byte

//go:embed example/chasers.png
var exampleMap []byte

// ExampleName is the name of the example project.
const ExampleName = "chasers"

// Example is the example project: its file, and its map image.
func Example() (project []byte, mapImage image.Image) {
	img, err := png.Decode(bytes.NewReader(exampleMap))
	if err != nil {
		panic(err)
	}
	return exampleProject, img
}
