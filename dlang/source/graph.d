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

		Center[HalfEdge] centerMap;
		foreach(he; halfEdges)
		{
			if (he in centerMap)
				continue;

			auto he1 = he;
			auto he2 = he1.next;
			auto he3 = he2.next;

			Point c = circumcenter(he1.from, he2.from, he3.from);
			Center center = new Center(c);

			centerMap[he1] = center;
			centerMap[he2] = center;
			centerMap[he3] = center;

			auto e1 = edgeMap[he1];
			auto e2 = edgeMap[he2];
			auto e3 = edgeMap[he3];

			center.edges ~= e1;
			center.edges ~= e2;
			center.edges ~= e3;

			e1.centers ~= center;
			e2.centers ~= center;
			e3.centers ~= center;

			centers ~= center;
		}

	}
}
