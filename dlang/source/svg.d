import std.stdio;
import std.format;
import std.array;
import std.conv;
import std.algorithm;
import std.math;

import heightmap;

double lerp(double a, double b, double x)
{
	return a * (1-x) + b * x;
}

void exportSVG(Heightmap m, string path)
{
	string[] lines;

	lines ~=
		`<svg width='%f' height='%f'
		viewBox='0 0 %f %f'
		xmlns='http://www.w3.org/2000/svg'
		version='1.1'
		xmlns:xlink='http://www.w3.org/1999/xlink'>`
			.format(m.width, m.height, m.width, m.height);

	foreach(center; m.centers)
	{
		auto s = center.corners.map!(c => "%f,%f".format(c.x, c.y)).join(" ");

		string col;

		if (!center.terrain)
			col = "pink";
		else
		switch (center.terrain.name)
		{
			case "lake":
				col = "#0E443D";
				break;
			case "sea":
				auto f = center.z / m.lowest;
				col = "rgb(%.2f,%.2f,%.2f)".format(
					lerp(95, 0, f),
					lerp(132, 10, f),
					lerp(255, 100, f),
				);
				break;
			default:
				auto f = center.z / m.highest;
				col = "rgb(%.2f,%.2f,%.2f)".format(
					lerp(84, 255, f),
					lerp(169, 255, f),
					lerp(50, 255, f)
				);
		}

		lines ~= format(`<polygon points="%s" fill="%s" stroke="%s" />`, s, col, col);
	}

	foreach(center; m.centers)
	{
		if (center.downhill)
		{
			auto width = pow(center.flow, 0.5) * pow(m.resolution / 30, 2);
			lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#0E443D" stroke-width="%f" />`,
	 			center.x, center.y,
	 			center.downhill.x, center.downhill.y,
				width
			);
		}
	}

	lines ~= "</svg>";

	toFile(lines.join("\n"), path);
}
