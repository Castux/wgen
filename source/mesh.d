import geom;
import std.stdio;

// Vertices and faces are secondary information, they only hold info about
// one connected edge. All the rest is deduced from the edge's data.

class Vertex
{
	Point	p;			// The actual payload

	Edge	edge;		// One arbitrary edge leaving from this vertex

	bool valid()
	{
		return edge !is null;
	}

	// Get all edges leaving from this vertex, ccw
	Edge[] edges()
	{
		return edge.origOrbit;
	}

	Vertex[] neighbours()
	{
		Vertex[] res;
		res.length = edges.length;

		foreach(i, e; edges)
			res[i] = e.dest;

		return res;
	}

	Edge split(Face left, Face right)
	{
		return edge.mesh.splitVertex(this, left, right);
	}

	void collapse()
	{
		edge.mesh.collapseVertex(this);
	}
}

class Face
{
	Point	p;

	Edge	edge;		// One arbitrary edge going ccw around this face (ie, this face is left of the edge)

	bool valid()
	{
		return edge !is null;
	}

	// Get all edges around this face, ccw
	Edge[] edges()
	{
		return edge.leftOrbit;
	}

	Vertex[] vertices()
	{
		Vertex[] res;
		res.length = edges.length;

		foreach(i, e; edges)
			res[i] = e.dest;

		return res;
	}

	Face[] neighbours()
	{
		Face[] res;
		res.length = edges.length;

		foreach(i, e; edges)
			res[i] = e.right;

		return res;
	}

	Edge split(Vertex orig, Vertex dest)
	{
		return edge.mesh.splitFace(this, orig, dest);
	}

	// Add a new vertex linked to all corners of the face
	Vertex triangulate()
	{
		Vertex[] corners = vertices;
		assert(corners.length >= 3);

		// Add all edges to the right, so that "this" remains the big part
		Edge tmp = split(corners[0],corners[1]);
		Vertex middle = tmp.split();

		foreach(corner; corners[2 .. $])
		{
			split(middle, corner);
		}

		return middle;
	}

	bool contains(Point p)
	{
		auto vs = vertices;
		Point[] points;
		points.length = vertices.length;

		foreach(i, v; vertices)
			points[i] = v.p;

		auto poly = Convex(points);
		return poly.contains(p);
	}

	Circle circumcircle()
	{
		Point[] corners;
		corners.length = vertices.length;

		foreach(j, c; vertices)
			corners[j] = c.p;

		return Convex(corners).circumcircle;
	}

	Convex convex()
	{
		import std.algorithm;
		import std.array;
		return Convex(vertices.map!"a.p".array);
	}
}

// Edges are the core component of the mesh
// We have a pseudo quad-edge system, where edges go by pair:
// one for each direction
// A "full" edge hold 8 pieces of data:
// - 2 vertices
// - 2 faces
// - the 4 "outmost" connected edges (the ones that circle around the 2 faces)
//
// Each directed simple edge holds half of this:
// - the destination vertex
// - the left face
// - the nextL and nextD edges
//
// Semantics: the "nextX" edge is the edge obtained by ccw rotation around X,
// keeping the same orientation related to X. Meaning:
// - if X is a face, we leave it on the same side
// - if X is a vertex, we keep pointing to or from it the same way
//
//	"prev" means the same with cw rotation
//
// X can be L(eft), R(ight), O(rigin), D(estination)
//
// The choice of which information is "raw" and which is deduced (from reversal) is
// arbitrary, but should be irrelevant at use. The container Mesh class should take
// care of all edge operations, so that the user doesn't break anything

class Edge
{
	Mesh	mesh;

	// The reversed edge
	Edge	rev;

	// The raw information
	Vertex	dest;
	Face	left;
	Edge	nextL;
	Edge	nextD;

	// The deduced information
	Vertex orig()
	{
		return rev.dest;
	}

	Face right()
	{
		return rev.left;
	}

	Edge nextR()
	{
		return rev.nextL.rev;
	}

	Edge nextO()
	{
		return rev.nextD.rev;
	}

	this(Mesh m)
	{
		mesh = m;
	}

	// The setter methods that set things back

	void setDest(Vertex d)
	{
		dest = d;
		d.edge = this.rev;
	}

	void setLeft(Face f)
	{
		left = f;
		f.edge = this;
	}

	void setNextL(Edge e)
	{
		nextL = e;
		e.rev.nextD = this;
	}

	void setNextD(Edge e)
	{
		nextD = e;
		e.nextL = this.rev;
	}

	// Check some invariants that should always be true
	void validate()
	{
		assert(mesh && rev && dest && left && nextL && nextD);
		assert(rev.rev is this);
		assert(nextL.rev.nextD is this);
		assert(nextD.nextL.rev is this);
	}

