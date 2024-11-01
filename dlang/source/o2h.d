import std.stdio;
import std.random;
import std.algorithm;

import gamut;
import delaunator;
import graph;

Image loadOutline(string path)
{
	Image image;
	image.loadFromFile(path);
	if (image.isError)
		throw new Exception("Could not load " ~ path);

	return image;
}

Graph generateGraph(int width, int height, int res)
{
	Point[] points;
	auto margin = 100;

	for(auto x = 0; x < width; x += res)
	for(auto y = 0; y < height; y += res)
		points ~= Point(
			x + uniform(-res / 2, res / 2),
			y + uniform(-res / 2, res / 2)
		);

	for(auto x = -margin; x < width + margin; x += res)
	{
		points ~= Point(x, -margin);
		points ~= Point(x, height + margin);
	}

	for(auto y = -margin; y < height + margin; y += res)
	{
		points ~= Point(-margin, y);
		points ~= Point(height + margin, y);
	}

	auto triangulation = Triangulation(points);
	auto graph = new Graph(triangulation.edges);

	return graph;
}

Graph relaxGraph(Graph g, int width, int height)
{
	Point[] points;

	foreach (center; g.centers)
	{
		if (center.x < 0 || center.x > width || center.y < 0 || center.y > height)
			points ~= Point(center.x, center.y);
		else
			points ~= center.corners.fold!((a,b) => a + b) / center.corners.length;
	}

	auto triangulation = Triangulation(points);
	auto graph = new Graph(triangulation.edges);

	return graph;
}

void main(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path> <resolution>");
		return;
	}

	auto path = args[1];
	auto resolution = 10;

	writeln("Loading " ~ path);
	auto outline = loadOutline(path);

	writeln("Generating graph...");
	auto graph = generateGraph(outline.width, outline.height, resolution);

	writeln("Relaxing graph...");
	graph = relaxGraph(graph, outline.width, outline.height);
}
