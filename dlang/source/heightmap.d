import std.random;
import std.algorithm;
import std.array;
import std.math;
import std.conv;
import std.range;

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

const double[Terrain] gradients =
[
	Terrain.sea: 0.5/3,
	Terrain.plains: 1.0/3,
	Terrain.hills: 2.0/3,
	Terrain.mountains: 4.0/3,
	Terrain.lake: 0.001/3,
	Terrain.flat: 0.2/3,
	Terrain.cliffs: 8.0/3
];

const smoothing = 0.5;

class Center : CenterBase!(Center, Edge, Corner)
{
	Terrain terrain;
	bool shore;

	double z = double.infinity;
	double gradient;

	this(Point p)
	{
		 super(p);
	}
}

class Edge : EdgeBase!(Center, Edge, Corner)
{
	this()
	{
	}
}

class Corner : CornerBase!(Center, Edge, Corner)
{
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
	double lowest, highest;

	this(string path, int resolution)
	{
		import std.stdio;

		outline.loadFromFile(path, LOAD_RGB | LOAD_8BIT | LOAD_NO_ALPHA);
		if (outline.isError)
			throw new Exception("Could not load " ~ path);

		this.width = outline.width;
		this.height = outline.height;
		this.resolution = resolution;

		writeln("Triangulating");
		auto triangulation = generateTriangulation();

		writeln("Building graph");
		super(triangulation.edges);

		generate();
	}

	int margin() const
	{
		return resolution * 4;
	}

	bool inBounds(Point p) const
	{
		return p.x >= 0 && p.x < width && p.y >= 0 && p.y < width;
	}

	bool inBoundsPlusHalfMargin(Point p) const
	{
		double h = margin / 2.0;
		return p.x > -h && p.x < width + h && p.y > -h && p.y < height + h;
	}

	private void generate()
	{
		import std.stdio;

		writeln("Relaxing");
		relaxGraph();

		writeln("Assigning terrain types");
		assignTerrainTypes();

		writeln("Computing elevation");
		computeElevation();
	}

	private Triangulation generateTriangulation()
	{
		Point[] points;

		for(auto x = -margin; x < width + margin; x += resolution)
		for(auto y = -margin; y < height + margin; y += resolution)
			points ~= Point(
				x + uniform(-resolution, resolution),
				y + uniform(-resolution, resolution)
			);

		return Triangulation(points);
	}

	private void relaxGraph()
	{
		Point[] points;

		foreach(center; centers)
		{
			if (!inBoundsPlusHalfMargin(center.p))
				points ~= center.p;
			else
				points ~= center.neighbours.map!"a.p".fold!((a,b) => a + b) / center.neighbours.length;
		}

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
			if (!inBoundsPlusHalfMargin(center.p))
			{
				center.terrain = Terrain.none;
				continue;
			}

			int row = center.y.to!int;
			int col = center.x.to!int;

			auto pixel = getPixel(row, col);
			center.terrain = colors.get(pixel, Terrain.none);
		}

		foreach(center; centers)
		{
			if (center.terrain != Terrain.sea
				&& center.terrain != Terrain.none
				&& center.neighbours.any!(n => n.terrain == Terrain.sea))
			{
				center.shore = true;
				shores ~= center;
			}
		}
	}

	private void computeElevation()
	{
		assert(shores.length > 0);
		Center[] queue;

		foreach(shore; shores)
		{
			shore.z = 0;
			shore.gradient = gradients[shore.terrain];
			queue ~= shore;
		}

		while (queue.length > 0)
		{
			auto c = queue[0];

			foreach (n; c.neighbours)
			{
				if (n.terrain == Terrain.none) continue;

				double gradient = gradients[n.terrain];

				if (n.terrain != Terrain.lake)
					gradient = gradient * (1 - smoothing) + c.gradient * smoothing;

				assert(gradient > 0);

				auto newZ = c.z + gradient * resolution;
				if (newZ < n.z)
				{
					n.gradient = gradient;
					n.z = newZ;
					queue ~= n;
				}
			}

			queue = queue[1..$];
		}

		lowest = double.infinity;
		highest = -double.infinity;
		centers
			.filter!(a => a.terrain != Terrain.none)
			.tee!((c) { lowest = min(lowest, c.z); highest = max(highest, c.z); })
			.filter!(a => a.terrain == Terrain.sea)
			.each!(c => c.z = -c.z);
	}

}
