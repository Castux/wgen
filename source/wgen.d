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

		auto p0 = corner.centers[0].p;
		auto p1 = corner.centers[1].p;
		auto p2 = corner.centers[2].p;

		auto v0 = vec3(p0.x, -p0.y, corner.centers[0].z);
		auto v1 = vec3(p1.x, -p1.y, corner.centers[1].z);
		auto v2 = vec3(p2.x, -p2.y, corner.centers[2].z);

		auto normal = cross(v1 - v0, v2 - v0);
		normal.normalize();

		foreach (center; corner.centers)
		{
			model.vertices ~= Vertex(
				vec3(center.x, -center.y, center.z),
				normal
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
		auto viewer = new Viewer(1024, 768, "wgen");
		auto model = viewer.newModel();
		updateVertices(model, map);

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
						updateVertices(model, map);

					break;
				}
			}

			auto shouldClose = viewer.draw(map.lowest, map.highest);
			if (shouldClose)
				break;
		}

	}
	else
		exports();

	return 0;
}
