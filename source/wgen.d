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
	model.vertices.length = map.corners.length * 3;
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

		const vec3[3] coords = [
			vec3(1,0,0),
			vec3(0,1,0),
			vec3(0,0,1)
		];

		foreach (j, center; corner.centers)
		{
			model.vertices[i * 3 + j] = Vertex(
				vec3(center.x, -center.y, center.z),
				normal,
				coords[j]
			);
		}
	}

	model.updateData();
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
				{
					writefln("Range %f %f", map.lowest, map.highest);
					updateVertices(model, map);
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

	return 0;
}
