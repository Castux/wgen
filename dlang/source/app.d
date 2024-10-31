import std.stdio;
import std.random;
import std.algorithm;
import std.array;

import delaunator;
import graph;

void draw(Point[] points, delaunator.Edge[] edges)
{
	import std.format;

	string[] lines;

	lines ~= `<svg width='1000' height='1000' viewBox='0 0 1000 1000' xmlns='http://www.w3.org/2000/svg' version='1.1' xmlns:xlink='http://www.w3.org/1999/xlink'>`;

	foreach(point; points)
		lines ~= format(`<circle cx='%f' cy='%f' r='%f' fill="black" />`, point.x, point.y, 2);

	foreach(edge; edges)
	{
		lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="blue" />`,
			edge.from.x, edge.from.y,
			edge.to.x, edge.to.y);
	}

	lines ~= "</svg>";

	toFile(lines.join("\n"), "out1.svg");
}

void draw(Graph g)
{
	import std.format;

	string[] lines;

	lines ~= `<svg width='1000' height='1000' viewBox='0 0 1000 1000' xmlns='http://www.w3.org/2000/svg' version='1.1' xmlns:xlink='http://www.w3.org/1999/xlink'>`;

	foreach(center; g.centers)
	{
		lines ~= format(`<circle cx='%f' cy='%f' r='%f' fill="black" />`, center.x, center.y, 2);

		foreach(n; center.neighbours)
			lines ~= format(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="blue" />`,
				center.x, center.y,
				n.x, n.y);
	}

	lines ~= "</svg>";

	toFile(lines.join("\n"), "out2.svg");
}

void main()
{
	Point[] points;

	while(points.length < 1000)
	{
		auto p = Point(uniform(10.0, 990.0), uniform(10.0, 990.0));
		if (p.dist(Point(500, 500)) < 480)
			points ~= p;
	}

	auto mesh = Triangulation(points);
	writeln("Ignored ", mesh.ignored.length);
	draw(points, mesh.edges);

	writeln(mesh.edges.all!(e => e.triangle.length == 3));

	auto g = new Graph(mesh.edges);
	writeln(g.corners.length);
	writeln(g.centers.length);

	draw(g);
}
