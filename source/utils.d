import std.math;
import std.algorithm;
import std.conv;
import std.format;

import dplug.math;

double lerp(double a, double b, double x)
{
	return a * (1-x) + b * x;
}

T safeGet(T, Pos)(T[] array, int width, int height, Pos pos)
{
	int row = pos.y.clamp(0, height - 1).to!int;
	int col = pos.x.clamp(0, width - 1).to!int;

	return array[row * width + col];
}

T safeGetInterpolated(T, Pos)(T[] array, int width, int height, Pos pos)
{
	real x, y;
	real xfrac = modf(pos.x, x);
	real yfrac = modf(pos.y, y);

	auto xint = x.floor;
	auto yint = y.floor;

	auto z00 = safeGet(array, width, height, Pos(xint + 0, yint + 0));
	auto z10 = safeGet(array, width, height, Pos(xint + 1, yint + 0));
	auto z01 = safeGet(array, width, height, Pos(xint + 0, yint + 1));
	auto z11 = safeGet(array, width, height, Pos(xint + 1, yint + 1));

	return lerp(
		lerp(z00, z01, yfrac),
		lerp(z10, z11, yfrac),
		xfrac
	);
}

string toString(T)(Vector!(T,2) vec)
{
	return "(%f,%f)".format(vec.x, vec.y);
}

string toString(T)(Vector!(T,3) vec)
{
	return "(%f,%f,%f)".format(vec.x, vec.y, vec.z);
}
