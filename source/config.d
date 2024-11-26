import std.json;
import std.stdio;
import std.file;

struct Pixel
{
	ubyte r,g,b;
}

class Terrain
{
	string name;
	Pixel color;
	double gradient;
	double fixedShore;
	bool smoothing = true;
	bool erosion = true;

	override bool opEquals(Object other)
	{
		if (typeid(this) != typeid(other)) return false;
		auto o = cast(Terrain) other;

		return name == o.name &&
			color == o.color &&
			gradient == o.gradient &&
			smoothing == o.smoothing &&
			erosion == o.erosion;
	}
}

class Config
{
	string path;
	double resolution;
	string grid;
	double jitter;
	bool relax;
	Terrain[Pixel] terrains;
	double smoothingRadius;
	int erosionMinFlow;
	double erosionFactor;
	double maxHeight = 0;
	int blurRadius;

	bool exportOBJ;
	bool exportSVG;
	bool png16;

	this(string jsonPath)
	{
		auto txt = readText(jsonPath);
		auto json = parseJSON(txt);

		path = json["path"].get!string;
		resolution = json["resolution"].get!double;
		grid = json["grid"].get!string;
		jitter = json["jitter"].get!double;
		relax = json["relax"].get!bool;
		smoothingRadius = json["smoothingRadius"].get!double;
		erosionMinFlow = json["erosionMinFlow"].get!int;
		erosionFactor = json["erosionFactor"].get!double;

		if ("maxHeight" in json)
			maxHeight = json["maxHeight"].get!double;

		if ("blurRadius" in json)
			blurRadius = json["blurRadius"].get!int;

		if ("exportOBJ" in json)
			exportOBJ = json["exportOBJ"].get!bool;

		if ("exportSVG" in json)
			exportSVG = json["exportSVG"].get!bool;

		if ("png16" in json)
		png16 = json["png16"].get!bool;


		foreach(string name, t; json["terrains"])
		{
			auto terrain = new Terrain();
			terrain.name = name;
			terrain.color = Pixel(t["r"].get!ubyte, t["g"].get!ubyte, t["b"].get!ubyte);
			terrain.gradient = t["gradient"].get!double;

			if ("fixedShore" in t)
				terrain.fixedShore = t["fixedShore"].get!double;

			if ("smoothing" in t)
				terrain.smoothing = t["smoothing"].get!bool;

			if ("erosion" in t)
				terrain.erosion = t["erosion"].get!bool;

			terrains[terrain.color] = terrain;
		}
	}
}
