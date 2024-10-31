import std.stdio;
import std.random;
import std.algorithm;

import delaunator;

void draw(Point[] points, Edge[] edges)
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

	svg.save("out.svg");
}

void main()
{
	Point[] points;

	foreach(i; 0..500)
		points ~= Point(uniform(0,1000), uniform(0,1000));

	auto mesh = Triangulation(points);
	//writeln(mesh.edges.map!"a.from");
	//writeln(mesh.ignored);
	writeln(mesh.edges.length);

	draw(points, mesh.edges);
}
