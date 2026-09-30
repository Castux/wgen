package app

import (
	"math"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

// terrainView draws the mesh in 3D: terrain colors or elevation per vertex,
// flat shading, and the overlay (rivers, contour lines, grid) as a texture
// drawn over, unlit.
//
// Lighting and colors are those of three.js: lit is its Lambert material
// with an ambient light of intensity 1 and a directional light of intensity
// 3, unlit its basic material, computed in linear colors and displayed in
// sRGB.
type terrainView struct {
	program *program
	mesh    meshBuffers
	details []groundLayer // finer ground around the eye, from the coarsest

	width, height   float64 // map size, 0 before the first mesh
	lowest, highest float64
	metersPerPixel  float64 // elevation units per map unit

	overlay    texture
	hasOverlay bool

	// The painted map over the terrain, while painting (the map view's)
	paint        *texture
	paintOpacity float64

	orbit orbitCamera
	eye   eyeCamera
}

// Background color, as linear values (as three.js takes them).
var skyColor = [3]float64{156.0 / 255, 196.0 / 255, 240.0 / 255}

// Direction toward the light, as in render: from the top left.
var lightDirection = mgl64.Vec3{-1, 1, 1}.Normalize()

const terrainVertexShader = `
#version 330 core

layout(location = 0) in vec3 position;     // map coordinates
layout(location = 1) in vec4 terrainColor; // sRGB; alpha: see terrainVertex

uniform mat4 view;
uniform mat4 projection;
uniform vec2 size;
uniform float lowest;
uniform bool heightColors;
uniform bool singleColor;
uniform vec3 landColor; // sRGB, of the single color mode
uniform bool rainbow;
uniform float highest;
uniform float zScale; // elevation to map units

const int waterBit = 128; // as in Go

out vec3 vPosition;
out vec3 vViewPosition;
out vec3 vColor;
out vec2 vUV;
out float vDepth; // distance along the view, for the logarithmic depth

vec3 srgbToLinear(vec3 c) {
	return mix(c / 12.92, pow((c + 0.055) / 1.055, vec3(2.4)), step(0.04045, c));
}

// Turbo, Google's improved rainbow colormap (polynomial approximation), sRGB
vec3 turbo(float t) {
	const vec4 r4 = vec4(0.13572138, 4.61539260, -42.66032258, 132.13108234);
	const vec4 g4 = vec4(0.09140261, 2.19418839, 4.84296658, -14.18503333);
	const vec4 b4 = vec4(0.10667330, 12.64194608, -60.58204836, 110.36276771);
	const vec2 r2 = vec2(-152.94239396, 59.28637943);
	const vec2 g2 = vec2(4.27729857, 2.82956604);
	const vec2 b2 = vec2(-89.90310912, 27.34824973);
	t = clamp(t, 0.0, 1.0);
	vec4 v4 = vec4(1.0, t, t * t, t * t * t);
	vec2 v2 = v4.zw * v4.z;
	return clamp(vec3(dot(v4, r4) + dot(v2, r2), dot(v4, g4) + dot(v2, g2), dot(v4, b4) + dot(v2, b2)), 0.0, 1.0);
}

// Height colors: from sea level to the highest point on land (gray or
// rainbow), blue in water, darker when deeper (sea level at 0). As in render.
const vec3 deepWater = vec3(25.0, 45.0, 100.0) / 255.0;
const vec3 shallowWater = vec3(110.0, 160.0, 215.0) / 255.0;

vec3 heightColor(float z, bool water) {
	if (water) {
		float t = clamp((z - lowest) / max(-lowest, 1.0), 0.0, 1.0);
		return srgbToLinear(mix(deepWater, shallowWater, t));
	}
	float t = clamp(z / max(highest, 1.0), 0.0, 1.0);
	return rainbow ? srgbToLinear(turbo(0.1 + 0.9 * t)) : srgbToLinear(vec3(t));
}

void main() {
	vPosition = position;
	vec4 viewPosition = view * vec4(position.xy - size / 2.0, position.z * zScale, 1.0);
	vViewPosition = viewPosition.xyz;

	int alpha = int(round(terrainColor.a * 255.0));
	bool water = alpha >= waterBit;
	if (heightColors)
		vColor = heightColor(position.z, water);
	else if (singleColor && !water)
		vColor = srgbToLinear(landColor);
	else
		vColor = srgbToLinear(terrainColor.rgb);

	// The shade of blocks, in hundredths, of the sRGB color
	int shade = alpha % waterBit;
	if (shade > 0)
		vColor *= pow(float(shade) / 100.0, 2.2);

	// The overlay is top row first
	vUV = vec2(position.x / size.x, 1.0 - position.y / size.y);

	gl_Position = projection * viewPosition;
	vDepth = gl_Position.w;
}
`

const terrainFragmentShader = `
#version 330 core

in vec3 vPosition;
in vec3 vViewPosition;
in vec3 vColor;
in vec2 vUV;
in float vDepth;

uniform vec2 size;
uniform bool logDepth;       // at eye level: from millimeters to the horizon
uniform float logDepthScale; // 1 / log2(far + 1)
uniform float hazeDistance;  // map units, 0 for none
uniform vec3 hazeColor;      // linear
uniform bool lit;
uniform vec3 lightDirection; // view space
uniform bool hasOverlay;
uniform sampler2D overlay;
uniform bool hasPaint;
uniform sampler2D paint; // top row first, as the overlay
uniform bool hasHole;
uniform vec4 hole; // a rectangle not drawn, where finer ground is: x0, y0, x1, y1
uniform float paintOpacity;

out vec4 fragColor;

const float PI = 3.141592653589793;

vec3 srgbToLinear(vec3 c) {
	return mix(c / 12.92, pow((c + 0.055) / 1.055, vec3(2.4)), step(0.04045, c));
}

vec3 linearToSrgb(vec3 c) {
	return mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(0.0031308, c));
}

void main() {
	// Hide the margin around the map, and the hole
	if (any(lessThan(vPosition.xy, vec2(0.0))) || any(greaterThan(vPosition.xy, size)))
		discard;
	if (hasHole && all(greaterThan(vPosition.xy, hole.xy)) && all(lessThan(vPosition.xy, hole.zw)))
		discard;

	vec3 color = vColor;
	if (hasPaint)
		color = mix(color, srgbToLinear(texture(paint, vUV).rgb), paintOpacity);

	if (lit) {
		vec3 normal = normalize(cross(dFdx(vViewPosition), dFdy(vViewPosition)));
		float diffuse = max(dot(normal, lightDirection), 0.0);
		color *= (1.0 + 3.0 * diffuse) / PI;
	}

	// The overlay over it, unlit: rivers at full brightness, whatever the
	// terrain. Premultiplied sRGB colors, decoded once divided by alpha.
	if (hasOverlay) {
		vec4 overlayColor = texture(overlay, vUV);
		if (overlayColor.a > 0.0)
			color = mix(color, srgbToLinear(overlayColor.rgb / overlayColor.a), overlayColor.a);
	}

	// Distance, at eye level
	if (hazeDistance > 0.0)
		color = mix(hazeColor, color, exp(-length(vViewPosition) / hazeDistance));

	fragColor = vec4(linearToSrgb(clamp(color, 0.0, 1.0)), 1.0);
	gl_FragDepth = logDepth ? log2(1.0 + vDepth) * logDepthScale : gl_FragCoord.z;
}
`

type terrainVertex struct {
	x, y, z    float32
	r, g, b, a uint8 // a: waterBit for water, plus a shade (see withShade)
}

// waterBit is the flag of water vertices, in their alpha. The other bits
// are a shade of the color in hundredths (0: none), in every color mode,
// which tells the blocks apart.
const waterBit = 128

// withShade has the color multiplied by factor, from 0.01 to 1.27.
func (v terrainVertex) withShade(factor float64) terrainVertex {
	v.a = v.a&waterBit | uint8(math.Max(1, math.Min(waterBit-1, math.Round(factor*100))))
	return v
}

func newTerrainView() (*terrainView, error) {
	p, err := newProgram(terrainVertexShader, terrainFragmentShader)
	if err != nil {
		return nil, err
	}

	v := &terrainView{program: p} // the overlay is decoded in the shader
	v.mesh.init()
	return v, nil
}

// meshBuffers are the buffers of a mesh of terrain vertices.
type meshBuffers struct {
	vao, vbo, ebo uint32
	count         int32 // indices
}

func (m *meshBuffers) init() {
	gl.GenVertexArrays(1, &m.vao)
	gl.GenBuffers(1, &m.vbo)
	gl.GenBuffers(1, &m.ebo)

	gl.BindVertexArray(m.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, m.vbo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, m.ebo)

	stride := int32(unsafe.Sizeof(terrainVertex{}))
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, stride, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 4, gl.UNSIGNED_BYTE, true, stride, 12)
	gl.BindVertexArray(0)
}

