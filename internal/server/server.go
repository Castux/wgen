// Package server serves the web viewer and its API, and regenerates the world
// when the config or the outline image change.
//
// API:
//
//	GET  /api/state       engine state (version, busy, error, dirty)
//	GET  /api/events      the same, as server-sent events on every change
//	GET  /api/config      current config, JSON
//	POST /api/config      apply a partial config (same format), regenerate
//	POST /api/save        write the current config to its file
//	POST /api/export      export the current world as configured
//	GET  /api/schema      editable parameters, for building the UI
//	GET  /api/mesh        mesh, binary (see encodeMesh)
//	GET  /api/render.png  rendered image (see renderOptions)
package server

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Castux/wgen/internal/config"
	"github.com/Castux/wgen/internal/export"
	"github.com/Castux/wgen/internal/gen"
	"github.com/Castux/wgen/internal/render"
)

type Server struct {
	engine     *Engine
	watcher    *Watcher
	configPath string
	static     fs.FS

	mu         sync.Mutex
	imagePath  string // currently watched
	meshCache  cached
	imageCache map[string]cached
}

type cached struct {
	version int
	data    []byte
}

const imageCacheSize = 16

func New(configPath string, static fs.FS) (*Server, error) {
	watcher, err := NewWatcher()
	if err != nil {
		return nil, err
	}

	s := &Server{
		engine:     NewEngine(),
		watcher:    watcher,
		configPath: configPath,
		static:     static,
		imageCache: map[string]cached{},
	}

	if err := watcher.Watch(configPath, s.loadConfig); err != nil {
		return nil, err
	}
	s.loadConfig()

	return s, nil
}

// loadConfig (re)loads the config file. On failure, the current world is kept
// and the error reported to clients.
func (s *Server) loadConfig() {
	conf, warnings, err := config.Load(s.configPath)
	for _, w := range warnings {
		slog.Warn(w, "config", s.configPath)
	}
	if err != nil {
		slog.Error("could not load config", "err", err)
		s.engine.SetError(err)
		return
	}

	slog.Info("loaded config", "path", s.configPath)
	s.watchImage(conf.Path)
	s.engine.SetConfig(conf, true)
}

func (s *Server) watchImage(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if path == s.imagePath {
		return
	}
	if s.imagePath != "" {
		s.watcher.Unwatch(s.imagePath)
	}

	s.imagePath = path
	err := s.watcher.Watch(path, func() {
		slog.Info("outline image changed", "path", path)
		s.engine.ReloadImage()
	})
	if err != nil {
		slog.Warn("cannot watch image", "path", path, "err", err)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/config", s.handlePatchConfig)
	mux.HandleFunc("POST /api/save", s.handleSave)
	mux.HandleFunc("POST /api/export", s.handleExport)
	mux.HandleFunc("GET /api/schema", s.handleSchema)
	mux.HandleFunc("GET /api/mesh", s.handleMesh)
	mux.HandleFunc("GET /api/render.png", s.handleRender)
	mux.Handle("GET /", http.FileServerFS(s.static))

	return mux
}

// Run serves until the listener fails.
func (s *Server) Run(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "localhost"
	}
	slog.Info("viewer ready", "url", fmt.Sprintf("http://%s", net.JoinHostPort(host, port)))

	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(ln)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.engine.State())
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	states, cancel := s.engine.Subscribe()
	defer cancel()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case state := <-states:
			data, _ := json.Marshal(state)
			fmt.Fprintf(w, "data: %s\n\n", data)
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
		case <-r.Context().Done():
			return
		}
		flusher.Flush()
	}
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	conf := s.engine.Config()
	if conf == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no config loaded"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(conf.Marshal())
}

func (s *Server) handlePatchConfig(w http.ResponseWriter, r *http.Request) {
	conf := s.engine.Config()
	if conf == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no config loaded"))
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	patched, err := conf.Patch(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if patched.Path != conf.Path {
		s.watchImage(patched.Path)
	}

	s.engine.SetConfig(patched, false)
	writeJSON(w, http.StatusAccepted, map[string]string{"stage": gen.ChangedStage(conf, patched).String()})
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	conf := s.engine.Config()
	if conf == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no config loaded"))
		return
	}

	if err := conf.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	slog.Info("saved config", "path", conf.ConfigPath)
	s.engine.MarkSaved()
	writeJSON(w, http.StatusOK, map[string]string{"path": conf.ConfigPath})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	world, _ := s.engine.Snapshot()
	if world == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("nothing generated yet"))
		return
	}

	files, err := export.All(world)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	conf := s.engine.Config()
	if conf == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no config loaded"))
		return
	}
	writeJSON(w, http.StatusOK, conf.Schema())
}