	// A non-atomic convenience function
	// Insert new vertex v on given edge
	// Return the middle vertex (which is the new one)

	// O-------------e------------->D
	// O------n----->Dnew---e------>D

	Vertex split()
	{
		auto newedge = mesh.splitVertex(orig, left, right);
		return newedge.dest;
	}

	Edge[] leftOrbit()
	{
		Edge[] res;

		Edge curr = this;
		do
		{
			res ~= curr;
			curr = curr.nextL;
		} while(curr !is this);

		return res;
	}

	Edge[] origOrbit()
	{
		Edge[] res;

		Edge curr = this;
		do
		{
			res ~= curr;
			curr = curr.nextO;
		} while(curr !is this);

		return res;
	}

	void collapseVertices()
	{
		mesh.collapseEdgeVertex(this);
	}

	void collapseFaces()
	{
		mesh.collapseEdgeFace(this);
	}

	// Is this edge on the right of the point?
	bool isRightOf(Point p)
	{
		return (dest.p - orig.p).cross(p - orig.p) > 0;
	}
}

// This mesh class is meant to do all the maintenance on the elements, and
// keep correctness. It represents the topology of a connected planar graph, closed:
// all elements have complete information: there is always a left and right face.
// This happens on closed surfaces (like spheres), or on planes if we consider the "outside" as
// a face.
//
// Note that these classes represent only the topology. It doesn't care about actual positions
// and planarity.
//
// Note also that to ensure correctness, it must be initialized with a polygon. All subsequent operations
// are transformations of the current mesh, which keep correctness. Hopefully.
// We choose a flat 2 point polygon.

class Mesh
{
	bool[Edge]		edges;		// Only one of two reverses goes here
	bool[Vertex]	vertices;
	bool[Face]		faces;

	// Makes a pair of reversed edges, return one of them
	protected Edge newEdge()
	{
		auto e1 = new Edge(this);
		auto e2 = new Edge(this);

		e1.rev = e2;
		e2.rev = e1;

		edges[e1] = true;

		return e1;
	}

	protected Vertex newVertex()
	{
		auto v = new Vertex;
		vertices[v] = true;

		return v;
	}

	protected Face newFace()
	{
		auto f = new Face;
		faces[f] = true;

		return f;
	}

	void killEdge(Edge edge)
	{
		foreach(e; [edge, edge.rev])
		{
			// Remove all possible references
			e.left.edge = e.nextL;
			e.dest.edge = e.nextL;
			e.orig.edge = e.nextO;

			edges.remove(e);
		}
	}

	// Create the initial polygon: one vertex, connected to itself
	this()
	{
		// Two faces
		auto inside = newFace();
		auto outside = newFace();

		// One vertex
		auto v = newVertex();

		// Two opposite edges
		auto e1 = newEdge();
		auto e2 = e1.rev;

		// Plug stuff together
		e1.setDest(v);
		e1.setLeft(inside);
		e1.setNextL(e1);
		e1.setNextD(e2);

		e2.setDest(v);
		e2.setLeft(outside);

		e1.validate;
		e2.validate;
	}

	// Get any edge
	Edge getEdge()
	{
		return edges.keys[0];
	}

	Edge[] getEdges()
	{
		return edges.keys;
	}

	Vertex getVertex()
	{
		return vertices.keys[0];
	}

	// Split a vertex in two, by inserting an edge between the given faces
	// Create new vertex at the destination point

	Edge splitVertex(Vertex v, Face left, Face right)
	{
		// Check that both faces touch the given vertex
		Edge futureNextL;
		Edge futureNextD;

		Edge tmp = left.edge;
		do
		{
			if(tmp.orig is v)
			{
				futureNextL = tmp;
				break;
			}
			tmp = tmp.nextL;
		}
		while(tmp !is left.edge);


		tmp = right.edge;
		do
		{
			if(tmp.dest is v)
			{
				futureNextD = tmp;
				break;
			}
			tmp = tmp.nextL;
		}
		while(tmp !is right.edge);

		assert(futureNextL && futureNextD, "both faces do not touch vertex");

		auto futureNextR = futureNextD.nextL.rev;
		auto futureNextO = futureNextL.nextO;

		Vertex nv = newVertex();
		Edge n = newEdge();

		n.setDest(nv);
		n.setLeft(left);
		n.setNextL(futureNextL);
		n.setNextD(futureNextD);

		n.rev.setDest(v);
		n.rev.setLeft(right);
		n.rev.setNextL(futureNextR.rev);
		n.rev.setNextD(futureNextO.rev);

		// Fix orbit of new destination
		tmp = n.nextD;
		do
		{
			tmp.setDest(nv);
			tmp = tmp.nextD;
		}
		while(tmp !is n.nextD);

		n.validate;

		return n;
	}

	// Remove edge, merge its origin and destination vertices (delete destination)
	// (reverse operation of splitVertex)

