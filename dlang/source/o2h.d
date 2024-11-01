import std.stdio;
import std.format;
import std.array;

import map;


void draw(Map g)
{
	string[] lines;

	lines ~=
		`<svg width='%f' height='%f'
		viewBox='0 0 %f %f'
		xmlns='http://www.w3.org/2000/svg'
		version='1.1'
		xmlns:xlink='http://www.w3.org/1999/xlink'>`
			.format(g.width, g.height, g.width, g.height);

	// foreach(center; g.centers)
	// {
	// 	lines ~= format(`<circle cx='%f' cy='%f' r='%f' fill="black" />`, center.x, center.y, 2);
	//
	// 	foreach(n; center.neighbours)
	// 		lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="blue" />`,
	// 			center.x, center.y,
	// 			n.x, n.y);
	// }
	//
	// foreach(corner; g.corners)
	// {
	// 	lines ~= format(`<circle cx='%f' cy='%f' r='%f' fill="red" />`, corner.x, corner.y, 2);
	//
	// 	foreach(n; corner.neighbours)
	// 		lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="pink" />`,
	// 			corner.x, corner.y,
	// 			n.x, n.y);
	// }

	foreach(edge; g.edges)
	{
		lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="blue" />`,
 			edge.center1.x, edge.center1.y,
 			edge.center2.x, edge.center2.y);

		if (edge.corner2)
			lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="red" />`,
				edge.corner1.x, edge.corner1.y,
				edge.corner2.x, edge.corner2.y);
	}

	lines ~= "</svg>";

	toFile(lines.join("\n"), "out.svg");
}

void main(string[] args)
{
	if (args.length < 2)
	{
		writeln("Usage: wgen <path> <resolution>");
		return;
	}

	auto path = args[1];
	auto resolution = 10;

	Map map = new Map(path, resolution);
	draw(map);
	//
	// writeln("Loading " ~ path);
	// auto outline = loadOutline(path);
	//
	// writeln("Generating graph...");
	// auto graph = generateGraph(outline.width, outline.height, resolution);
	//
	// writeln("Relaxing graph...");
	// graph = relaxGraph(graph, outline.width, outline.height);
	//
	// foreach(center; graph.centers)
	// {
	// 	center.foo = 20;
	// 	writeln("WOOO ", center.neighbours.length);
	// }

	// writeln("Assigning landmasses");
	// assignCellTypes(graph, outline);

}
