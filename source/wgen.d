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

int main(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path> [--interactive]");
		return 1;
	}

	auto path = args[1];
	bool interactive = args.length >= 3 && args[2] == "--interactive";

	Heightmap map = new Heightmap(path);

	void doExport()
	{
		writeln("Exporting");
		if (map.conf.exportSVG) exportSVG(map, path ~ ".svg");
		if (map.conf.exportOBJ) exportOBJ(map, path ~ ".obj");
		map.exportHeightmaps(path ~ ".png", path ~ "-w.png");
	}

	if (interactive)
	{
		Viewer viewer = new Viewer(map, "wgen");

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
	}

	doExport();

	return 0;
}
