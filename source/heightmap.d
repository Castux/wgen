import std.random;
import std.algorithm;
import std.array;
import std.math;
import std.conv;
import std.range;
import std.typecons;
import std.stdio;

import dplug.math;

import gamut;
import delaunator;
import config;

alias HalfEdge = delaunator.Edge;
alias Vec2 = vec2d;
alias Vec3 = vec3d;


class Center
{
	Vec3 pos;
	alias pos this;

	Center[] neighbours;
	Edge[] edges;
	Corner[] corners;

	Terrain terrain;
	bool shore;

	double gradient;

	Center downhill;
	Center[] uphill;
	int flow;

	vec3d normal;

	this(Vec2 pos)
	{
		this.pos = Vec3(pos, double.infinity);
	}

	void reset()
	{
		terrain = null;
		shore = false;
		z = double.infinity;
		gradient = 0;
		downhill = null;
		uphill = [];
		flow = 0;
	}
}

class Edge
{
	Center center1, center2;
	Corner corner1, corner2;

	void addCenter(Center c)
	{
		if (center1 is null)
			center1 = c;
		else
		{
			assert(center2 is null);
			center2 = c;
		}
	}

	void addCorner(Corner c)
	{
		if (corner1 is null)
			corner1 = c;
		else
		{
			assert(corner2 is null);
			corner2 = c;
		}
	}
}

class Corner
{
	Vec3 pos;
	alias pos this;

	Corner[] neighbours;
	Edge[] edges;
	Center[] centers;

	Vec3 normal;

	this(Vec2 pos)
	{
		this.pos = Vec3(pos, double.nan);
	}
}

class Heightmap
{
	Config conf;

	Center[] centers;
	Corner[] corners;
	Edge[] edges;

	Image outline;
	int width;
	int height;
	double resolution;

	Center[] shores;
	double lowest, highest;

	Corner[][][] spatialIndex;
	double binSize;

	this(Config conf)
	{
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
		createGraph(triangulation.edges);

		if (conf.relax)
		{
			writeln("Relaxing");
			relaxGraph();
		}

		generate();
	}

	bool updateConfig(Config newConfig)
	{
		if (newConfig.path != conf.path || newConfig.resolution != conf.resolution)
		{
			writeln("Cannot update path or resolution");
			return false;
		}

		if (newConfig.terrains != conf.terrains || newConfig.smoothingRadius != conf.smoothingRadius
			|| newConfig.maxHeight != conf.maxHeight)
		{
			centers.each!(c => c.reset);

			writeln("Assigning terrain types");
			assignTerrainTypes();

			writeln("Computing elevation");
			computeElevation((Center c, Center n) {return c.gradient;});

			writeln("Computing river flow");
			computeRiverFlow();

			writeln("Eroding");
			erode();

			conf = newConfig;
			return true;
		}

		if (newConfig.erosionMinFlow != conf.erosionMinFlow || newConfig.erosionFactor != conf.erosionFactor)
		{
			writeln("Eroding");
			erode();

			conf = newConfig;
			return true;
		}

		if (newConfig.blurRadius != conf.blurRadius)
		{
			conf = newConfig;
			return true;
		}

		return false;
	}

	double margin() const
	{
		return resolution * 4;
	}

	bool inBounds(V)(V p) const
	{
		return p.x >= 0 && p.x < width && p.y >= 0 && p.y < width;
	}

	bool inBoundsPlusHalfMargin(V)(V p) const
	{
		double h = margin / 2.0;
		return p.x > -h && p.x < width + h && p.y > -h && p.y < height + h;
	}

	private void generate()
	{
		import std.stdio;

		writeln("Assigning terrain types");
		assignTerrainTypes();

		writeln("Computing elevation");
		computeElevation((Center c, Center n) {return c.gradient;});

		writeln("Computing river flow");
		computeRiverFlow();

		writeln("Eroding");
		erode();
	}

	private Triangulation generateTriangulation()
	{
		Vec2[] points;

		if (conf.grid == "square")
		{
			for(auto x = -margin; x < width + margin; x += resolution)
			for(auto y = -margin; y < height + margin; y += resolution)
				points ~= Vec2(
					x + uniform(-resolution, resolution) * conf.jitter,
					y + uniform(-resolution, resolution) * conf.jitter
				);
		}
		else if(conf.grid == "hex")
		{
			for(auto x = -margin; x < width + margin; x += resolution)
			{
				auto row = 0;
				for(auto y = -margin; y < height + margin; y += resolution * sqrt(3.0) / 2.0)
				{
					row++;
					points ~= Vec2(
						x + ((row % 2) * 0.5 * resolution) + uniform(-resolution, resolution) * conf.jitter,
						y + uniform(-resolution, resolution) * conf.jitter
					);
				}
			}
		}

		return Triangulation(points);
	}

