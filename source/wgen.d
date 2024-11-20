import std.stdio;
import std.format;
import std.conv;
import std.regex;

import fswatch;

import config;
import heightmap;
import svg;
import obj;
import image;
import viewer;

void outputHeightmap(float[] map, int width, int height, double lowest, double highest, string path)
{
	import gamut;
	writeln("Exporting heightmap");

	float[] normalized = new float[map.length];
	normalized[] = (map[] - lowest) / (highest - lowest);

	Image image;
	image.createViewFromData(normalized.ptr, width, height, PixelType.lf32, width * float.sizeof.to!int);
	image.flipVertical();
	image.convertTo(PixelType.l16);
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

	Config conf = new Config(path);
	Heightmap map = new Heightmap(conf);

	writefln("Range %f %f", map.lowest, map.highest);

	auto viewer = new Viewer(map, "wgen");
	auto watcher = FileWatch(path);

	while (true)
	{
		foreach (event; watcher.getEvents())
		{
			if (event.type == FileChangeEventType.modify)
			{
				Config newConfig = new Config(path);
				auto changed = map.updateConfig(newConfig);

				if (changed)
				{
					writefln("Range %f %f", map.lowest, map.highest);
					viewer.onMapChanged();
				}

				break;
			}
		}

		auto shouldClose = viewer.draw();
		if (shouldClose)
			break;
	}

	writeln("Exporting");
	if (conf.exportSVG) exportSVG(map, path ~ ".svg");
	if (conf.exportOBJ) exportOBJ(map, path ~ ".obj");
	outputHeightmap(viewer.blurredHeightmap, map.width, map.height, map.lowest, map.highest, path ~ ".png");

	return 0;
}
