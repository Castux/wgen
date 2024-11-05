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
	Terrain[Pixel] terrains;
	double smoothingRadius;
	int erosionMinFlow;
	double erosionFactor;

	this(string jsonPath)
	{
		auto txt = readText(jsonPath);
		auto json = parseJSON(txt);

		try
		{
		path = json["path"].get!string;
		resolution = json["resolution"].get!double;
		smoothingRadius = json["smoothingRadius"].get!double;
		erosionMinFlow = json["erosionMinFlow"].get!int;
		erosionFactor = json["erosionFactor"].get!double;

		foreach(string name, t; json["terrains"])
		{
			auto terrain = new Terrain();
			terrain.name = name;
			terrain.color = Pixel(t["r"].get!ubyte, t["g"].get!ubyte, t["b"].get!ubyte);
			terrain.gradient = t["gradient"].get!double;

			if ("smoothing" in t)
				terrain.smoothing = t["smoothing"].get!bool;

			if ("erosion" in t)
				terrain.smoothing = t["erosion"].get!bool;

			terrains[terrain.color] = terrain;
		}

		}

		catch(Exception e)
		{
			writeln(jsonPath);
			writeln(txt);
			writeln(e);
		}
	}
}
