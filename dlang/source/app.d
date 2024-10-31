import std.stdio;
import std.random;
import std.algorithm;

import delaunator;
import graph;

void draw(Point[] points, delaunator.Edge[] edges)
{
	import canvas;
	import std.math;
	import std.conv;

	auto svg = SVGCanvas(1000,1000);

	foreach(point; points)
	{
		new Circle(cast(int) floor(point.x), cast(int) floor(point.y), 3)
			.setFillColor(Colors.blue)
			.setStrokeWidth(0)
			.addToCanvas(svg);
	}

	foreach(edge; edges)
	{
		new Polyline([edge.from.x, edge.from.y, edge.to.x, edge.to.y].to!(int[]))
			.setStrokeWidth(1)
			.setStrokeColor(Colors.black)
			.addToCanvas(svg);
	}

	svg.save("out1.svg");
}

void draw(Graph g)
{
	import canvas;
	import std.math;
	import std.conv;

	auto svg = SVGCanvas(1000,1000);

	foreach(center; g.centers)
	{
		new Circle(cast(int) floor(center.x), cast(int) floor(center.y), 3)
			.setFillColor(Colors.blue)
			.setStrokeWidth(0)
			.addToCanvas(svg);

		foreach(n; center.neighbours)
		{
			if (center.p >= n.p) continue;
			new Line(
				cast(int) floor(center.x), cast(int) floor(center.y),
				cast(int) floor(n.x), cast(int) floor(n.y)
				)
				.setStrokeColor(Colors.gray)
				.setStrokeWidth(1)
				.addToCanvas(svg);
		}
	}

	foreach(corner; g.corners)
	{
		new Circle(cast(int) floor(corner.x), cast(int) floor(corner.y), 3)
			.setFillColor(Colors.red)
			.setStrokeWidth(0)
			.addToCanvas(svg);

		foreach(n; corner.neighbours)
		{
			if (corner.p >= n.p) continue;

			new Line(
				cast(int) floor(corner.x), cast(int) floor(corner.y),
				cast(int) floor(n.x), cast(int) floor(n.y)
				)
				.setStrokeColor(Colors.orange)
				.setStrokeWidth(1)
				.addToCanvas(svg);
		}
	}

	svg.save("out2.svg");
}

void main()
{
	Point[] points;

	while(points.length < 1000)
	{
		auto p = Point(uniform(0.0, 1000.0), uniform(0.0, 1000.0));
		if (p.dist(Point(500, 500)) < 480)
			points ~= p;
	}

	auto mesh = Triangulation(points);
	writeln("Ignored ", mesh.ignored.length);
	draw(points, mesh.edges);

	auto g = new Graph(mesh.edges);
	writeln(g.corners.length);
	writeln(g.centers.length);

	draw(g);
}
