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
import web;

int main(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path> [--interactive]");
		return 1;
	}

	auto path = args[1];
	bool interactive = args.length >= 3 && args[2] == "--interactive";

	void doExport(Heightmap map)
	{
		writeln("Exporting");
		if (map.conf.exportSVG) exportSVG(map, path ~ ".svg");
		if (map.conf.exportOBJ) exportOBJ(map, path ~ ".obj");
		map.exportHeightmaps(path ~ ".png", path ~ "-w.png");
	}

	if (interactive)
	{
		// Viewer viewer = new Viewer(map, "wgen");
		// map.interactive = true;
		//
		// while (true)
		// {
		// 	if (!map.loading)
		// 	{
		// 		task(&map.checkConfigUpdate).executeInNewThread();
		// 	}
		//
		// 	viewer.update(map);
		// 	auto shouldClose = viewer.draw();
		// 	if (shouldClose)
		// 		break;
		//
		// 	if (viewer.requestExport)
		// 	{
		// 		viewer.requestExport = false;
		// 		task(&doExport).executeInNewThread();
		// 	}
		// }

		auto server = new Server();
		server.run(path);
	}
	else
	{
		Heightmap map = new Heightmap(path);
		doExport(map);
	}

	return 0;
}
