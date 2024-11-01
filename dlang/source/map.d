import std.stdio;
import std.random;
import std.algorithm;
import std.array;
import std.math;
import std.conv;

import gamut;
import delaunator;
import graph;

enum Terrain
{
	none,
	sea,
	plains,
	hills,
	mountains,
	lake,
	flat,
	cliffs
}

struct Pixel
{
	ubyte r,g,b;
}

const Terrain[Pixel] colors =
[
	Pixel(66, 66, 125): Terrain.sea,

	Pixel(135, 168, 81): Terrain.plains,
	Pixel(209, 184, 134): Terrain.hills,
	Pixel(101, 72, 31): Terrain.mountains,

	Pixel(109, 148, 194): Terrain.lake,
	Pixel(153, 153, 153): Terrain.flat,
	Pixel(148, 10, 0): Terrain.cliffs
];

class Center : CenterBase!(Center, Edge, Corner)
{
	Terrain terrain;

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
		outline.loadFromFile(path, LOAD_RGB | LOAD_8BIT | LOAD_NO_ALPHA);
		if (outline.isError)
			throw new Exception("Could not load " ~ path);

		this.width = outline.width;
		this.height = outline.height;
		this.resolution = resolution;

		auto triangulation = generateTriangulation(width, height, resolution);

		super(triangulation.edges);
	}

	bool inBounds(int row, int col)
	{
		return col >= 0 && col < width && row >= 0 && row < height;
	}

	Pixel getPixel(int row, int col)
	{
		assert(outline.type == PixelType.rgb8);
  		assert(outline.hasData());
		auto scanline = cast(ubyte[]) outline.scanline(row);

		return Pixel(
			scanline[col * 3 + 0],
			scanline[col * 3 + 1],
			scanline[col * 3 + 2]
		);
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

	void assignTerrainTypes()
	{
		foreach(center; centers)
		{
			int row = center.y.to!int;
			int col = center.x.to!int;

			if (!inBounds(col, row))
			{
				center.terrain = Terrain.none;
				continue;
			}

			auto pixel = getPixel(row, col);
			center.terrain = colors.get(pixel, Terrain.none);
		}
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
