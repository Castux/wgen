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

int main2(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path> [--update]");
		return 1;
	}

	auto path = args[1];
	auto updateMode = args.length == 3 && args[2] == "--update";

	Config conf = new Config(path);
	Heightmap map = new Heightmap(conf);

	void exports()
	{
		writeln("Exporting");
		if (conf.exportSVG) exportSVG(map, path ~ ".svg");
		if (conf.exportOBJ) exportOBJ(map, path ~ ".obj");
		if (conf.exportHeightmap)
		{
			auto elevations = map.rasterize();
			exportHeightmap(elevations, path ~ "-h.png");
		}
	}

	exports();

	if (updateMode)
	{
		writeln("Waiting for update");

		auto watcher = FileWatch(path);
		while (true)
		{
			foreach (event; watcher.getEvents())
			{
				if (event.path == path && event.type == FileChangeEventType.modify)
				{
					Config newConfig = new Config(path);
					auto changed = map.updateConfig(newConfig);

					if (changed)
					{
						exports();
						writeln("Waiting for update");
					}
				}
			}
		}
	}

	return 0;
}

void main()
{
	auto viewer = new Viewer(1024, 768, "wgen");
	viewer.run();
}
