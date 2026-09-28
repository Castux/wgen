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
	delaunayPoints := make([]delaunay.Point, len(points))
	for i, p := range points {
		delaunayPoints[i] = delaunay.Point{X: p.X, Y: p.Y}
	}

	triangulation, err := delaunay.Triangulate(delaunayPoints)
	if err != nil {
		return nil, fmt.Errorf("triangulation failed: %w", err)
	}

	// Compact the points actually used by triangles

	vertexOf := make([]int32, len(points)) // vertex of each input point, -1 if unused
	for i := range vertexOf {
		vertexOf[i] = -1
	}

	m := &Mesh{}
	for _, p := range triangulation.Triangles {
		if vertexOf[p] < 0 {
			vertexOf[p] = int32(len(m.Points))
			m.Points = append(m.Points, points[p])
		}
	}

	vertexCount := len(m.Points)
	triangleCount := len(triangulation.Triangles) / 3

	// Triangles, and the vertex -> triangles relation

	m.Triangles = make([][3]int32, triangleCount)
	m.Circumcenters = make([]geom.Vec2, triangleCount)
	m.VertexTriangles = make([][]int32, vertexCount)

	for t := range triangleCount {
		var vertices [3]int32
		for k := range 3 {
			vertices[k] = vertexOf[triangulation.Triangles[3*t+k]]
			m.VertexTriangles[vertices[k]] = append(m.VertexTriangles[vertices[k]], int32(t))
		}

		center := geom.Circumcenter(m.Points[vertices[0]], m.Points[vertices[1]], m.Points[vertices[2]])
		m.Circumcenters[t] = center

		// All vertices are on the circumcircle, so sorting them by angle
		// around its center orders them counterclockwise
		slices.SortFunc(vertices[:], func(a, b int32) int {
			return cmpAngle(m.Points[a].Sub(center), m.Points[b].Sub(center))
		})
		m.Triangles[t] = vertices
	}

	// Neighbours. Each undirected edge is either two opposite half edges,
	// or a single hull half edge.

	m.Neighbours = make([][]int32, vertexCount)
	for e, opposite := range triangulation.Halfedges {
		if opposite >= 0 && opposite < e {
			continue
		}

		from := vertexOf[triangulation.Triangles[e]]
		to := vertexOf[triangulation.Triangles[nextHalfedge(e)]]

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

// nextHalfedge is the next half edge of the same triangle.
func nextHalfedge(e int) int {
	if e%3 == 2 {
		return e - 2
	}
	return e + 1
}

// cmpAngle compares the angles of two vectors, counterclockwise.
func cmpAngle(a, b geom.Vec2) int {
	angleA, angleB := geom.PseudoAngle(a), geom.PseudoAngle(b)
	switch {
	case angleA < angleB:
		return -1
	case angleA > angleB:
		return 1
	}
	return 0
}
