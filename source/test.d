import std.stdio;
import map;
import std.conv;
import geom;
import std.random;
import std.math;

void main()
{
	auto map = new Map(1920,1080);

	Point[] points = [];

	while(points.length < 100)
	{
		auto p = Point(uniform(1.0, map.width - 1), uniform(1.0, map.height - 1));
		points ~= p;
	}

	writeln("done points");

	map.makeDelaunay(points);

	auto fp = File("fubar.svg", "w");
	fp.write(map.toSVG);
	fp.close;
}