func (m *meshBuffers) upload(vertices []terrainVertex, indices []uint32) {
	if m.vao == 0 {
		m.init()
	}
	gl.BindVertexArray(m.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, m.vbo)
	if len(vertices) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*int(unsafe.Sizeof(terrainVertex{})), gl.Ptr(vertices), gl.STATIC_DRAW)
	}
	if len(indices) > 0 {
		gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Ptr(indices), gl.STATIC_DRAW)
	}
	gl.BindVertexArray(0)
	m.count = int32(len(indices))
}

func (m *meshBuffers) draw() {
	if m.count == 0 {
		return
	}
	gl.BindVertexArray(m.vao)
	gl.DrawElementsWithOffset(gl.TRIANGLES, m.count, gl.UNSIGNED_INT, 0)
	gl.BindVertexArray(0)
}

func (m *meshBuffers) delete() {
	if m.vao != 0 {
		gl.DeleteVertexArrays(1, &m.vao)
		gl.DeleteBuffers(1, &m.vbo)
		gl.DeleteBuffers(1, &m.ebo)
		*m = meshBuffers{}
	}
}

// setMesh uploads the mesh of a world. It returns whether the map size
// changed: the camera needs resetting then.
func (v *terrainView) setMesh(w *gen.World) bool {
	mesh := w.Mesh

	vertices := make([]terrainVertex, len(mesh.Points))
	for i, p := range mesh.Points {
		z := w.Elevation[i]
		if math.IsNaN(z) || math.IsInf(z, 0) {
			z = 0
		}
		color := render.VertexColor(w, int32(i))
		vertices[i] = terrainVertex{x: float32(p.X), y: float32(p.Y), z: float32(z), r: color[0], g: color[1], b: color[2]}
		if w.IsWater(int32(i)) {
			vertices[i].a = waterBit
		}
	}

	indices := make([]uint32, 0, 3*len(mesh.Triangles))
	for _, t := range mesh.Triangles {
		indices = append(indices, uint32(t[0]), uint32(t[1]), uint32(t[2]))
	}

	v.mesh.upload(vertices, indices)

	width, height := float64(w.Width), float64(w.Height)
	changed := width != v.width || height != v.height
	v.width, v.height = width, height
	v.lowest, v.highest = w.Lowest, w.Highest
	v.metersPerPixel = w.MetersPerPixel
	if v.metersPerPixel <= 0 {
		v.metersPerPixel = 1
	}
	return changed
}

