import std.stdio;
import std.random;
import std.algorithm;
import std.array;

import gamut;
import delaunator;
import graph;

class Center : CenterBase!(Center, Edge, Corner)
{
	int foo;

	this(Point p)
	{
		 super(p);
	}
}

class Edge : EdgeBase!(Center, Edge, Corner)
{
	int meh;
	this()
	{
	}
}

class Corner : CornerBase!(Center, Edge, Corner)
{
	int lol;
	this(Point p)
	{
		super(p);
	}
}

class Map : Graph!(Center, Edge, Corner)
{
	Image outline;
	int width;
	int height;
	int resolution;

	this(string path, int resolution)
	{
		outline.loadFromFile(path);
		if (outline.isError)
			throw new Exception("Could not load " ~ path);

		this.width = outline.width;
		this.height = outline.height;
		this.resolution = resolution;

		auto triangulation = generateTriangulation(width, height, resolution);

		super(triangulation.edges);
	}

	void relaxGraph()
	{
		auto points = centers.map!(c => c.corners.map!"a.p".fold!((a,b) => a + b) / c.corners.length).array;

		auto triangulation = Triangulation(points);
		auto tmp = new Graph!(Center, Edge, Corner)(triangulation.edges);

		edges = tmp.edges;
		centers = tmp.centers;
		corners = tmp.corners;
	}
}

private Triangulation generateTriangulation(int width, int height, int res)
{
	Point[] points;
	auto margin = 4 * res;

	for(auto x = -margin; x < width + margin; x += res)
	for(auto y = -margin; y < height + margin; y += res)
		points ~= Point(
			x + uniform(-res / 2, res / 2),
			y + uniform(-res / 2, res / 2)
		);
	//
	// for(auto x = -margin; x < width + margin; x += res)
	// {
	// 	points ~= Point(x, -margin);
	// 	points ~= Point(x, height + margin);
	// }
	//
	// for(auto y = -margin; y < height + margin; y += res)
	// {
	// 	points ~= Point(-margin, y);
	// 	points ~= Point(height + margin, y);
	// }

	return Triangulation(points);
}
//
// Map relaxGraph(Map g, int width, int height)
// {
// 	Point[] points;
//
// 	foreach (center; g.centers)
// 	{
// 		if (center.x < 0 || center.x > width || center.y < 0 || center.y > height)
// 			points ~= Point(center.x, center.y);
// 		else
// 			points ~= center.corners.map!"a.p".fold!((a,b) => a + b) / center.corners.length;
// 	}
//
// 	auto triangulation = Triangulation(points);
// 	auto graph = new Map(triangulation.edges);
//
// 	return graph;
// }