func (s *Server) handleMesh(w http.ResponseWriter, r *http.Request) {
	world, version := s.engine.Snapshot()
	if world == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("nothing generated yet"))
		return
	}

	s.mu.Lock()
	c := s.meshCache
	s.mu.Unlock()

	if c.data == nil || c.version != version {
		c = cached{version, encodeMesh(world, version)}
		s.mu.Lock()
		s.meshCache = c
		s.mu.Unlock()
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Wgen-Version", strconv.Itoa(version))
	w.Write(c.data)
}

// encodeMesh serializes the mesh, little endian:
//
//	header:    "WGM1", u32 version, u32 vertex count, u32 triangle count,
//	           f32 width, height, lowest, highest
//	positions: f32 x, y, z per vertex (z is 0 outside the map)
//	indices:   u32 * 3 per triangle, counterclockwise
//	colors:    u8 r, g, b, a per vertex (terrain color)
func encodeMesh(w *gen.World, version int) []byte {
	m := w.Mesh
	var buf bytes.Buffer
	buf.Grow(32 + len(m.Points)*16 + len(m.Triangles)*12)

	le := binary.LittleEndian
	buf.WriteString("WGM1")
	for _, v := range []uint32{uint32(version), uint32(len(m.Points)), uint32(len(m.Triangles))} {
		binary.Write(&buf, le, v)
	}
	for _, v := range []float64{float64(w.Width), float64(w.Height), w.Lowest, w.Highest} {
		binary.Write(&buf, le, float32(v))
	}

	positions := make([]float32, 0, 3*len(m.Points))
	for v, p := range m.Points {
		z := w.Z[v]
		if math.IsNaN(z) || math.IsInf(z, 0) {
			z = 0
		}
		positions = append(positions, float32(p.X), float32(p.Y), float32(z))
	}
	binary.Write(&buf, le, positions)

	indices := make([]uint32, 0, 3*len(m.Triangles))
	for _, t := range m.Triangles {
		indices = append(indices, uint32(t[0]), uint32(t[1]), uint32(t[2]))
	}
	binary.Write(&buf, le, indices)

	colors := make([]byte, 0, 4*len(m.Points))
	for v := range m.Points {
		c := render.VertexColor(w, int32(v))
		colors = append(colors, c[0], c[1], c[2], 255)
	}
	buf.Write(colors)

	return buf.Bytes()
}

// renderOptions reads the render parameters from the query:
//
//	scale       relative to the outline image (default 1)
//	base        terrain, height or none (default terrain)
//	shading     1 for hillshading
//	riverPower  river width growth (default 0.5)
//	riverWidth  max river width, world units (default 0: no rivers)
//	contours    contour interval (default 0: none)
//	grid        grid size (default 0: none)
func renderOptions(r *http.Request) (render.Options, error) {
	q := r.URL.Query()
	o := render.Options{Scale: 1, Base: render.BaseTerrain, RiverPower: 0.5}

	var errs []error
	float := func(key string, dst *float64) {
		if v := q.Get(key); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(f) || f < 0 {
				errs = append(errs, fmt.Errorf("bad %s: %q", key, v))
				return
			}
			*dst = f
		}
	}

	float("scale", &o.Scale)
	float("riverPower", &o.RiverPower)
	float("riverWidth", &o.RiverWidth)
	float("contours", &o.Contours)
	float("grid", &o.Grid)
	o.Shading = q.Get("shading") == "1"

	if b := q.Get("base"); b != "" {
		o.Base = render.Base(b)
		if o.Base != render.BaseTerrain && o.Base != render.BaseHeight && o.Base != render.BaseNone {
			errs = append(errs, fmt.Errorf("bad base: %q", b))
		}
	}
	if o.Scale <= 0 {
		errs = append(errs, errors.New("scale must be positive"))
	}

	return o, errors.Join(errs...)
}

func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	world, version := s.engine.Snapshot()
	if world == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("nothing generated yet"))
		return
	}

	o, err := renderOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	key := fmt.Sprintf("%+v", o)

	s.mu.Lock()
	c, ok := s.imageCache[key]
	s.mu.Unlock()

	if !ok || c.version != version {
		start := time.Now()
		img := render.Render(world, o)

		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&buf, img); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		c = cached{version, buf.Bytes()}
		slog.Debug("rendered", "options", key, "took", time.Since(start).Round(time.Millisecond))

		s.mu.Lock()
		for k, v := range s.imageCache {
			if v.version != version || len(s.imageCache) >= imageCacheSize {
				delete(s.imageCache, k)
			}
		}
		s.imageCache[key] = c
		s.mu.Unlock()
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Wgen-Version", strconv.Itoa(version))
	w.Write(c.data)
}