// resetCamera frames the whole map, and puts the eye at its center.
func (v *terrainView) resetCamera() {
	if v.width == 0 {
		return
	}
	v.orbit.reset(v.width, v.height)
	v.eye.fromOrbit(&v.orbit)
}

// Eye level: the nearest distance seen, the eye's height above the ground,
// in meters, and the haze
const (
	eyeNear     = 0.2
	eyeHeight   = 1.7
	hazeMeters  = 150e3
	eyeFarRatio = 3 // of the map's extent
)

// camera is the view and projection of a 3D view, orbit or eye level, and
// the far distance of the eye's logarithmic depth (0 for the orbit).
func (v *terrainView) camera(view string, aspect float64) (mgl64.Mat4, mgl64.Mat4, float64) {
	if view == viewEye {
		far := eyeFarRatio * math.Max(v.width, v.height)
		return v.eye.view(), v.eye.projection(aspect, eyeNear/v.metersPerPixel, far), far
	}
	return v.orbit.view(), v.orbit.projection(aspect), 0
}

// overlayScale is the overlay resolution, relative to the map image:
// detailed enough for close ups, within GPU limits.
func (v *terrainView) overlayScale() float64 {
	var maxSize int32
	gl.GetIntegerv(gl.MAX_TEXTURE_SIZE, &maxSize)
	extent := math.Max(v.width, v.height)
	return math.Min(4, math.Min(8192, float64(maxSize))/2/extent)
}

