import std.stdio;
import std.format;
import std.array;
import std.conv;
import std.algorithm;
import std.math;

import heightmap;
import utils;

void exportSVG(Heightmap m, string path)
{
	string[] lines;

	lines ~=
		`<svg width='%f' height='%f'
		viewBox='%f %f %f %f'
		xmlns='http://www.w3.org/2000/svg'
		version='1.1'
		xmlns:xlink='http://www.w3.org/1999/xlink'>`
			.format(m.width, m.height,
					0.0, -m.height, m.width, m.height);

	foreach(vertex; m.vertices)
	{
		auto s = vertex.triangles.map!(c => "%f,%f".format(c.x, -c.y)).join(" ");

		string col;

		if (!vertex.terrain)
			col = "pink";
		else
		switch (vertex.terrain.name)
		{
			case "lake":
				col = "#0E443D";
				break;
			case "sea":
				auto f = vertex.z / m.lowest;
				col = "rgb(%.2f,%.2f,%.2f)".format(
					lerp(95, 0, f),
					lerp(132, 10, f),
					lerp(255, 100, f),
				);
				break;
			default:
				auto f = vertex.z / m.highest;
				col = "rgb(%.2f,%.2f,%.2f)".format(
					lerp(84, 255, f),
					lerp(169, 255, f),
					lerp(50, 255, f)
				);
		}

		lines ~= format(`<polygon points="%s" fill="%s" stroke="%s" />`, s, col, col);
	}

	foreach(vertex; m.vertices)
	{
		if (vertex.downhill)
		{
			auto width = pow(vertex.flow, 0.5) * pow(m.resolution / 30, 2);
			lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#0E443D" stroke-width="%f" />`,
	 			vertex.x, -vertex.y,
	 			vertex.downhill.x, -vertex.downhill.y,
				width
			);
		}
	}

	lines ~= "</svg>";

	toFile(lines.join("\n"), path);
}
