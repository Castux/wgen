import std.random;
import std.algorithm;
import std.array;
import std.math;
import std.conv;
import std.range;
import std.typecons;
import std.stdio;

import fswatch;
import dplug.math;
import gamut;

import delaunator;
import config;

alias HalfEdge = delaunator.Edge;
alias Vec2 = vec2d;
alias Vec3 = vec3d;

class Vertex
{
	Vec3 pos;
	alias pos this;

	Vertex[] neighbours;
	Edge[] edges;
	Triangle[] triangles;

	Terrain terrain;
	bool shore;

	double gradient;

	Vertex downhill;
	Vertex[] uphill;
	int flow;

	Vec3 normal;

	this(Vec2 pos)
	{
		this.pos = Vec3(pos, double.nan);
	}

	void reset()
	{
		terrain = null;
		shore = false;
		z = double.nan;
		gradient = double.nan;
		downhill = null;
		uphill = [];
		flow = 0;
		normal = Vec3();
	}

	bool isWater()
	{
		return terrain !is null && gradient < 0;
	}

	bool isLand()
	{
		return terrain !is null && gradient >= 0;
	}
}

class Edge
{
	Vertex vertex1, vertex2;
	Triangle triangle1, triangle2;

	void addVertex(Vertex c)
	{
		if (vertex1 is null)
			vertex1 = c;
		else
		{
			assert(vertex2 is null);
			vertex2 = c;
		}
	}

	void addTriangle(Triangle c)
	{
		if (triangle1 is null)
			triangle1 = c;
		else
		{
			assert(triangle2 is null);
			triangle2 = c;
		}
	}
}

class Triangle
{
	Vec3 pos;
	alias pos this;

	Triangle[] neighbours;
	Edge[] edges;
	Vertex[] vertices;

	Vec3 normal;

	this(Vec2 pos)
	{
		this.pos = Vec3(pos, double.nan);
	}

	void reset()
	{
		z = double.infinity;
		normal = Vec3();
	}
}

class Heightmap
{
	Config conf;
	FileWatch configWatcher;
	FileWatch imageWatcher;

	Vertex[] vertices;
	Triangle[] triangles;
	Edge[] edges;

	Image outline;
	int width;
	int height;
	double resolution;

	Vertex[] shores;
	double lowest, highest;

	Triangle[][][] spatialIndex;
	double binSize;

	this(string path)
	{
		loadConfig(path);
	}

	private void loadConfig(string path)
	{
		auto config = new Config(path);
		updateConfig(config);

		configWatcher = FileWatch(path);
		imageWatcher = FileWatch(config.path);
	}

	bool checkConfigUpdate()
	{
		foreach (event; configWatcher.getEvents())
		if (event.type == FileChangeEventType.modify)
		{
			loadConfig(event.path);
			return true;
		}

		foreach (event; imageWatcher.getEvents())
		if (event.type == FileChangeEventType.modify)
		{
			int oldWidth = outline.width;
			int oldHeight = outline.height;

			outline.loadFromFile(conf.path, LOAD_RGB | LOAD_8BIT | LOAD_NO_ALPHA);
			if (outline.isError)
				throw new Exception("Could not load " ~ conf.path);
			outline.flipVertical();

			auto sameSize = oldWidth == outline.width && oldHeight == outline.height;
			updateConfig(conf, skipImageLoad: true, skipMesh: sameSize);

			return true;
		}

		return false;
	}

	private void updateConfig(Config newConf, bool skipImageLoad = false, bool skipMesh = false)
	{
		auto old = conf;
		conf = newConf;

		Triangulation triangulation;

		if (old is null ||
			old is conf ||
			conf.path != old.path ||
			conf.resolution != old.resolution ||
			conf.relax != old.relax ||
			conf.grid != old.grid ||
			conf.jitter != old.jitter)
			goto NewMesh;

		if (conf.terrains != old.terrains ||
			conf.smoothingRadius != old.smoothingRadius ||
			conf.maxHeight != old.maxHeight)
			goto NewTerrain;

		if (conf.erosionMinFlow != old.erosionMinFlow ||
			conf.erosionFactor != old.erosionFactor)
			goto NewErosion;

		if (conf.blurRadius != old.blurRadius)
			goto Default;

		NewMesh:

		if (!skipImageLoad)
		{
			writeln("Loading " ~ conf.path);
			outline.loadFromFile(conf.path, LOAD_RGB | LOAD_8BIT | LOAD_NO_ALPHA);
			if (outline.isError)
				throw new Exception("Could not load " ~ conf.path);
			outline.flipVertical();
		}

		width = outline.width;
		height = outline.height;
		resolution = conf.resolution;
		binSize = conf.resolution * 4;

		if (!skipMesh)
		{
			writeln("Triangulating");
			triangulation = generateTriangulation();

			writeln("Building graph");
			createGraph(triangulation.edges);

			if (conf.relax)
			{
				writeln("Relaxing");
				relaxGraph();
			}
		}

		NewTerrain:

		vertices.each!(v => v.reset);
		triangles.each!(t => t.reset);
		shores = [];

		writeln("Assigning terrain types");
		assignTerrainTypes();

		writeln("Computing elevation");
		computeElevation();

		writeln("Computing river flow");
		computeRiverFlow();

		NewErosion:

		writeln("Eroding");
		computeElevation(erosionPass: true);
		writefln("Range %f %f", lowest, highest);

		Default:
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

		return new Triangulation(points);
	}

