import std.stdio;
import std.format;
import std.array;
import std.conv;
import std.algorithm;
import std.random;
import std.math;

import heightmap;

double lerp(double a, double b, double x)
{
	return a * (1-x) + b * x;
}

void draw(Map g, int i)
{
	string[] lines;

	lines ~=
		`<svg width='%f' height='%f'
		viewBox='0 0 %f %f'
		xmlns='http://www.w3.org/2000/svg'
		version='1.1'
		xmlns:xlink='http://www.w3.org/1999/xlink'>`
			.format(g.width, g.height, g.width, g.height);

	foreach(center; g.centers)
	{
		auto s = center.corners.map!(c => "%f,%f".format(c.x, c.y)).join(" ");

		// auto h = (center.z - g.lowest) / (g.highest - g.lowest) * 255;
		// auto col = "rgb(%.2f,%.2f,%.2f)".format(h, h, h);
		//
		// if (center.terrain == Terrain.none)
		// 	col = "pink";

		string col;
		switch (center.terrain)
		{
			case Terrain.none: col = "pink"; break;
			case Terrain.lake:
				col = "#0E443D";
				break;
			case Terrain.sea:
				auto f = center.z / g.lowest;
				col = "rgb(%.2f,%.2f,%.2f)".format(
					lerp(95, 0, f),
					lerp(132, 10, f),
					lerp(255, 100, f),
				);
				break;
			default:
				auto f = center.z / g.highest;
				col = "rgb(%.2f,%.2f,%.2f)".format(
					lerp(84, 255, f),
					lerp(169, 255, f),
					lerp(50, 255, f)
				);
		}
		
		lines ~= format(`<polygon points="%s" fill="%s" stroke="%s" />`, s, col, col);

	}

	foreach(center; g.centers)
	{
		if (center.downhill)
		{
			auto width = pow(center.flow, 0.5) * pow(g.resolution / 30, 2);
			lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#0E443D" stroke-width="%f" />`,
	 			center.x, center.y,
	 			center.downhill.x, center.downhill.y,
				width
			);
		}
	}

	lines ~= "</svg>";

	toFile(lines.join("\n"), "out%d.svg".format(i));
}

void main(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path> <resolution>");
		return;
	}

	auto path = args[1];
	auto resolution = args[2].to!int;

	Map map = new Map(path, resolution);
	draw(map, 0);

	writeln("Low ", map.lowest);
	writeln("High ", map.highest);
}
