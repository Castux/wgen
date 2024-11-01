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
	bool shore;

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

	Center[] shores;

	this(string path, int resolution)
	{
		outline.loadFromFile(path, LOAD_RGB | LOAD_8BIT | LOAD_NO_ALPHA);
		if (outline.isError)
			throw new Exception("Could not load " ~ path);

		this.width = outline.width;
		this.height = outline.height;
		this.resolution = resolution;

		writeln("Triangulating");
		auto triangulation = generateTriangulation(width, height, resolution);

		writeln("Building graph");
		super(triangulation.edges);

		generate();
	}

	private void generate()
	{
		writeln("Relaxing");
		relaxGraph();

		writeln("Assingning terrain types");
		assignTerrainTypes();

		writeln("Computing elevation");
		computeElevation();
	}

	private void relaxGraph()
	{
		auto points = centers.map!(c => c.corners.map!"a.p".fold!((a,b) => a + b) / c.corners.length).array;

		auto triangulation = Triangulation(points);
		auto tmp = new Graph!(Center, Edge, Corner)(triangulation.edges);

		edges = tmp.edges;
		centers = tmp.centers;
		corners = tmp.corners;
	}

	private Pixel getPixel(int row, int col)
	{
		row = clamp(row, 0, height - 1);
		col = clamp(col, 0, width - 1);

		assert(outline.type == PixelType.rgb8);
  		assert(outline.hasData());

		auto scanline = cast(ubyte[]) outline.scanline(row);

		return Pixel(
			scanline[col * 3 + 0],
			scanline[col * 3 + 1],
			scanline[col * 3 + 2]
		);
	}

	private void assignTerrainTypes()
	{
		foreach(center; centers)
		{
			int row = center.y.to!int;
			int col = center.x.to!int;

			auto pixel = getPixel(row, col);
			center.terrain = colors.get(pixel, Terrain.none);
		}

		foreach(center; centers)
		{
			if (center.terrain != Terrain.sea
				&& center.neighbours.any!(n => n.terrain == Terrain.sea))
			{
				center.shore = true;
				shores ~= center;
			}
		}
	}

	private void computeElevation()
	{
	}

}

private Triangulation generateTriangulation(int width, int height, int res)
{
	Point[] points;
	auto margin = 4 * res;

	for(auto x = -margin; x < width + margin; x += res)
	for(auto y = -margin; y < height + margin; y += res)
		points ~= Point(
			x + uniform(-res, res),
			y + uniform(-res, res)
		);

	return Triangulation(points);
}
