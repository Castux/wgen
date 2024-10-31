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

			auto hedges = [he, he.next, he.next.next];
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

			// Find all the halfedges coming from the same point
			HalfEdge[] hedges;
			auto current = he;

			// Get to the last edge (if on the hull, there is one)
			while (current.rev)
			{
				current = current.rev.next;
				if (current is he)
					break;
			}

			// Traverse the other way
			auto start = current;
			while (current)
			{
				hedges ~= current;
				auto tmp = current.next.next;

				if (!tmp.rev)
				{
					// This half edge (coming into the center) is on the hull
					// so it doesn't have a coming out counterpart.
					// We still want to register its full edge
					hedges ~= tmp;
					break;
				}

				current = tmp.rev;

				if (current is start)
					break;
			}

			Center center = new Center(he.from);

			foreach(hedge; hedges)
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
