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

void updateVertices(Model model, Heightmap map)
{
	model.vertices.length = 0;
	foreach_reverse(i, corner; map.corners)
	{
		if (!map.inBounds(corner.p)) continue;

		foreach (center; corner.centers)
		{
			double ratio = (center.z - map.lowest) / (map.highest - map.lowest);
			vec3 col;
			if (center.z > 0)
			{
				col = vec3(ratio, ratio, ratio);
			}
			else
			{
				col = vec3(0.0, 0.0, 0.8) * ratio;
			}

			model.vertices ~= Vertex(
				vec3(center.x, -center.y, center.z),
				col
			);
		}
	}

	model.updateData();
}

int main(string[] args)
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

	writefln("Range %f %f", map.lowest, map.highest);

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

	if (updateMode)
	{
		auto viewer = new Viewer(1024, 1024, "wgen");
		auto model = viewer.newModel();

		updateVertices(model, map);

		viewer.run();

		//
		// writeln("Waiting for update");
		//
		// auto watcher = FileWatch(path);
		// while (true)
		// {
		// 	foreach (event; watcher.getEvents())
		// 	{
		// 		if (event.path == path && event.type == FileChangeEventType.modify)
		// 		{
		// 			Config newConfig = new Config(path);
		// 			auto changed = map.updateConfig(newConfig);
		//
		// 			if (changed)
		// 			{
		// 				exports();
		// 				writeln("Waiting for update");
		// 			}
		// 		}
		// 	}
		// }
	}
	else
		exports();

	return 0;
}
