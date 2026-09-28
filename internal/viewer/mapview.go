package viewer

import (
	"github.com/go-gl/gl/v3.3-core/gl"
)

// mapView draws the rendered map image in 2D, with pan and zoom, and the
// map being edited over it.
type mapView struct {
	program *program
	vao     uint32
	image   texture
	camera  mapCamera

	paint        texture // the edited map
	paintOpacity float64 // 0 hides it
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

uniform bool hasImage;
uniform sampler2D image;
uniform vec3 background;
uniform bool hasPaint;
uniform sampler2D paint;
uniform float paintOpacity;

out vec4 fragColor;

void main() {
	vec3 color = hasImage ? texture(image, vUV).rgb : background;
	if (hasPaint)
		color = mix(color, texture(paint, vUV).rgb, paintOpacity);
	fragColor = vec4(color, 1.0);
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

	camera := &v.camera
	hasPaint := v.paint.id != 0 && v.paintOpacity > 0
	if (v.image.id == 0 && !hasPaint) || camera.width == 0 {
		return
	}

	// Pixelated when zoomed in far past the image resolution
	if v.image.id != 0 {
		v.image.setSmooth(camera.zoom*pixelRatio < float64(v.image.width)/camera.width*2)
	}
	if hasPaint {
		v.paint.setSmooth(camera.zoom*pixelRatio < float64(v.paint.width)/camera.width*2)
	}

	x0, y0 := camera.offset.X(), camera.offset.Y()
	x1, y1 := x0+camera.width*camera.zoom, y0+camera.height*camera.zoom
	ndcX := func(x float64) float32 { return float32(2*x/viewWidth - 1) }
	ndcY := func(y float64) float32 { return float32(1 - 2*y/viewHeight) }

	p := v.program
	p.use()
	gl.Uniform4f(p.location("rect"), ndcX(x0), ndcY(y0), ndcX(x1), ndcY(y1))
	p.setInt("hasImage", boolInt(v.image.id != 0))
	p.setInt("image", 0)
	gl.Uniform3f(p.location("background"), mapBackground[0], mapBackground[1], mapBackground[2])
	if v.image.id != 0 {
		v.image.bind(0)
	}
	p.setInt("hasPaint", boolInt(hasPaint))
	p.setInt("paint", 1)
	p.setFloat("paintOpacity", v.paintOpacity)
	if hasPaint {
		v.paint.bind(1)
	}

	gl.BindVertexArray(v.vao)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	gl.BindVertexArray(0)
}
