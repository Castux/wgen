import std.random;
import std.algorithm;
import std.array;
import std.math;
import std.conv;
import std.range;
import std.typecons;

import gamut;
import delaunator;
import graph;
import config;

class Center : CenterBase!(Center, Edge, Corner)
{
	Terrain terrain;
	bool shore;

	double z = double.infinity;
	double gradient;

	Center downhill;
	Center[] uphill;
	int flow;

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

class Heightmap : Graph!(Center, Edge, Corner)
{
	Config conf;

	Image outline;
	const(int) width;
	const(int) height;
	const(double) resolution;

	Center[] shores;
	double lowest, highest;

	Corner[][][] spatialIndex;
	const(double) binSize;

	this(Config conf)
	{
		import std.stdio;

		this.conf = conf;

		outline.loadFromFile(conf.path, LOAD_RGB | LOAD_8BIT | LOAD_NO_ALPHA);
		if (outline.isError)
			throw new Exception("Could not load " ~ conf.path);

		this.width = outline.width;
		this.height = outline.height;
		this.resolution = conf.resolution;
		this.binSize = conf.resolution * 4;

		writeln("Triangulating");
		auto triangulation = generateTriangulation();

		writeln("Building graph");
		super(triangulation.edges);

		generate();
	}

	double margin() const
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
		computeElevation((Center c) {return c.gradient;});

		writeln("Computing river flow");
		computeRiverFlow();

		writeln("Eroding");
		erode();
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
				points ~= center.corners.map!"a.p".fold!((a,b) => a + b) / center.corners.length;
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
				center.terrain = null;
				continue;
			}

			int row = center.y.to!int;
			int col = center.x.to!int;

			auto pixel = getPixel(row, col);
			center.terrain = conf.terrains.get(pixel, null);

			auto r = conf.smoothingRadius;
			if (center.terrain.smoothing && r > 0.0)
			{
				auto sum = 0.0;
				auto count = 0;

				auto numSamples = pow(conf.smoothingRadius / conf.resolution, 2).to!int;

				while (count < numSamples)
				{
					Point p = Point(uniform(center.x - r, center.x + r), uniform(center.y - r, center.y + r));

					if (inBounds(p))
					{
						pixel = getPixel(p.y.to!int, p.x.to!int);
						auto terrain = conf.terrains.get(pixel, null);
						if (terrain)
						{
							sum += terrain.gradient;
							count++;
						}
					}
				}

				center.gradient = sum/count;
			}
			else
				center.gradient = center.terrain.gradient;
		}

		foreach(center; centers)
		{
			if (center.terrain !is null
				&& center.terrain.name != "sea"
				&& center.neighbours.any!(n => n.terrain && n.terrain.name == "sea"))
			{
				center.shore = true;
				shores ~= center;
			}
		}
	}

	private void computeElevation(double delegate(Center) gradFunc)
	{
		assert(shores.length > 0);
		Center[] queue;

		foreach(shore; shores)
		{
			shore.z = 0;
			queue ~= shore;
		}

		while (queue.length > 0)
		{
			auto c = queue[0];

			foreach (n; c.neighbours)
			{
				if (n.terrain is null) continue;

				auto gradient = gradFunc(c);
				auto newZ = c.z + gradient * n.p.dist(c.p);
				if (newZ < n.z)
				{
					n.z = newZ;
					queue ~= n;
				}
			}

			queue = queue[1..$];
		}

		lowest = double.infinity;
		highest = -double.infinity;
		centers
			.filter!(a => a.terrain !is null)
			.tee!((c) { lowest = min(lowest, c.z); highest = max(highest, c.z); })
			.filter!(a => a.terrain.name == "sea")
			.each!(c => c.z = -c.z);
	}

	private void computeRiverFlow()
	{
		// Find the steepest downhill from every point

		foreach(center; centers)
		{
			// It needs to be actually downhill (avoid rivers along shores)
			auto lower = center.neighbours.filter!(n => n.z < center.z);

			if (!lower.empty)
			{
				auto lowest = lower.minElement!(n => n.z);
				center.downhill = lowest;
				lowest.uphill ~= center;
			}
		}

		// Go up from the shores to compute flows: each rivers
		// gets the sum of the uphill ones, plus 1 for itself

		int flow(Center center)
		{
			center.flow = center.uphill.map!flow.sum + 1;
			return center.flow;
		}

		shores.each!flow;
	}

	static double linearMap(double a, double b, double u, double v, double x)
	{
		return (x - a) / (b - a) * (v - u) + u;
	}

	void erode()
	{
		centers.each!(c => c.z = double.infinity);
		computeElevation((Center c) {
			return c.terrain.erosion && c.flow >= conf.erosionMinFlow ?
				c.gradient * conf.erosionFactor :
				c.gradient;
		});
	}

	private void indexCorners()
	{
		auto numBinsH = ceil(height / binSize).to!int;
		auto numBinsW = ceil(width / binSize).to!int;

		auto index = new Corner[][][numBinsH];
		foreach(ref row; index)
			row = new Corner[][numBinsW];

		foreach(corner; corners)
		{
			if (!inBounds(corner.p)) continue;

			auto row = floor(corner.y / binSize).to!int;
			auto col = floor(corner.x / binSize).to!int;

			index[row][col] ~= corner;
		}

		spatialIndex = index;
	}

	alias BinResult = Tuple!(Corner,double[3]);

	private static BinResult findTriangleInBin(Point p, Corner[] bin)
	{
		foreach(corner; bin)
		{
			auto bary = barycentricCoordinates(corner.centers[0].p, corner.centers[1].p, corner.centers[2].p, p);
			if (bary[0] >= 0 && bary[1] >= 0 && bary[2] >= 0)
			{
				return tuple(corner, bary);
			}
		}

		return BinResult.init;
	}

	private BinResult findTriangle(Point p)
	{
		if (!inBounds(p)) return BinResult.init;

		auto row = floor(p.y / binSize).to!int;
		auto col = floor(p.x / binSize).to!int;

		// First check the bin itself

		auto res = findTriangleInBin(p, spatialIndex[row][col]);
		if (res[0])
			return res;

		// Then the ones around

		auto numBinsH = spatialIndex.length;
		auto numBinsW = spatialIndex[0].length;

		for(int r = row - 1; r <= row + 1; r++)
		for(int c = col - 1; c <= col + 1; c++)
		{
			if (r == row && c == col) continue;
			if (c < 0 || c >= numBinsW) continue;
			if (r < 0 || r >= numBinsH) continue;

			res = findTriangleInBin(p, spatialIndex[r][c]);
			if (res[0])
				return res;
		}

		return BinResult.init;
	}

	private static double interpolateElevation(BinResult res)
	{
		auto c = res[0];
		auto coords = res[1];

		return
			c.centers[0].z * coords[0] +
			c.centers[1].z * coords[1] +
			c.centers[2].z * coords[2];
	}

	double[][] rasterize()
	{
		import std.stdio;

		indexCorners();

		auto data = new double[][height];
		foreach (ref row; data)
			row = new double[width];

		foreach(row; 0..height)
		foreach(col; 0..width)
		{
			auto p = Point(col, row);
			auto res = findTriangle(p);
			if (res[0])
				data[row][col] = interpolateElevation(res);
		}

		return data;
	}
}
