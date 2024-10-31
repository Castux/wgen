import std.stdio;
import std.random;
import std.algorithm;

import delaunator;

void main()
{
	Point[] points;

	foreach(i; 0..10000)
		points ~= Point(uniform(0,20000), uniform(0,20000));

	auto mesh = Triangulation(points);
	writeln(mesh.edges.map!"a.from");
}
