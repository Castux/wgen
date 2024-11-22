import std.stdio;
import std.format;
import std.conv;
import std.regex;
import std.algorithm;
import std.math;
import std.array;

import config;
import heightmap;
import svg;
import obj;
import image;
import viewer;

void outputHeightmap(T)(T[] data, int width, int height, string path)
{
	import gamut;
	writeln("Exporting heightmap");

	alias T = ubyte;
	auto PT = (typeid(T) == typeid(ushort)) ? PixelType.l16 : PixelType.l8;

	auto floored = data.map!("a.floor.lrint");
	T[] output = floored.map!(v => v.clamp(0, T.max).to!T).array;

	writefln("Range: %d, %d", output.minElement, output.maxElement);

	Image image;
	image.createViewFromData(output.ptr, width, height, PT, width * T.sizeof.to!int);
	image.flipVertical();
	image.saveToFile(path);
}

int main(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path>");
		return 1;
	}

	auto path = args[1];
	Heightmap map = new Heightmap(path);
	auto viewer = new Viewer(map, "wgen");

	void doExport()
	{
		outputHeightmap(map.heightmap, map.width, map.height, path ~ ".png");
		outputHeightmap(map.waterLevel, map.width, map.height, path ~ "-w.png");
	}

	while (true)
	{
		auto changed = map.checkConfigUpdate();
		if (changed)
			viewer.onMapChanged();

		auto shouldClose = viewer.draw();
		if (shouldClose)
			break;

		if (viewer.requestExport)
		{
			viewer.requestExport = false;
			doExport();
		}
	}

	writeln("Exporting");
	if (map.conf.exportSVG) exportSVG(map, path ~ ".svg");
	if (map.conf.exportOBJ) exportOBJ(map, path ~ ".obj");
	doExport();

	return 0;
}
