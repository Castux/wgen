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
	Edge[] edges;
	Corner[] corners;

	this(HalfEdge[] halfEdges)
	{
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
			do
			{
				hedges ~= current;
				current = current.rev ? current.rev.next : null;

			} while (current && current !is he);

			if (current is null)
			{
				// If the vertex is on the hull (we broke because no rev)
				// We have to loop the other way too

				current = he;
				while (true)
				{
					current = current.next.next.rev;
					if (!current)
						break;
					hedges ~= current;
				}
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
			if (edge.centers.length == 2)
			{
				auto c1 = edge.centers[0];
				auto c2 = edge.centers[1];

				c1.neighbours ~= c2;
				c2.neighbours ~= c1;
			}

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
