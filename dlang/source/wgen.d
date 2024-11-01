import std.stdio;
import std.format;
import std.conv;
import std.regex;

import heightmap;
import svg;
import obj;
import image;

void main(string[] args)
{
	if (args.length < 3)
	{
		writeln("Usage: wgen <path> <resolution>");
		return;
	}

	auto path = args[1];
	auto resolution = args[2].to!int;

	Heightmap map = new Heightmap(path, resolution);

	writeln("Exporting");
	exportSVG(map, path.replaceFirst(regex(`\....$`), ".svg"));
	exportOBJ(map, path.replaceFirst(regex(`\....$`), "-h.obj"));

	writeln("Rasterizing");
	auto elevations = map.rasterize();
	exportHeightmap(elevations, path.replaceFirst(regex(`\....$`), "-h.png"));
}