	private void createGraph(HalfEdge[] halfEdges)
	{
		vertices = [];
		triangles = [];
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

		// Create triangles at the circumvertex of each triangle
		// Associate them with the three edges of the triangle

		Triangle[HalfEdge] triangleMap;
		foreach(he; halfEdges)
		{
			if (he in triangleMap)
				continue;

			auto tri = he.triangle;
			auto c = circumcenter(tri.e1.from, tri.e2.from, tri.e3.from);
			Triangle triangle = new Triangle(c);

			foreach(hedge; tri.tupleof)
			{
				triangleMap[hedge] = triangle;

				auto e = edgeMap[hedge];
				triangle.edges ~= e;
				e.addTriangle(triangle);
			}

			triangles ~= triangle;
		}

		// Create vertices for each original point given to triangulate
		// Associate all the edges in their orbits

		Vertex[HalfEdge] vertexMap;
		foreach(he; halfEdges)
		{
			if (he in vertexMap)
				continue;

			auto orbit = he.orbit;
			Vertex vertex = new Vertex(he.from);

			foreach(orbitHedge; orbit)
			{
				vertexMap[orbitHedge] = vertex;

				auto e = edgeMap[orbitHedge];
				e.addVertex(vertex);
				vertex.edges ~= e;

				// On the hull, we need to also add the one edge comes into
				// this node (but don't put it in the map or it might
				// get skipped for its own vertex)

				if (orbitHedge.onHull)
				{
					e = edgeMap[orbitHedge.hullPrev];
					e.addVertex(vertex);
					vertex.edges ~= e;
				}
			}

			vertices ~= vertex;
		}

		// Connect vertices to vertices, and triangles to triangles,
		// via the edges

		foreach(edge; edges)
		{
			assert(edge.vertex1 && edge.vertex2);

			// Every edge must connect two vertices (they're the original
			// point we triangulated)

			auto c1 = edge.vertex1;
			auto c2 = edge.vertex2;

			c1.neighbours ~= c2;
			c2.neighbours ~= c1;

			// but hull edges don't connect triangles to anything

			if (edge.triangle2 !is null)
			{
				auto co1 = edge.triangle1;
				auto co2 = edge.triangle2;

				co1.neighbours ~= co2;
				co2.neighbours ~= co1;
			}
		}

		// Finally, connect vertices to triangles and vice-versa

		foreach(he; halfEdges)
		{
			auto triangle = triangleMap[he];
			auto vertex = vertexMap[he];

			triangle.vertices ~= vertex;
			vertex.triangles ~= triangle;
		}

		// Order counterclockwise

		foreach(triangle; triangles)
			triangle.vertices.sort!((a,b) => pseudoAngle(a.xy - triangle.xy) < pseudoAngle(b.xy - triangle.xy));

		foreach(vertex; vertices)
			vertex.triangles.sort!((a,b) => pseudoAngle(a.xy - vertex.xy) < pseudoAngle(b.xy - vertex.xy));
	}

