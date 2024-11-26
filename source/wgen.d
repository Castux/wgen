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

void outputHeightmap(float[] data, int width, int height, string path)
{
	import gamut;
	writeln("Exporting heightmap");

	alias T = ushort;
	auto PT = (typeid(T) == typeid(ushort)) ? PixelType.l16 : PixelType.l8;

	T[] output = new T[data.length];
	bool outOfBounds;
	float outOfBoundsValue;

	foreach(i, v; data)
	{
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
		outputHeightmap(viewer.blurredHeightmap, map.width, map.height, path ~ ".png");
		outputHeightmap(viewer.waterLevelHeightmap, map.width, map.height, path ~ "-w.png");
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
