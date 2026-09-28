package app

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl64"
)

// Anisotropic filtering, from EXT_texture_filter_anisotropic (core in 4.6),
// supported by every desktop driver.
const (
	glTextureMaxAnisotropy    = 0x84FE
	glMaxTextureMaxAnisotropy = 0x84FF
)

type program struct {
	id       uint32
	uniforms map[string]int32
}

func newProgram(vertexSource, fragmentSource string) (*program, error) {
	vs, err := compileShader(gl.VERTEX_SHADER, vertexSource)
	if err != nil {
		return nil, fmt.Errorf("vertex shader: %w", err)
	}
	defer gl.DeleteShader(vs)

	fs, err := compileShader(gl.FRAGMENT_SHADER, fragmentSource)
	if err != nil {
		return nil, fmt.Errorf("fragment shader: %w", err)
	}
	defer gl.DeleteShader(fs)

	id := gl.CreateProgram()
	gl.AttachShader(id, vs)
	gl.AttachShader(id, fs)
	gl.LinkProgram(id)

	var status int32
	gl.GetProgramiv(id, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		log := programLog(id)
		gl.DeleteProgram(id)
		return nil, fmt.Errorf("link: %s", log)
	}

	return &program{id: id, uniforms: map[string]int32{}}, nil
}

func compileShader(kind uint32, source string) (uint32, error) {
	id := gl.CreateShader(kind)
	src, free := gl.Strs(source + "\x00")
	defer free()
	gl.ShaderSource(id, 1, src, nil)
	gl.CompileShader(id)

	var status int32
	gl.GetShaderiv(id, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var length int32
		gl.GetShaderiv(id, gl.INFO_LOG_LENGTH, &length)
		log := strings.Repeat("\x00", int(length)+1)
		gl.GetShaderInfoLog(id, length, nil, gl.Str(log))
		gl.DeleteShader(id)
		return 0, fmt.Errorf("%s", strings.TrimRight(log, "\x00"))
	}
	return id, nil
}

func programLog(id uint32) string {
	var length int32
	gl.GetProgramiv(id, gl.INFO_LOG_LENGTH, &length)
	log := strings.Repeat("\x00", int(length)+1)
	gl.GetProgramInfoLog(id, length, nil, gl.Str(log))
	return strings.TrimRight(log, "\x00")
}

func (p *program) use() { gl.UseProgram(p.id) }

func (p *program) location(name string) int32 {
	loc, ok := p.uniforms[name]
	if !ok {
		loc = gl.GetUniformLocation(p.id, gl.Str(name+"\x00"))
		p.uniforms[name] = loc
	}
	return loc
}

func (p *program) setInt(name string, v int32) { gl.Uniform1i(p.location(name), v) }

func (p *program) setFloat(name string, v float64) { gl.Uniform1f(p.location(name), float32(v)) }

func (p *program) setVec2(name string, x, y float64) {
	gl.Uniform2f(p.location(name), float32(x), float32(y))
}

func (p *program) setVec3(name string, v mgl64.Vec3) {
	gl.Uniform3f(p.location(name), float32(v[0]), float32(v[1]), float32(v[2]))
}

func (p *program) setMat4(name string, m mgl64.Mat4) {
	var f [16]float32
	for i, v := range m {
		f[i] = float32(v)
	}
	gl.UniformMatrix4fv(p.location(name), 1, false, &f[0])
}

// texture is a color texture with mipmaps, top row first. An sRGB texture is
// decoded to linear colors when sampled, otherwise colors are used as is.
type texture struct {
	id            uint32
	srgb          bool
	width, height int
}

// upload replaces the texture content with an image.
func (t *texture) upload(img *image.RGBA) {
	format := int32(gl.RGBA8)
	if t.srgb {
		format = gl.SRGB8_ALPHA8
	}

	if t.id == 0 {
		gl.GenTextures(1, &t.id)
	}
	t.width, t.height = img.Rect.Dx(), img.Rect.Dy()

	gl.BindTexture(gl.TEXTURE_2D, t.id)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, int32(img.Stride/4))
	gl.TexImage2D(gl.TEXTURE_2D, 0, format, int32(t.width), int32(t.height), 0,
		gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))
	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
	gl.GenerateMipmap(gl.TEXTURE_2D)

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

	var anisotropy float32
	gl.GetFloatv(glMaxTextureMaxAnisotropy, &anisotropy)
	if anisotropy > 0 {
		gl.TexParameterf(gl.TEXTURE_2D, glTextureMaxAnisotropy, anisotropy)
	}
}

// uploadPixels replaces the texture content with RGBA pixels, rows from the
// top, without mipmaps: for textures then updated in parts (updateRegion).
func (t *texture) uploadPixels(width, height int, pixels []byte) {
	if t.id == 0 {
		gl.GenTextures(1, &t.id)
	}
	t.width, t.height = width, height

	gl.BindTexture(gl.TEXTURE_2D, t.id)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(width), int32(height), 0,
		gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
}

// updateRegion replaces the pixels of a rectangle (rows from the top) of a
// texture made by uploadPixels.
func (t *texture) updateRegion(r image.Rectangle, pixels []byte) {
	if t.id == 0 || r.Empty() {
		return
	}
	gl.BindTexture(gl.TEXTURE_2D, t.id)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()),
		gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
}

// setSmooth chooses between linear and nearest magnification.
func (t *texture) setSmooth(smooth bool) {
	filter := int32(gl.NEAREST)
	if smooth {
		filter = gl.LINEAR
	}
	gl.BindTexture(gl.TEXTURE_2D, t.id)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter)
}

func (t *texture) bind(unit uint32) {
	gl.ActiveTexture(gl.TEXTURE0 + unit)
	gl.BindTexture(gl.TEXTURE_2D, t.id)
}

func (t *texture) delete() {
	if t.id != 0 {
		gl.DeleteTextures(1, &t.id)
		t.id = 0
	}
}

// srgbEncode converts a linear color component to sRGB.
func srgbEncode(c float64) float64 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}
