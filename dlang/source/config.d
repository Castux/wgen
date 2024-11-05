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
}

class Config
{
	string path;
	int resolution;
	Terrain[Pixel] terrains;
	double smoothingRadius;
	int erosionMinFlow;

	this(string jsonPath)
	{
		auto json = parseJSON(readText(jsonPath));

		path = json["path"].get!string;
		resolution = json["resolution"].get!int;
		smoothingRadius = json["smoothingRadius"].get!double;
		erosionMinFlow = json["erosionMinFlow"].get!int;

		foreach(string name, t; json["terrains"])
		{
			auto terrain = new Terrain();
			terrain.name = name;
			terrain.color = Pixel(t["r"].get!ubyte, t["g"].get!ubyte, t["b"].get!ubyte);
			terrain.gradient = t["gradient"].get!double;

			if ("smoothing" in t)
				terrain.smoothing = t["smoothing"].get!bool;

			terrains[terrain.color] = terrain;
		}
	}
}
