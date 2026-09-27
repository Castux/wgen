package viewer

import (
	"math"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

// terrainView draws the mesh in 3D: terrain colors or elevation per vertex,
// flat shading, and the overlay (rivers, contour lines, grid) as a texture.
//
// Lighting and colors reproduce what three.js did in the web viewer: lit is
// its Lambert material with an ambient light of intensity 1 and a directional
// light of intensity 3, unlit its basic material, computed in linear colors
// and displayed in sRGB.
type terrainView struct {
	program       *program
	vao, vbo, ebo uint32
	count         int32 // indices

	width, height   float64 // map size, 0 before the first mesh
	lowest, highest float64

	overlay    texture
	hasOverlay bool

	orbit orbitCamera
	top   topCamera
}

// Background color: three.js took these as linear values.
var skyColor = [3]float64{156.0 / 255, 196.0 / 255, 240.0 / 255}

// Direction toward the light, as in render: from the top left.
var lightDirection = mgl64.Vec3{-1, 1, 1}.Normalize()

const terrainVertexShader = `
#version 330 core

layout(location = 0) in vec3 position;     // map coordinates
layout(location = 1) in vec3 terrainColor; // sRGB

uniform mat4 view;
uniform mat4 projection;
uniform vec2 size;
uniform float lowest;
uniform float span;
uniform bool heightColors;

out vec3 vPosition;
out vec3 vViewPosition;
out vec3 vColor;
out vec2 vUV;

vec3 srgbToLinear(vec3 c) {
	return mix(c / 12.92, pow((c + 0.055) / 1.055, vec3(2.4)), step(0.04045, c));
}

void main() {
	vPosition = position;
	vec4 viewPosition = view * vec4(position.xy - size / 2.0, position.z, 1.0);
	vViewPosition = viewPosition.xyz;

	vColor = heightColors ? vec3((position.z - lowest) / span) : srgbToLinear(terrainColor);

	// The overlay is top row first
	vUV = vec2(position.x / size.x, 1.0 - position.y / size.y);

	gl_Position = projection * viewPosition;
}
`

const terrainFragmentShader = `
#version 330 core

in vec3 vPosition;
in vec3 vViewPosition;
in vec3 vColor;
in vec2 vUV;

uniform vec2 size;
uniform bool lit;
uniform vec3 lightDirection; // view space
uniform bool hasOverlay;
uniform sampler2D overlay;

out vec4 fragColor;

const float PI = 3.141592653589793;

vec3 linearToSrgb(vec3 c) {
	return mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(0.0031308, c));
}

void main() {
	// Hide the margin around the map
	if (any(lessThan(vPosition.xy, vec2(0.0))) || any(greaterThan(vPosition.xy, size)))
		discard;

	vec3 color = vColor;
	if (hasOverlay)
		color *= texture(overlay, vUV).rgb;

	if (lit) {
		vec3 normal = normalize(cross(dFdx(vViewPosition), dFdy(vViewPosition)));
		float diffuse = max(dot(normal, lightDirection), 0.0);
		color *= (1.0 + 3.0 * diffuse) / PI;
	}

	fragColor = vec4(linearToSrgb(clamp(color, 0.0, 1.0)), 1.0);
}
`

type terrainVertex struct {
	x, y, z    float32
	r, g, b, _ uint8
}

func newTerrainView() (*terrainView, error) {
	p, err := newProgram(terrainVertexShader, terrainFragmentShader)
	if err != nil {
		return nil, err
	}

	v := &terrainView{program: p, overlay: texture{srgb: true}}

	gl.GenVertexArrays(1, &v.vao)
	gl.GenBuffers(1, &v.vbo)
	gl.GenBuffers(1, &v.ebo)

	gl.BindVertexArray(v.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, v.vbo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, v.ebo)

	stride := int32(unsafe.Sizeof(terrainVertex{}))
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, stride, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 3, gl.UNSIGNED_BYTE, true, stride, 12)
	gl.BindVertexArray(0)

	return v, nil
}

// setMesh uploads the mesh of a world. It returns whether the map size
// changed (which resets the cameras).
func (v *terrainView) setMesh(w *gen.World) bool {
	m := w.Mesh

	vertices := make([]terrainVertex, len(m.Points))
	for i, p := range m.Points {
		z := w.Z[i]
		if math.IsNaN(z) || math.IsInf(z, 0) {
			z = 0
		}
		c := render.VertexColor(w, int32(i))
		vertices[i] = terrainVertex{x: float32(p.X), y: float32(p.Y), z: float32(z), r: c[0], g: c[1], b: c[2]}
	}

	indices := make([]uint32, 0, 3*len(m.Triangles))
	for _, t := range m.Triangles {
		indices = append(indices, uint32(t[0]), uint32(t[1]), uint32(t[2]))
	}

	gl.BindVertexArray(v.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, v.vbo)
	if len(vertices) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*int(unsafe.Sizeof(terrainVertex{})), gl.Ptr(vertices), gl.STATIC_DRAW)
	}
	if len(indices) > 0 {
		gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Ptr(indices), gl.STATIC_DRAW)
	}
	gl.BindVertexArray(0)
	v.count = int32(len(indices))

	width, height := float64(w.Width), float64(w.Height)
	changed := width != v.width || height != v.height
	v.width, v.height = width, height
	v.lowest, v.highest = w.Lowest, w.Highest

	if changed {
		v.resetCameras()
	}
	return changed
}

func (v *terrainView) resetCameras() {
	if v.width == 0 {
		return
	}
	v.orbit.reset(v.width, v.height)
	v.top.reset(v.width, v.height)
}

// overlayScale is the overlay resolution, relative to the outline image:
// detailed enough for close ups, within GPU limits.
func (v *terrainView) overlayScale() float64 {
	var maxSize int32
	gl.GetIntegerv(gl.MAX_TEXTURE_SIZE, &maxSize)
	extent := math.Max(v.width, v.height)
	return math.Min(4, math.Min(8192, float64(maxSize))/2/extent)
}

func (v *terrainView) draw(s *Settings, aspect float64) {
	gl.ClearColor(float32(srgbEncode(skyColor[0])), float32(srgbEncode(skyColor[1])), float32(srgbEncode(skyColor[2])), 1)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	if v.count == 0 {
		return
	}

	var view, projection mgl64.Mat4
	if s.View == "top" {
		view, projection = v.top.view(), v.top.projection(aspect)
	} else {
		view, projection = v.orbit.view(), v.orbit.projection(aspect)
	}

	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	gl.Enable(gl.CULL_FACE)
	gl.CullFace(gl.BACK)
	gl.FrontFace(gl.CCW)
	if s.Wireframe {
		gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
	}

	p := v.program
	p.use()
	p.setMat4("view", view)
	p.setMat4("projection", projection)
	p.setVec2("size", v.width, v.height)
	p.setFloat("lowest", v.lowest)
	span := v.highest - v.lowest
	if span == 0 {
		span = 1
	}
	p.setFloat("span", span)
	p.setInt("heightColors", boolInt(s.Color == "height"))
	p.setInt("lit", boolInt(s.Shading == "lit"))
	p.setVec3("lightDirection", view.Mat3().Mul3x1(lightDirection).Normalize())
	p.setInt("hasOverlay", boolInt(v.hasOverlay))
	p.setInt("overlay", 0)
	if v.hasOverlay {
		v.overlay.bind(0)
	}

	gl.BindVertexArray(v.vao)
	gl.DrawElementsWithOffset(gl.TRIANGLES, v.count, gl.UNSIGNED_INT, 0)
	gl.BindVertexArray(0)

	gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.DEPTH_TEST)
}

func boolInt(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
