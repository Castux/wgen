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

vec3 fromCenter(Center c)
{
	return vec3(c.x, c.y, c.z);
}

void updateVertices(Model model, Heightmap map)
{
	model.vertices.length = 3 * map.corners.length;
	foreach(i, corner; map.corners)
	{
		model.vertices[i * 3 + 0].pos = corner.centers[0].fromCenter;
		model.vertices[i * 3 + 1].pos = corner.centers[1].fromCenter;
		model.vertices[i * 3 + 2].pos = corner.centers[2].fromCenter;
		model.vertices[i * 3 + 0].col = vec3(1.0, 0.0, 0.0);
		model.vertices[i * 3 + 1].col = vec3(0.0, 1.0, 0.0);
		model.vertices[i * 3 + 2].col = vec3(0.0, 0.0, 1.0);
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
		auto viewer = new Viewer(1024, 768, "wgen");
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