	private void createGraph(HalfEdge[] halfEdges)
	{
		centers = [];
		corners = [];
		edges = [];

		// Create edges. Associate pairs of opposite half edges
		// to the same full edge

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

		// Create corners at the circumcenter of each triangle
		// Associate them with the three edges of the triangle

		Corner[HalfEdge] cornerMap;
		foreach(he; halfEdges)
		{
			if (he in cornerMap)
				continue;

			auto tri = he.triangle;
			auto c = circumcenter(tri.e1.from, tri.e2.from, tri.e3.from);
			Corner corner = new Corner(c);

			foreach(hedge; tri.tupleof)
			{
				cornerMap[hedge] = corner;

				auto e = edgeMap[hedge];
				corner.edges ~= e;
				e.addCorner(corner);
			}

			corners ~= corner;
		}

		// Create centers for each original point given to triangulate
		// Associate all the edges in their orbits

		Center[HalfEdge] centerMap;
		foreach(he; halfEdges)
		{
			if (he in centerMap)
				continue;

			auto orbit = he.orbit;
			Center center = new Center(he.from);

			foreach(orbitHedge; orbit)
			{
				centerMap[orbitHedge] = center;

				auto e = edgeMap[orbitHedge];
				e.addCenter(center);
				center.edges ~= e;

				// On the hull, we need to also add the one edge comes into
				// this node (but don't put it in the map or it might
				// get skipped for its own center)

				if (orbitHedge.onHull)
				{
					e = edgeMap[orbitHedge.hullPrev];
					e.addCenter(center);
					center.edges ~= e;
				}
			}

			centers ~= center;
		}

		// Connect centers to centers, and corners to corners,
		// via the edges

		foreach(edge; edges)
		{
			assert(edge.center1 && edge.center2);

			// Every edge must connect two centers (they're the original
			// point we triangulated)

			auto c1 = edge.center1;
			auto c2 = edge.center2;

			c1.neighbours ~= c2;
			c2.neighbours ~= c1;

			// but hull edges don't connect corners to anything

			if (edge.corner2 !is null)
			{
				auto co1 = edge.corner1;
				auto co2 = edge.corner2;

				co1.neighbours ~= co2;
				co2.neighbours ~= co1;
			}
		}

		// Finally, connect centers to corners and vice-versa

		foreach(he; halfEdges)
		{
			auto corner = cornerMap[he];
			auto center = centerMap[he];

			corner.centers ~= center;
			center.corners ~= corner;
		}

		// Order clockwise

		import std.algorithm;

		foreach(corner; corners)
			corner.centers.sort!((a,b) => pseudoAngle(a.xy - corner.xy) < pseudoAngle(b.xy - corner.xy));

		foreach(center; centers)
			center.corners.sort!((a,b) => pseudoAngle(a.xy - center.xy) < pseudoAngle(b.xy - center.xy));
	}

	private void relaxGraph()
	{
		Vec2[] points;

		foreach(center; centers)
		{
			if (!inBoundsPlusHalfMargin(center.pos))
				points ~= center.xy;
			else
				points ~= center.corners.map!"a.xy".sum / center.corners.length;
		}

		auto triangulation = Triangulation(points);
		createGraph(triangulation.edges);
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
		import fast_noise;
		FNLState noise = fnlCreateState();
		noise.noise_type = FNLNoiseType.FNL_NOISE_PERLIN;
		noise.frequency = 1.0/200.0;
		noise.fractal_type = FNLFractalType.FNL_FRACTAL_FBM;
		noise.octaves = 3;
		noise.lacunarity = 2;
		noise.gain = 0.5;

		foreach(center; centers)
		{
			if (!inBoundsPlusHalfMargin(center.pos))
			{
				center.terrain = null;
				continue;
			}

			int row = center.y.to!int;
			int col = center.x.to!int;

			auto pixel = getPixel(row, col);
			center.terrain = conf.terrains.get(pixel, null);

			if (center.terrain is null)
			{
				writefln("Bad pixel %s at %d,%d", pixel, col, row);
				center.gradient = 0;
				continue;
			}

			auto r = conf.smoothingRadius;
			if (center.terrain.smoothing && r > 0.0)
			{
				auto sum = center.terrain.gradient;
				auto count = 1;

				auto numSamples = ceil(pow(conf.smoothingRadius / conf.resolution, 2)).to!int;

				foreach(i; 0..numSamples)
				{
					Vec2 p = Vec2(uniform(center.x - r, center.x + r), uniform(center.y - r, center.y + r));

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

			// if (center.terrain.name != "sea")
			// 	center.gradient = pow((fnlGetNoise2D(&noise, center.x, center.y) + 1.0) / 2.0, 1.0);
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

	private void computeElevation(double delegate(Center, Center) gradFunc)
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

				auto gradient = gradFunc(c, n);
				auto newZ = c.z + gradient * n.xy.distanceTo(c.xy);
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

		if (conf.maxHeight != 0.0)
		{
			centers.each!(c => c.z = c.z / highest * conf.maxHeight);
			lowest = lowest / highest * conf.maxHeight;
			highest = conf.maxHeight;
		}

		foreach(corner; corners)
			corner.normal = cross(corner.centers[1].pos - corner.centers[0].pos, corner.centers[2].pos - corner.centers[0].pos).normalized;

		foreach(center; centers)
			center.normal = center.corners.map!"a.normal".sum.normalized;
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
		computeElevation((Center c, Center n) {
			return c.terrain.erosion && n.downhill is c && c.flow > conf.erosionMinFlow ?
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
			if (!inBounds(corner.pos)) continue;

			auto row = floor(corner.y / binSize).to!int;
			auto col = floor(corner.x / binSize).to!int;

			index[row][col] ~= corner;
		}

		spatialIndex = index;
	}

	alias BinResult = Tuple!(Corner,double[3]);

	private static BinResult findTriangleInBin(Vec2 p, Corner[] bin)
	{
		foreach(corner; bin)
		{
			auto bary = barycentricCoordinates(corner.centers[0].xy, corner.centers[1].xy, corner.centers[2].xy, p);
			if (bary[0] >= 0 && bary[1] >= 0 && bary[2] >= 0)
			{
				return tuple(corner, bary);
			}
		}

		return BinResult.init;
	}

	private BinResult findTriangle(Vec2 p)
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
			auto p = Vec2(col, row);
			auto res = findTriangle(p);
			if (res[0])
				data[row][col] = interpolateElevation(res);
		}

		return data;
	}
}