func (v *terrainView) draw(settings *Settings, aspect float64) {
	gl.ClearColor(float32(srgbEncode(skyColor[0])), float32(srgbEncode(skyColor[1])), float32(srgbEncode(skyColor[2])), 1)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	if v.mesh.count == 0 {
		return
	}

	view, projection, far := v.camera(settings.View, aspect)

	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	gl.Enable(gl.CULL_FACE)
	gl.CullFace(gl.BACK)
	gl.FrontFace(gl.CCW)
	if settings.Wireframe {
		gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
	}

	p := v.program
	p.use()
	p.setMat4("view", view)
	p.setMat4("projection", projection)
	p.setVec2("size", v.width, v.height)
	p.setFloat("lowest", v.lowest)
	p.setInt("heightColors", boolInt(settings.Color == colorHeight))
	p.setInt("singleColor", boolInt(settings.Color == colorSingle))
	land := settings.LandColor
	p.setVec3("landColor", mgl64.Vec3{float64(land[0]) / 255, float64(land[1]) / 255, float64(land[2]) / 255})
	p.setInt("rainbow", boolInt(settings.HeightScale == render.ScaleRainbow))
	p.setFloat("highest", v.highest)
	p.setFloat("zScale", settings.VerticalScale/v.metersPerPixel)
	p.setInt("logDepth", boolInt(far > 0))
	p.setFloat("logDepthScale", 1/math.Log2(max(far, 1)+1))
	haze := 0.0
	if settings.View == viewEye {
		haze = hazeMeters / v.metersPerPixel
	}
	p.setFloat("hazeDistance", haze)
	p.setVec3("hazeColor", mgl64.Vec3(skyColor))
	p.setInt("lit", boolInt(settings.Shading == shadingLit))
	p.setVec3("lightDirection", view.Mat3().Mul3x1(lightDirection).Normalize())
	p.setInt("hasOverlay", boolInt(v.hasOverlay))
	p.setInt("overlay", 0)
	if v.hasOverlay {
		v.overlay.bind(0)
	}
	hasPaint := v.paint != nil && v.paint.id != 0 && v.paintOpacity > 0
	p.setInt("hasPaint", boolInt(hasPaint))
	p.setInt("paint", 1)
	p.setFloat("paintOpacity", v.paintOpacity)
	if hasPaint {
		v.paint.bind(1)
	}

	// At eye level, finer ground around the eye: each layer drawn in a hole
	// of the coarser one, which it meets at its edges
	holes := make([][4]float64, len(v.details))
	for i := range v.details {
		holes[i] = v.details[i].bounds
	}
	hole := func(i int) {
		p.setInt("hasHole", boolInt(i < len(holes)))
		if i < len(holes) {
			gl.Uniform4f(p.location("hole"), float32(holes[i][0]), float32(holes[i][1]), float32(holes[i][2]), float32(holes[i][3]))
		}
	}
	hole(0)
	v.mesh.draw()
	for i := range v.details {
		hole(i + 1)
		v.details[i].mesh.draw()
	}

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