	void collapseEdgeVertex(Edge e)
	{
		auto orig = e.orig;
		auto dest = e.dest;

		// Reset destination orbit's destination vertex
		auto tmp = e.nextD;
		do
		{
			tmp.setDest(orig);
			tmp = tmp.nextD;
		}
		while(tmp !is e.nextD);

		// Patch the edges
		e.nextR.setNextD(e.nextD);
		e.nextO.rev.setNextL(e.nextL);

		// Delete edge and vertex
		killEdge(e);
		vertices.remove(dest);
	}

	// Add new edge between orig and dest, by splitting given face in two
	// (new face is at the right of the new edge)

	Edge splitFace(Face face, Vertex orig, Vertex dest)
	{
		// Check that both vertices are actually around the given face
		// collect edges that point at them
		Edge origedge;
		Edge destedge;
		Edge tmp = face.edge;
		do
		{
			if(tmp.dest is orig)
				origedge = tmp;
			if(tmp.dest is dest)
				destedge = tmp;

			if(origedge && destedge)
				break;

			tmp = tmp.nextL;
		}
		while(tmp !is face.edge);

		assert(origedge && destedge, "vertices are not on the face");

		auto newface = newFace();
		auto n = newEdge();

		n.setDest(dest);
		n.setLeft(face);
		n.setNextL(destedge.nextL);
		n.setNextD(destedge);

		n.rev.setDest(orig);
		n.rev.setLeft(newface);
		n.rev.setNextL(origedge.nextL);
		n.rev.setNextD(origedge);

		// Fix orbit of the new face
		tmp = n.rev;
		do
		{
			tmp.setLeft(newface);
			tmp = tmp.nextL;
		}
		while(tmp !is n.rev);

		n.validate;
		return n;
	}

	// Remove edge by merging left and right faces (delete right face)
	// (reverse operation of splitFace)
	// Destination and origin must have at least two other edges, so that they're not left "hanging"

	void collapseEdgeFace(Edge e)
	{
		// Make sure we're not part of a "chain" and removing us would leave edges hanging
		assert(e.nextD.nextD !is e);
		assert(e.nextO.nextO !is e);

		// Patch sides
		e.nextD.setNextL(e.nextL);
		e.rev.nextD.setNextL(e.rev.nextL);

		// Fix orbit's face
		auto tmp = e.nextL;
		do
		{
			tmp.setLeft(e.left);
			tmp = tmp.nextL;
		}
		while(tmp !is e.nextL);

		// Cleanup
		faces.remove(e.right);
		killEdge(e);
	}

	Face findFaceContaining(Point p)
	{
		// Trivial and slow algorithm
		foreach(f; faces.byKey)
		{
			if(f.contains(p))
				return f;
		}

		return null;
	}

	// Assuming all faces are triangles
	// And that p is indeed in an existing face

	Face fastFaceFind(Point p)
	{
		// Start from a random edge
		Edge curr = getEdge;

		// Assuming all faces are triangles, (curr, curr.nextO, and curr.nextL) is one

		int i = 0;
		auto total = edges.length;

		while(true)
		{
			if(i++ > total)
				return null;

			// Keep the target on the left
			if(!curr.isRightOf(p))
				curr = curr.rev;

			// Keep the target in the sector formed by us and nextO
			else if(curr.nextO.isRightOf(p))
				curr = curr.nextO;

			// If target is beyond nextL, start again from there
			else if(!curr.nextL.isRightOf(p))
				curr = curr.nextL.rev;

			// If it's before nextL, it's in our left triangle!
			else
				break;
		}

		Face result = curr.left;
		assert(result.contains(p));
		return result;
	}

	// Works only if p falls inside a face
	Vertex insertVertex(Point p)
	{
		Face f = fastFaceFind(p);
		if(!f)
		{
			f = findFaceContaining(p);
			if(!f)
				return null;
		}

		Vertex v = f.triangulate();
		v.p = p;

		return v;
	}

	// Works only if edge is the diagonal of a quad
	Edge swapEdge(Edge e)
	{
		Edge next = e.nextL;
		e.collapseFaces();		// This kills the right face
		Edge rotated = next.left.split(next.nextL.nextL.dest, next.dest);
		return rotated;
	}

	// Remove vertex, all attached edges, and merge all attached faces into one
	// All neighbour vertices must have at least two other neighbours (so they're not left "hanging")
	void collapseVertex(Vertex v)
	{
		while(v.edges.length > 2)
		{
			v.edge.collapseFaces;
		}

		// We now have only two edges left, connected by a single vertex
		// Remember them before killing v
		auto eds = v.edges;

		eds[0].rev.collapseVertices;	// Middle vertex is destroyed, only one inside edge left
		eds[1].collapseFaces;
	}
}
