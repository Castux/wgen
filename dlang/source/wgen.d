import std.stdio;
import std.format;
import std.conv;
import std.regex;

import config;
import heightmap;
import svg;
import obj;
import image;

int main(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path>");
		return 1;
	}

	auto path = args[1];

	Config conf = new Config(path);
	Heightmap map = new Heightmap(conf);

	writeln("Exporting");
	exportSVG(map, path ~ ".svg");
	exportOBJ(map, path ~ ".obj");

	writeln("Rasterizing");
	auto elevations = map.rasterize();
	exportHeightmap(elevations, path ~ "-h.png");

	return 0;
}
