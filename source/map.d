import mesh;
import geom;
import std.random;
import std.stdio;
import std.math: isNaN;
import std.format;

class Map
{
	num		width;
	num		height;

	num		margin;

	Mesh			mesh;
	bool[Vertex]	dummies;

	this(num w, num h)
	{
		width = w;
		height = h;
		margin = 0;
		mesh = new Mesh;
	}

	Point toSVGCoors(Point p)
	{
		return Point(p.x, height - p.y) + Point(margin,margin);
	}

	bool inBounds(Point p)
	{
		return (p.x > 0 && p.x < width && p.y > 0 && p.y < height);
	}

	string toSVG()
	{
		string s;

		s =
	`<?xml version="1.0" encoding="utf-8"?>
	<svg
	xmlns="http://www.w3.org/2000/svg"
	version="1.1"
	`;

		s ~= format("width=\"%s\"\n\theight=\"%s\">\n", width + 2*margin, height + 2*margin);
		s ~= "<title>Test</title>\n";

		// Background


		s ~= `<polygon points="`;
		foreach(p; [Point(0,0),
					Point(width,0),
					Point(width,height),
					Point(0,height) ])
		{
			p = toSVGCoors(p);
			s ~= format("%s,%s ", p.x, p.y);
		}

		s ~= `" fill="green" />` ~ '\n';

		// Voronoi

		foreach(vert; mesh.vertices.byKey)
		{
			auto edges = vert.edges;
			Point[] corners;
			corners.length = edges.length;

			foreach(i, e; edges)
				corners[i] = e.left.p;

			s ~= `<polygon points="`;
			foreach(v; corners)
			{
				auto p = toSVGCoors(v);
				s ~= format("%s,%s ", p.x, p.y);
			}

			auto grey = uniform(0,255);
			s ~= format(`" fill="rgb(%s,%s,%s)" />`, grey, grey, grey) ~ '\n';
		}

		// foreach(edge; mesh.edges.byKey)
		// {
		// 	Point p1 = toSVGCoors(edge.left.p);
		// 	Point p2 = toSVGCoors(edge.right.p);
		//
		// 	s ~= format(`<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="blue" stroke-width="1" />` ~ '\n',
		// 		p1.x, p1.y, p2.x, p2.y );
		// }

		// Delaunay
		//
		// foreach(face; mesh.faces.byKey)
		// {
		// 	// if(face is outside)
		// 	// 	continue;
		// 	//
		// 	auto corners = face.vertices;
		// 	s ~= `<polygon points="`;
		// 	foreach(v; corners)
		// 	{
		// 		auto p = toSVGCoors(v.p);
		// 		s ~= format("%s,%s ", p.x, p.y);
		// 	}
		// 	s ~= `" />\n`;
		// 	//s ~= format(`" fill="rgb(%s,%s,%s)" />`, uniform(0,255),uniform(0,255),uniform(0,255)) ~ '\n';
		// }
		//
		//
		// foreach(vert; mesh.vertices.byKey)
		// {
		// 	Point p = toSVGCoors(vert.p);
		// 	s ~= format(`<circle cx="%s" cy="%s" r="2" fill="black" />` ~ '\n', p.x, p.y );
		// }
		//
		//
		// foreach(edge; mesh.edges.byKey)
		// {
		// 	if(edge.orig in dummies || edge.dest in dummies)
		// 		continue;
		//
		// 	Point p1 = toSVGCoors(edge.orig.p);
		// 	Point p2 = toSVGCoors(edge.dest.p);
		//
		// 	s ~= format(`<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="black" stroke-width="1" />` ~ '\n',
		// 		p1.x, p1.y, p2.x, p2.y );
		// }


		s ~= "</svg>\n";

		return s;
	}

	void makeDelaunay(Point[] points)
	{
		mesh = new Mesh;
		auto e = mesh.getEdge;
		auto outside = e.right;

		// Create frame of dummies, since our algo can only add points
		// inside existing faces

		for(uint i = 0 ; i < 3 ; i++)
			e.split();

		auto verts = e.left.vertices;
		assert(verts.length == 4);

		auto margin = 1000;

		verts[0].p = Point(-margin, -margin);
		verts[1].p = Point(width + margin, -margin);
		verts[2].p = Point(width + margin, height + margin);
		verts[3].p = Point(-margin, height + margin);

		foreach(v; verts)
			dummies[v] = true;

		foreach(count, p; points)
		{
			assert(inBounds(p));
			if(count%100 == 0)
				writeln(count);

			auto newv = mesh.insertVertex(p);

			// Start the big Delaunay inspection of suspicious edges
			// They are the edges of the original triangle

			auto tmp = newv.edges;

			Edge[] suspicious;
			suspicious.length = tmp.length;

			foreach(i, t; tmp)
				suspicious[i] = t.nextL;

			while(suspicious.length > 0)
			{
				auto curr = suspicious[$-1];
				suspicious.length = suspicious.length - 1;
				auto tri = curr.right;

				if(tri.convex.insideCircumcirle(p))
				{
					curr = mesh.swapEdge(curr);				// Rotate ccw the edge in the quad
					suspicious ~= curr.rev.nextL;			// Recurse down the newly modified tri
					suspicious ~= curr.nextO.rev;
				}
			}
		}

		// And Voronoi centers

		foreach(face; mesh.faces.byKey)
		{
			if(face.edges.length > 3)
				continue;

			face.p = face.circumcircle.c;
		}
	}

	void makeMultiPassDelaunay(Point[] points)
	{
		for(uint i = 0 ; i < 5 ; i++)
		{
			makeDelaunay(points);

			// Collect pseudo centroids of Voronoi cells
			Point[] centers;

			foreach(vert; mesh.vertices.byKey)
			{
				auto neigh = vert.neighbours;
				auto sum = vert.p;
				foreach(n; neigh)
				{
					sum = sum + n.p;
				}

				sum = sum / vert.neighbours.length;

				centers ~= sum;
			}

			points = centers;

			auto fp = File(format("fubar%02s.svg", i), "w");
			fp.write(toSVG);
			fp.close;
		}
	}
}
