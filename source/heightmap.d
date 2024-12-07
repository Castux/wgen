import std.random;
import std.algorithm;
import std.array;
import std.math;
import std.conv;
import std.range;
import std.typecons;
import std.stdio;
import std.parallelism;

import fswatch;
import dplug.math;
import gamut;

import delaunator;
import config;
import utils;

alias HalfEdge = delaunator.Edge;

class Vertex
{
	vec3d pos;
	alias pos this;

	Vertex[] neighbours;
	Edge[] edges;
	Triangle[] triangles;

	Terrain terrain;
	bool shore;

	double gradient;
	double waterLevel;

	Vertex downhill;
	Vertex[] uphill;
	int flow;

	this(vec2d pos)
	{
		this.pos = vec3d(pos, double.nan);
	}

	void reset()
	{
		terrain = null;
		shore = false;
		z = double.init;
		gradient = double.init;
		waterLevel = double.init;
		downhill = null;
		uphill = [];
		flow = 0;
	}

	bool isWater() const
	{
		return terrain !is null && gradient < 0;
	}

	bool isLand() const
	{
		return terrain !is null && gradient >= 0;
	}

	bool isLake() const
	{
		return isWater && terrain.fixedShore.isNaN;
	}

	bool isSea() const
	{
		return isWater && !terrain.fixedShore.isNaN;
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
	vec3d pos;
	alias pos this;

	Triangle[] neighbours;
	Edge[] edges;
	Vertex[] vertices;

	vec3d normal;

	this(vec2d pos)
	{
		this.pos = vec3d(pos, double.nan);
	}

	void reset()
	{
		z = double.infinity;
		normal = vec3d();
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

	vec3d[] outline;
	int width;
	int height;

	Vertex[] shores;
	double lowest, highest;

	double[] heightmap;
	double[] waterLevel;

	bool interactive;
	int changeCount;
	bool loading;

	this(string path)
	{
		loadConfig(path);
	}

	private void loadConfig(string path)
	{
		writeln("Loading ", path);

		auto config = new Config(path);
		updateConfig(config);

		configWatcher = FileWatch(path);
		imageWatcher = FileWatch(config.path);
	}

	void checkConfigUpdate()
	{
		loading = true;

		foreach (event; configWatcher.getEvents())
		if (event.type == FileChangeEventType.modify)
		{
			writeln("=================");
			loadConfig(event.path);
		}

		foreach (event; imageWatcher.getEvents())
		if (event.type == FileChangeEventType.modify)
		{
			int oldWidth = width;
			int oldHeight = height;

			writeln("=================");
			loadOutline();

			auto sameSize = oldWidth == width && oldHeight == height;
			updateConfig(conf, skipImageLoad: true, skipMesh: sameSize);
		}

		loading = false;
	}

	private enum Phase
	{
		NewImage,
		NewMesh,
		NewTerrain,
		NewErosion,
		Rasterize,
		None
	}

	private void loadOutline()
	{
		writeln("Loading " ~ conf.path);

		Image image;
		image.loadFromFile(conf.path, LOAD_RGB | LOAD_8BIT | LOAD_NO_ALPHA);
		if (image.isError)
			throw new Exception("Could not load " ~ conf.path);
		image.flipVertical();

		width = image.width;
		height = image.height;

		outline = new vec3d[width * height];

		foreach(row; 0 .. height)
		{
			auto scanline = cast(ubyte[]) image.scanline(row);
			foreach(col; 0 .. width)
			{
				outline[row * image.width + col] = vec3d(
					scanline[col * 3 + 0],
					scanline[col * 3 + 1],
					scanline[col * 3 + 2]
				);
			}
		}
	}

	private void updateConfig(Config newConf, bool skipImageLoad = false, bool skipMesh = false)
	{
		auto old = conf;
		conf = newConf;

		Phase phase = Phase.None;
		Triangulation triangulation;

		if (old is null ||
			old is conf ||
			conf.path != old.path)
			phase = Phase.NewImage;

		else if (conf.resolution != old.resolution ||
			conf.relax != old.relax ||
			conf.grid != old.grid ||
			conf.jitter != old.jitter)
			phase = Phase.NewMesh;

		else if (conf.terrains != old.terrains ||
			conf.smoothingRadius != old.smoothingRadius ||
			conf.maxHeight != old.maxHeight)
			phase = Phase.NewTerrain;

		else if (conf.erosionMinFlow != old.erosionMinFlow ||
			conf.erosionFactor != old.erosionFactor)
			phase = Phase.NewErosion;

		else if (conf.blurRadius != old.blurRadius)
			phase = Phase.Rasterize;

		if (phase <= Phase.NewImage && !skipImageLoad)
		{
			loadOutline();
		}

		if (phase <= Phase.NewMesh)
		{
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
		}

		if (phase <= Phase.NewTerrain)
		{
			vertices.each!(v => v.reset);
			triangles.each!(t => t.reset);
			shores = [];

			writeln("Assigning terrain types");
			assignTerrainTypes();

			writeln("Computing elevation");
			computeElevation();

			writeln("Computing river flow");
			computeRiverFlow();
		}

		if (phase <= Phase.NewErosion)
		{
			writeln("Eroding");
			computeElevation(erosionPass: true);

			writeln("Computing water depth");
			computeWaterDepth();
			finalizeElevation();
			writefln("Range %f %f", lowest, highest);
		}

		if (phase <= Phase.Rasterize)
		{
			writeln("Rasterizing");
			rasterize();

			if (conf.blurRadius > 0)
			{
				writeln("Applying blur");
				blurHeightmap();
			}
		}

		changeCount++;
	}

	double margin() const
	{
		return conf.resolution * 4;
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
		vec2d[] points;
		auto resolution = conf.resolution;

		if (conf.grid == "square")
		{
			for(auto x = -margin; x < width + margin; x += resolution)
			for(auto y = -margin; y < height + margin; y += resolution)
				points ~= vec2d(
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
					points ~= vec2d(
						x + ((row % 2) * 0.5 * resolution) + uniform(-resolution, resolution) * conf.jitter,
						y + uniform(-resolution, resolution) * conf.jitter
					);
				}
			}
		}
		else
			throw new Exception("Unknown grid type: " ~ conf.grid);

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
		vec2d[] points;

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

			vec3d pixel = safeGet(outline, width, height, vertex.xy);
			vertex.terrain = conf.terrains.get(pixel, null);

			if (vertex.terrain is null)
			{
				writefln("Bad pixel %s at %d,%d", pixel.toString, vertex.y.to!int, vertex.x.to!int);
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
					vec2d p = vertex.xy + vec2d(uniform(-r, r), uniform(-r, +r));

					if (inBounds(p))
					{
						pixel = safeGet(outline, width, height, p);
						auto terrain = conf.terrains.get(pixel, null);
						if (terrain && terrain.gradient * vertex.terrain.gradient > 0)		// Only consider same sign gradients
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

		if (!shores.any!"a.isSea")
			throw new Exception("No sea defined");
	}

	private void computeElevation(bool erosionPass = false)
	{
		assert(shores.length > 0);
		Vertex[] queue;

		if (erosionPass)
			vertices.each!(v => v.z = double.nan);

		foreach(shore; shores)
		{
			if (shore.isSea)
			{
				shore.z = shore.terrain.fixedShore;
				queue ~= shore;
			}
		}

		while (queue.length > 0)
		{
			auto c = queue[0];

			foreach (n; c.neighbours)
			{
				if (n.terrain is null || n.isSea) continue;

				auto gradient = n.gradient;

				if (n.isLake)
					gradient = 0.00001;

				if (erosionPass && n.terrain.erosion && n.downhill is c && n.flow > conf.erosionMinFlow)
					gradient *= conf.erosionFactor;

				auto newZ = c.z + gradient * c.xy.distanceTo(n.xy);
				if (n.z.isNaN || newZ < n.z)
				{
					n.z = newZ;
					queue ~= n;
				}
			}

			queue = queue[1..$];
		}
	}

	private void computeWaterDepth()
	{
		Vertex[] queue = shores.dup;

		vertices.filter!"a.isWater && !a.shore".each!(v => v.z = double.nan);

		while (queue.length > 0)
		{
			auto c = queue[0];

			if (c.isSea)
				c.waterLevel = c.terrain.fixedShore;
			else if (c.isLake && c.shore)
				c.waterLevel = c.z;

			foreach (n; c.neighbours)
			{
				if (n.terrain is null || n.isLand) continue;

				auto newZ = c.z + n.gradient * c.xy.distanceTo(n.xy);
				if (n.z.isNaN || newZ > n.z)
				{
					n.z = newZ;

					if (n.isLake)
						n.waterLevel = c.waterLevel;

					queue ~= n;
				}
			}

			queue = queue[1..$];
		}
	}

	private void finalizeElevation()
	{
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

		vertices.filter!"a.downhill is null".each!flow;
	}

	static double linearMap(double a, double b, double u, double v, double x)
	{
		return (x - a) / (b - a) * (v - u) + u;
	}

	static double cross2d(vec2d a, vec2d b) pure
	{
		return a.x * b.y - a.y * b.x;
	}

	static vec3d barycentricCoordinates(vec2d a, vec2d b, vec2d c, vec2d p) pure
	{
		auto x = cross2d(b - p, c - p);
		auto y = cross2d(c - p, a - p);
		auto z = cross2d(a - p, b - p);
		auto s = x + y + z;

		return vec3d(x / s, y / s, z / s);
	}

	private void rasterize()
	{
		auto heightmap = new double[height * width];
		auto waterLevel = new double[height * width];

		foreach(tri; triangles.parallel)
		{
			auto p0 = tri.vertices[0];
			auto p1 = tri.vertices[1];
			auto p2 = tri.vertices[2];

			auto z = vec3d(p0.z, p1.z, p2.z);
			auto water = tri.vertices.all!"a.isWater" ? tri.vertices[0].waterLevel : lowest;

			auto minx = min(p0.x, p1.x, p2.x).floor.to!int;
			auto miny = min(p0.y, p1.y, p2.y).floor.to!int;
			auto maxx = max(p0.x, p1.x, p2.x).ceil.to!int;
			auto maxy = max(p0.y, p1.y, p2.y).ceil.to!int;

			foreach(x; minx .. maxx + 1)
			foreach(y; miny .. maxy + 1)
			{
				if (x < 0 || x >= width || y < 0 || y >= height) continue;

				auto coords = barycentricCoordinates(p0.xy, p1.xy, p2.xy, vec2d(x, y));
				if (coords.x >= 0 && coords.y >= 0 && coords.z >= 0)
				{
					heightmap[y * width + x] = dot(z, coords);
					waterLevel[y * width + x] = water;
				}
			}
		}

		this.heightmap = heightmap;
		this.waterLevel = waterLevel;
	}

	private static int[] binomialCoefs(int order) pure
	{
		int[] coefs = [1];
		foreach(k; 1 .. order + 1)
			coefs ~= coefs[$ - 1] * (order + 1 - k) / k;

		return coefs;
	}

	private void blurHeightmap()
	{
		const radius = conf.blurRadius;
		if (radius == 0)
			return;

		auto tmp = new double[heightmap.length];
		auto output = new double[heightmap.length];

		auto coefs = binomialCoefs(2 * radius)[radius .. $];

		foreach(row; iota(0, height).array.parallel)
		foreach(col; 0 .. width)
		{
			double sum = 0.0;
			int coefsum = 0;

			foreach(dcol; -radius .. radius + 1)
			{
				auto c = col + dcol;
				if (c < 0 || c >= width) continue;
				sum += heightmap[row * width + c] * coefs[dcol.abs];
				coefsum += coefs[dcol.abs];
			}

			tmp[row * width + col] = sum / coefsum;
		}

		foreach(col; iota(0, width).array.parallel)
		foreach(row; 0 .. height)
		{
			double sum = 0.0;
			int coefsum = 0;

			foreach(drow; -radius .. radius + 1)
			{
				auto r = row + drow;
				if (r < 0 || r >= height) continue;
				sum += tmp[r * width + col] * coefs[drow.abs];
				coefsum += coefs[drow.abs];
			}

			output[row * width + col] = sum / coefsum;
		}

		heightmap = output;
	}

	private void writeImage(T)(double[] data, string path)
	{
		import gamut;
		auto PT = (typeid(T) == typeid(ushort)) ? PixelType.l16 : PixelType.l8;

		writefln("Exporting %d bits heightmap to %s", T.sizeof * 8, path);

		T[] output = new T[data.length];
		bool outOfBounds;
		float outOfBoundsValue;

		foreach(i, v; data)
		{
			if (v.isNaN)
			{
				outOfBounds = true;
				outOfBoundsValue = v;
				v = 0.0;
			}

			auto tmp = v.floor;
			if (tmp < 0 || tmp > T.max)
			{
				outOfBounds = true;
				outOfBoundsValue = tmp;
			}
			tmp = tmp.clamp(0, T.max);
			output[i] = tmp.to!T;
		}

		writefln("Warning, some values out of bounds: %f", outOfBoundsValue);
		writefln("Range: %d, %d", output.minElement, output.maxElement);

		Image image;
		image.createView(output.ptr, width, height, PT, width * T.sizeof.to!int);
		image.flipVertical();
		image.saveToFile(path);
	}

	void exportHeightmaps(string heightmapPath, string waterLevelPath)
	{
		if (conf.png16)
		{
			writeImage!ushort(heightmap, heightmapPath);
			writeImage!ushort(waterLevel, waterLevelPath);
		}
		else
		{
			writeImage!ubyte(heightmap, heightmapPath);
			writeImage!ubyte(waterLevel, waterLevelPath);
		}
	}
}
