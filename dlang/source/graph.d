import delaunator;

alias HalfEdge = delaunator.Edge;

class Center
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

class Edge
{
	Center[] centers;
	Corner[] corners;
}

class Corner
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

class Graph
{
	Center[] centers;
	Corner[] corners;

	this(HalfEdge[] halfEdges)
	{
		Edge[] edges;

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

		Corner[HalfEdge] cornerMap;
		foreach(he; halfEdges)
		{
			if (he in cornerMap)
				continue;

			auto hedges = he.triangle;
			Point c = circumcenter(hedges[0].from, hedges[1].from, hedges[2].from);
			Corner corner = new Corner(c);

			foreach(hedge; hedges)
			{
				cornerMap[hedge] = corner;

				auto e = edgeMap[hedge];
				corner.edges ~= e;
				e.corners ~= corner;
			}

			corners ~= corner;
		}

		Center[HalfEdge] centerMap;
		foreach(he; halfEdges)
		{
			if (he in centerMap)
				continue;

			auto orbit = he.orbit;

			// On the hull, we need to also add the one edge that doesn't come
			// out of this node
			foreach(h; orbit)
			{
				if (!h.rev)
				{
					orbit ~= h.hullPrev;
					break;
				}
			}

			Center center = new Center(he.from);

			foreach(hedge; orbit)
			{
				centerMap[hedge] = center;

				auto e = edgeMap[hedge];
				e.centers ~= center;
				center.edges ~= e;
			}

			centers ~= center;
		}

		foreach(edge; edges)
		{
			assert(edge.centers.length == 2);

			auto c1 = edge.centers[0];
			auto c2 = edge.centers[1];

			c1.neighbours ~= c2;
			c2.neighbours ~= c1;

			if (edge.corners.length == 2)
			{
				auto co1 = edge.corners[0];
				auto co2 = edge.corners[1];

				co1.neighbours ~= co2;
				co2.neighbours ~= co1;
			}

		}
	}
}
