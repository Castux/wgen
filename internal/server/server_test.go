package server

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const testConfig = `{
	"path": "island.png",
	"resolution": 4, "grid": "hex", "jitter": 0.5, "relax": false,
	"smoothingRadius": 0, "erosionMinFlow": 5, "erosionFactor": 0.5,
	"terrains": {
		"sea": { "r": 66, "g": 66, "b": 125, "gradient": -0.1, "fixedShore": 0.0 },
		"land": { "r": 135, "g": 168, "b": 81, "gradient": 0.5 }
	}
}`

func writeIsland(t *testing.T, radius float64) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 80))
	for y := range 80 {
		for x := range 100 {
			c := color.NRGBA{66, 66, 125, 255}
			if math.Hypot(float64(x-50), float64(y-40)) < radius {
				c = color.NRGBA{135, 168, 81, 255}
			}
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	if err := os.WriteFile("island.png", buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type harness struct {
	t      *testing.T
	s      *Server
	http   *httptest.Server
	states <-chan State
}

func setup(t *testing.T) *harness {
	t.Chdir(t.TempDir())
	writeIsland(t, 30)
	os.WriteFile("config.json", []byte(testConfig), 0o644)

	static := fstest.MapFS{"index.html": {Data: []byte("viewer")}}
	s, err := New("config.json", static)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.watcher.Close() })

	h := &harness{t: t, s: s, http: httptest.NewServer(s.Handler())}
	t.Cleanup(h.http.Close)

	states, cancel := s.engine.Subscribe()
	t.Cleanup(cancel)
	h.states = states

	h.waitFor(func(st State) bool { return st.Ready && !st.Busy })
	return h
}

// waitFor waits for a state matching f.
func (h *harness) waitFor(f func(State) bool) State {
	h.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case st := <-h.states:
			if f(st) {
				return st
			}
		case <-timeout:
			h.t.Fatalf("timeout, state %+v", h.s.engine.State())
		}
	}
}

func (h *harness) do(method, path, body string) (*http.Response, []byte) {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.http.URL+path, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func TestEndpoints(t *testing.T) {
	h := setup(t)

	// Mesh
	resp, data := h.do("GET", "/api/mesh", "")
	if resp.StatusCode != 200 || string(data[:4]) != "WGM1" {
		t.Fatalf("mesh: %d %q", resp.StatusCode, data[:min(4, len(data))])
	}
	le := binary.LittleEndian
	numVertices, numTriangles := le.Uint32(data[8:]), le.Uint32(data[12:])
	if want := 32 + 16*int(numVertices) + 12*int(numTriangles); len(data) != want {
		t.Errorf("mesh size %d, expected %d", len(data), want)
	}

	// Render
	resp, data = h.do("GET", "/api/render.png?scale=2&base=height&shading=1&riverWidth=3&contours=5&grid=10", "")
	if resp.StatusCode != 200 {
		t.Fatalf("render: %d %s", resp.StatusCode, data)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 200 {
		t.Fatalf("render: %v %v", err, img.Bounds())
	}

	resp, _ = h.do("GET", "/api/render.png?base=purple", "")
	if resp.StatusCode != 400 {
		t.Errorf("bad render options: %d", resp.StatusCode)
	}

	// Config and schema
	resp, data = h.do("GET", "/api/config", "")
	var conf map[string]any
	if json.Unmarshal(data, &conf); conf["resolution"] != 4.0 {
		t.Errorf("config: %s", data)
	}

	_, data = h.do("GET", "/api/schema", "")
	if !bytes.Contains(data, []byte(`"path":["terrains","land","gradient"]`)) {
		t.Errorf("schema: %s", data)
	}

	// Static files
	_, data = h.do("GET", "/", "")
	if string(data) != "viewer" {
		t.Errorf("index: %q", data)
	}
}

func TestPatchSave(t *testing.T) {
	h := setup(t)
	v0 := h.s.engine.State().Version

	resp, data := h.do("POST", "/api/config", `{"resolution": -1}`)
	if resp.StatusCode != 400 {
		t.Errorf("invalid patch: %d %s", resp.StatusCode, data)
	}

	resp, data = h.do("POST", "/api/config", `{"erosionFactor": 0.2}`)
	if resp.StatusCode != 202 || !bytes.Contains(data, []byte("erosion")) {
		t.Fatalf("patch: %d %s", resp.StatusCode, data)
	}
	st := h.waitFor(func(st State) bool { return st.Version > v0 && !st.Busy })
	if !st.Dirty || st.Stage != "erosion" {
		t.Errorf("after patch: %+v", st)
	}

	// Save writes the file, which reloads as is: nothing regenerated
	resp, _ = h.do("POST", "/api/save", "")
	if resp.StatusCode != 200 {
		t.Fatalf("save: %d", resp.StatusCode)
	}
	saved, _ := os.ReadFile("config.json")
	if !bytes.Contains(saved, []byte(`"erosionFactor": 0.2`)) {
		t.Errorf("saved config: %s", saved)
	}

	time.Sleep(500 * time.Millisecond)
	if st := h.s.engine.State(); st.Dirty || st.Version != v0+1 {
		t.Errorf("after save: %+v", st)
	}

	// Export (nothing enabled but heightmaps, by default)
	resp, data = h.do("POST", "/api/export", "")
	if resp.StatusCode != 200 || !bytes.Contains(data, []byte("config.json-w.png")) {
		t.Errorf("export: %d %s", resp.StatusCode, data)
	}
}

func TestHotReload(t *testing.T) {
	h := setup(t)
	v0 := h.s.engine.State().Version

	// Broken config: error reported, world kept
	os.WriteFile("config.json", []byte(`{"path": `), 0o644)
	st := h.waitFor(func(st State) bool { return st.Error != "" })
	if !st.Ready || st.Version != v0 {
		t.Errorf("after broken config: %+v", st)
	}

	// Fixed config: regenerated, error cleared
	os.WriteFile("config.json", []byte(strings.Replace(testConfig, `"resolution": 4`, `"resolution": 5`, 1)), 0o644)
	st = h.waitFor(func(st State) bool { return st.Version > v0 && !st.Busy })
	if st.Error != "" || st.Stage != "mesh" {
		t.Errorf("after fixed config: %+v", st)
	}

	// New image, same size: from terrain
	writeIsland(t, 20)
	st = h.waitFor(func(st State) bool { return st.Version > v0+1 && !st.Busy })
	if st.Stage != "terrain" {
		t.Errorf("after image change: %+v", st)
	}
}
