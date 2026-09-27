package viewer

import (
	"github.com/go-gl/gl/v3.3-core/gl"
)

// mapView draws the rendered map image in 2D, with pan and zoom.
type mapView struct {
	program *program
	vao     uint32
	image   texture
	camera  mapCamera
}

// Background around the map
var mapBackground = [3]float32{0x2b / 255.0, 0x2b / 255.0, 0x3a / 255.0}

const mapVertexShader = `
#version 330 core

uniform vec4 rect; // top left and bottom right corners, normalized device coordinates

out vec2 vUV;

void main() {
	vec2 corner = vec2(gl_VertexID & 1, gl_VertexID >> 1);
	vUV = corner;
	gl_Position = vec4(mix(rect.xy, rect.zw, corner), 0.0, 1.0);
}
`

const mapFragmentShader = `
#version 330 core

in vec2 vUV;

uniform sampler2D image;

out vec4 fragColor;

void main() {
	fragColor = vec4(texture(image, vUV).rgb, 1.0);
}
`

func newMapView() (*mapView, error) {
	p, err := newProgram(mapVertexShader, mapFragmentShader)
	if err != nil {
		return nil, err
	}

	v := &mapView{program: p}
	gl.GenVertexArrays(1, &v.vao) // no attributes, but core profile needs one
	return v, nil
}

// setSize sets the map size, and fits the map in the view if it changed.
func (v *mapView) setSize(width, height, viewWidth, viewHeight float64) {
	if width == v.camera.width && height == v.camera.height {
		return
	}
	v.camera.width, v.camera.height = width, height
	v.camera.fit(viewWidth, viewHeight)
}

// draw shows the image in a view of the given size, in window coordinates.
// pixelRatio is the number of framebuffer pixels per window unit.
func (v *mapView) draw(viewWidth, viewHeight, pixelRatio float64) {
	gl.ClearColor(mapBackground[0], mapBackground[1], mapBackground[2], 1)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	c := &v.camera
	if v.image.id == 0 || c.width == 0 {
		return
	}

	// Pixelated when zoomed in far past the image resolution
	smooth := c.zoom*pixelRatio < float64(v.image.width)/c.width*2
	v.image.setSmooth(smooth)

	x0, y0 := c.offset.X(), c.offset.Y()
	x1, y1 := x0+c.width*c.zoom, y0+c.height*c.zoom
	ndcX := func(x float64) float32 { return float32(2*x/viewWidth - 1) }
	ndcY := func(y float64) float32 { return float32(1 - 2*y/viewHeight) }

	v.program.use()
	gl.Uniform4f(v.program.location("rect"), ndcX(x0), ndcY(y0), ndcX(x1), ndcY(y1))
	v.program.setInt("image", 0)
	v.image.bind(0)

	gl.BindVertexArray(v.vao)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	gl.BindVertexArray(0)
}
