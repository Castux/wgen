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
	}

	foreach(corner; g.corners)
	{
		new Circle(cast(int) floor(corner.x), cast(int) floor(corner.y), 3)
			.setFillColor(Colors.red)
			.setStrokeWidth(0)
			.addToCanvas(svg);
	}

	foreach(edge; g.edges)
	{
		if (edge.corners.length == 2)
		new Line(
			cast(int) floor(edge.corners[0].x), cast(int) floor(edge.corners[0].y),
			cast(int) floor(edge.corners[1].x), cast(int) floor(edge.corners[1].y)
			)
			.setStrokeColor(Colors.yellow)
			.setStrokeWidth(1)
			.addToCanvas(svg);

		if (edge.centers.length == 2)
		new Line(
			cast(int) floor(edge.centers[0].x), cast(int) floor(edge.centers[0].y),
			cast(int) floor(edge.centers[1].x), cast(int) floor(edge.centers[1].y)
			)
			.setStrokeColor(Colors.orange)
			.setStrokeWidth(1)
			.addToCanvas(svg);
	}

	svg.save("out2.svg");
}

void main()
{
	Point[] points;

	foreach(i; 0..100)
		points ~= Point(uniform(0.0, 1000.0), uniform(0.0, 1000.0));

	auto mesh = Triangulation(points);
	draw(points, mesh.edges);

	auto g = new Graph(mesh.edges);
	writeln(g.edges.length);
	writeln(g.corners.length);
	writeln(g.centers.length);

	draw(g);
}
