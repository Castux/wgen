import delaunator;

alias HalfEdge = delaunator.Edge;

class CenterBase(Center, Edge, Corner)
{
	Point p;
	alias p this;

	Center[] neighbours;
	Edge[] edges;
	Corner[] corners;

	this(Point p)
	{
		this.p = p;
	}
}

class EdgeBase(Center, Edge, Corner)
{
	Center center1, center2;
	Corner corner1, corner2;

	void addCenter(Center c)
	{
		if (center1 is null)
			center1 = c;
		else
			center2 = c;
	}

	void addCorner(Corner c)
	{
		if (corner1 is null)
			corner1 = c;
		else
			corner2 = c;
	}
}

class CornerBase(Center, Edge, Corner)
{
	Point p;
	alias p this;

	Corner[] neighbours;
	Edge[] edges;
	Center[] centers;

	this(Point p)
	{
		this.p = p;
	}
}

class Graph(Center, Edge, Corner)
{
	Center[] centers;
	Corner[] corners;
	Edge[] edges;

	this(HalfEdge[] halfEdges)
	{
		// Create edges. Associate pairs of opposite half edges
		// to the same full edge

		Edge[HalfEdge] edgeMap;
		foreach(he; halfEdges)
		{
			if (he in edgeMap)
				continue;

			Edge edge = new Edge();
			edgeMap[he] = edge;
			if (he.rev)
				edgeMap[he.rev] = edge;

			edges ~= edge;
		}

		// Create corners at the circumcenter of each triangle
		// Associate them with the three edges of the triangle

		Corner[HalfEdge] cornerMap;
		foreach(he; halfEdges)
		{
			if (he in cornerMap)
				continue;

			auto tri = he.triangle;
			Point c = circumcenter(tri.e1.from, tri.e2.from, tri.e3.from);
			Corner corner = new Corner(c);

			foreach(hedge; tri.tupleof)
			{
				cornerMap[hedge] = corner;

				auto e = edgeMap[hedge];
				corner.edges ~= e;
				e.addCorner(corner);
			}

			corners ~= corner;
		}

		// Create centers for each original point given to triangulate
		// Associate all the edges in their orbits

		Center[HalfEdge] centerMap;
		foreach(he; halfEdges)
		{
			if (he in centerMap)
				continue;

			auto orbit = he.orbit;
			Center center = new Center(he.from);

			foreach(orbitHedge; orbit)
			{
				centerMap[orbitHedge] = center;

				auto e = edgeMap[orbitHedge];
				e.addCenter(center);
				center.edges ~= e;

				// On the hull, we need to also add the one edge comes into
				// this node (but don't put it in the map or it might
				// get skipped for its own center)

				if (orbitHedge.onHull)
				{
					e = edgeMap[orbitHedge.hullPrev];
					e.addCenter(center);
					center.edges ~= e;
				}
			}

			centers ~= center;
		}

		// Connect centers to centers, and corners to corners,
		// via the edges

		foreach(edge; edges)
		{
			assert(edge.center1 && edge.center2);

			// Every edge must connect two centers (they're the original
			// point we triangulated)

			auto c1 = edge.center1;
			auto c2 = edge.center2;

			c1.neighbours ~= c2;
			c2.neighbours ~= c1;

			// but hull edges don't connect corners to anything

			if (edge.corner2 !is null)
			{
				auto co1 = edge.corner1;
				auto co2 = edge.corner2;

				co1.neighbours ~= co2;
				co2.neighbours ~= co1;
			}
		}

		// Finally, connect centers to corners and vice-versa

		foreach(he; halfEdges)
		{
			auto corner = cornerMap[he];
			auto center = centerMap[he];

			corner.centers ~= center;
			center.corners ~= corner;
		}

		// Order clockwise

		import std.algorithm;

		foreach(corner; corners)
			corner.centers.sort!((a,b) => (a.p - corner.p).angle < (b.p - corner.p).angle);

		foreach(center; centers)
			center.corners.sort!((a,b) => (a.p - center.p).angle < (b.p - center.p).angle);
	}
}
