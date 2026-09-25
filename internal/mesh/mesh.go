// Package mesh builds the Delaunay triangulation of a point set and its dual
// graph.
//
// Vertices are the triangulated points: each has neighbours (the Delaunay
// edges) and a cell (the dual Voronoi polygon, made of the circumcenters of
// the triangles around it). Triangles are the Delaunay triangles.
package mesh

import (
	"fmt"
	"slices"

	"github.com/fogleman/delaunay"

	"github.com/Castux/wgen/internal/geom"
)

type Mesh struct {
	Points     []geom.Vec2
	Neighbours [][]int32

	// Triangles around each vertex, counterclockwise. Their circumcenters
	// form the Voronoi cell of the vertex.
	VertexTriangles [][]int32

	// Vertex indices of each triangle, counterclockwise.
	Triangles     [][3]int32
	Circumcenters []geom.Vec2
}

// Build triangulates the points. Points that the triangulation skipped
// (duplicates) are dropped, so indices in the mesh may not match the input.
func Build(points []geom.Vec2) (*Mesh, error) {
	dpoints := make([]delaunay.Point, len(points))
	for i, p := range points {
		dpoints[i] = delaunay.Point{X: p.X, Y: p.Y}
	}

	tri, err := delaunay.Triangulate(dpoints)
	if err != nil {
		return nil, fmt.Errorf("triangulation failed: %w", err)
	}

	// Compact the points actually used by triangles

	index := make([]int32, len(points))
	for i := range index {
		index[i] = -1
	}

	m := &Mesh{}
	for _, p := range tri.Triangles {
		if index[p] < 0 {
			index[p] = int32(len(m.Points))
			m.Points = append(m.Points, points[p])
		}
	}

	numVertices := len(m.Points)
	numTriangles := len(tri.Triangles) / 3

	// Triangles, and the vertex -> triangles relation

	m.Triangles = make([][3]int32, numTriangles)
	m.Circumcenters = make([]geom.Vec2, numTriangles)
	m.VertexTriangles = make([][]int32, numVertices)

	for t := range numTriangles {
		var vs [3]int32
		for k := range 3 {
			vs[k] = index[tri.Triangles[3*t+k]]
			m.VertexTriangles[vs[k]] = append(m.VertexTriangles[vs[k]], int32(t))
		}

		center := geom.Circumcenter(m.Points[vs[0]], m.Points[vs[1]], m.Points[vs[2]])
		m.Circumcenters[t] = center

		// All vertices are on the circumcircle, so sorting them by angle
		// around its center orders them counterclockwise
		slices.SortFunc(vs[:], func(a, b int32) int {
			return cmpAngle(m.Points[a].Sub(center), m.Points[b].Sub(center))
		})
		m.Triangles[t] = vs
	}

	// Neighbours. Each undirected edge is either two opposite half edges,
	// or a single hull half edge.

	m.Neighbours = make([][]int32, numVertices)
	for e, opposite := range tri.Halfedges {
		if opposite >= 0 && opposite < e {
			continue
		}

		from := index[tri.Triangles[e]]
		to := index[tri.Triangles[nextHalfedge(e)]]

		m.Neighbours[from] = append(m.Neighbours[from], to)
		m.Neighbours[to] = append(m.Neighbours[to], from)
	}

	// Order cells counterclockwise

	for v, triangles := range m.VertexTriangles {
		p := m.Points[v]
		slices.SortFunc(triangles, func(a, b int32) int {
			return cmpAngle(m.Circumcenters[a].Sub(p), m.Circumcenters[b].Sub(p))
		})
	}

	return m, nil
}

func nextHalfedge(e int) int {
	if e%3 == 2 {
		return e - 2
	}
	return e + 1
}

func cmpAngle(a, b geom.Vec2) int {
	pa, pb := geom.PseudoAngle(a), geom.PseudoAngle(b)
	switch {
	case pa < pb:
		return -1
	case pa > pb:
		return 1
	}
	return 0
}

// Relax moves every vertex for which keep returns false to the centroid of its
// Voronoi cell (the average of its circumcenters), and triangulates again.
func (m *Mesh) Relax(keep func(geom.Vec2) bool) (*Mesh, error) {
	points := make([]geom.Vec2, len(m.Points))

	for v, p := range m.Points {
		if keep(p) {
			points[v] = p
			continue
		}

		var sum geom.Vec2
		for _, t := range m.VertexTriangles[v] {
			sum = sum.Add(m.Circumcenters[t])
		}
		points[v] = sum.Scale(1 / float64(len(m.VertexTriangles[v])))
	}

	return Build(points)
}
