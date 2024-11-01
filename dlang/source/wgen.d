import std.stdio;
import std.format;
import std.conv;
import std.regex;

import heightmap;
import svg;

void main(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path> <resolution>");
		return;
	}

	auto path = args[1];
	auto resolution = args[2].to!int;

	Heightmap map = new Heightmap(path, resolution);
	exportSVG(map, path.replaceFirst(regex(`\....$`), ".svg"));

	writeln("Low ", map.lowest);
	writeln("High ", map.highest);
}