	private void relaxGraph()
	{
		Vec2[] points;

		foreach(vertex; vertices)
		{
			if (!inBoundsPlusHalfMargin(vertex.pos))
				points ~= vertex.xy;
			else
				points ~= vertex.triangles.map!"a.xy".sum / vertex.triangles.length;
		}

		auto triangulation = new Triangulation(points);
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

		foreach(vertex; vertices)
		{
			if (!inBoundsPlusHalfMargin(vertex.pos))
			{
				vertex.terrain = null;
				continue;
			}

			int row = vertex.y.to!int;
			int col = vertex.x.to!int;

			auto pixel = getPixel(row, col);
			vertex.terrain = conf.terrains.get(pixel, null);

			if (vertex.terrain is null)
			{
				writefln("Bad pixel %s at %d,%d", pixel, col, row);
				continue;
			}

			auto r = conf.smoothingRadius;
			if (vertex.terrain.smoothing && r > 0.0)
			{
				auto sum = vertex.terrain.gradient;
				auto count = 1;

				auto numSamples = ceil(pow(conf.smoothingRadius / conf.resolution, 2)).to!int;

				foreach(i; 0 .. numSamples)
				{
					Vec2 p = vertex.xy + Vec2(uniform(-r, r), uniform(-r, +r));

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

				vertex.gradient = sum/count;
			}
			else
				vertex.gradient = vertex.terrain.gradient;

			// if (vertex.terrain.name != "sea")
			// 	vertex.gradient = pow((fnlGetNoise2D(&noise, vertex.x, vertex.y) + 1.0) / 2.0, 1.0);
		}

		foreach (vertex; vertices)
		{
			if (vertex.isWater && vertex.neighbours.any!"a.isLand")
			{
				vertex.shore = true;
				shores ~= vertex;
			}
		}
	}

	private void computeElevation(bool erosionPass = false)
	{
		assert(shores.length > 0);
		Vertex[] queue;

		if (erosionPass)
			vertices.each!(v => v.z = double.nan);

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
				if (n.terrain is null || n.shore) continue;

				auto gradient = n.gradient;

				if (erosionPass && n.terrain.erosion && n.downhill is c && n.flow > conf.erosionMinFlow)
					gradient *= conf.erosionFactor;

				auto newZ = c.z + gradient * c.xy.distanceTo(n.xy);
				if (n.z.isNaN || gradient > 0 && newZ < n.z || gradient < 0 && newZ > n.z)
				{
					n.z = newZ;
					queue ~= n;
				}
			}

			queue = queue[1..$];
		}

		lowest = double.infinity;
		highest = -double.infinity;

		foreach (vertex; vertices)
		{
			if (vertex.terrain is null) continue;

			lowest = min(lowest, vertex.z);
			highest = max(highest, vertex.z);
		}

		if (conf.maxHeight != 0.0)
		{
			vertices.each!(c => c.z = c.z / highest * conf.maxHeight);
			lowest = lowest / highest * conf.maxHeight;
			highest = conf.maxHeight;
		}

		foreach(triangle; triangles)
			triangle.normal = cross(triangle.vertices[1].pos - triangle.vertices[0].pos, triangle.vertices[2].pos - triangle.vertices[0].pos).normalized;

		foreach(vertex; vertices)
			vertex.normal = vertex.triangles.map!"a.normal".sum.normalized;
	}

	private void computeRiverFlow()
	{
		// Find the steepest downhill from every point

		foreach(vertex; vertices)
		{
			// It needs to be actually downhill (avoid rivers along shores)
			auto lower = vertex.neighbours.filter!(n => n.z < vertex.z);

			if (!lower.empty)
			{
				auto lowest = lower.minElement!(n => n.z);
				vertex.downhill = lowest;
				lowest.uphill ~= vertex;
			}
		}

		// Go up from the shores to compute flows: each rivers
		// gets the sum of the uphill ones, plus 1 for itself

		int flow(Vertex vertex)
		{
			vertex.flow = vertex.uphill.map!flow.sum + 1;
			return vertex.flow;
		}

		shores.each!flow;
	}

	static double linearMap(double a, double b, double u, double v, double x)
	{
		return (x - a) / (b - a) * (v - u) + u;
	}

	private void indexTriangles()
	{
		auto numBinsH = ceil(height / binSize).to!int;
		auto numBinsW = ceil(width / binSize).to!int;

		auto index = new Triangle[][][numBinsH];
		foreach(ref row; index)
			row = new Triangle[][numBinsW];

		foreach(triangle; triangles)
		{
			if (!inBounds(triangle.pos)) continue;

			auto row = floor(triangle.y / binSize).to!int;
			auto col = floor(triangle.x / binSize).to!int;

			index[row][col] ~= triangle;
		}

		spatialIndex = index;
	}

	alias BinResult = Tuple!(Triangle,double[3]);

	private static BinResult findTriangleInBin(Vec2 p, Triangle[] bin)
	{
		foreach(triangle; bin)
		{
			auto bary = barycentricCoordinates(triangle.vertices[0].xy, triangle.vertices[1].xy, triangle.vertices[2].xy, p);
			if (bary[0] >= 0 && bary[1] >= 0 && bary[2] >= 0)
			{
				return tuple(triangle, bary);
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
			c.vertices[0].z * coords[0] +
			c.vertices[1].z * coords[1] +
			c.vertices[2].z * coords[2];
	}

	double[][] rasterize()
	{
		import std.stdio;

		indexTriangles();

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
