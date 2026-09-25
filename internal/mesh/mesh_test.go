package mesh

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/Castux/wgen/internal/geom"
)

func randomPoints(n int) []geom.Vec2 {
	r := rand.New(rand.NewPCG(1, 2))
	points := make([]geom.Vec2, n)
	for i := range points {
		points[i] = geom.Vec2{X: r.Float64() * 100, Y: r.Float64() * 100}
	}
	return points
}

func TestBuild(t *testing.T) {
	points := randomPoints(500)
	points = append(points, points[0]) // duplicate, gets dropped

	m, err := Build(points)
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Points) != 500 {
		t.Errorf("expected 500 vertices, got %d", len(m.Points))
	}

	for v, ns := range m.Neighbours {
		for _, n := range ns {
			if !slices.Contains(m.Neighbours[n], int32(v)) {
				t.Fatalf("neighbours not symmetric: %d -> %d", v, n)
			}
		}
	}

	for _, tri := range m.Triangles {
		a, b, c := m.Points[tri[0]], m.Points[tri[1]], m.Points[tri[2]]
		if geom.Cross2(b.Sub(a), c.Sub(a)) <= 0 {
			t.Fatalf("triangle not counterclockwise: %v", tri)
		}
	}

	// Euler: E = V + F - 1 (F without the outer face), and each triangle
	// is in the cell of its 3 vertices
	edges, cells := 0, 0
	for v := range m.Points {
		edges += len(m.Neighbours[v])
		cells += len(m.VertexTriangles[v])
	}
	if edges/2 != len(m.Points)+len(m.Triangles)-1 {
		t.Errorf("Euler characteristic mismatch: V=%d E=%d F=%d", len(m.Points), edges/2, len(m.Triangles))
	}
	if cells != 3*len(m.Triangles) {
		t.Errorf("cells: %d, triangles: %d", cells, len(m.Triangles))
	}
}

func TestRelax(t *testing.T) {
	m, _ := Build(randomPoints(500))
	inside := func(p geom.Vec2) bool { return p.X > 10 && p.X < 90 && p.Y > 10 && p.Y < 90 }

	r, err := m.Relax(func(p geom.Vec2) bool { return !inside(p) })
	if err != nil {
		t.Fatal(err)
	}

	moved := 0
	for i := range m.Points {
		if m.Points[i] != r.Points[i] {
			moved++
		}
	}
	if moved == 0 || len(r.Points) != len(m.Points) {
		t.Errorf("relax moved %d of %d points", moved, len(m.Points))
	}
}
