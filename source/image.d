import std.stdio;
import std.array;
import std.conv;
import std.algorithm;
import std.math;

import gamut;

void exportHeightmap(double[][] data, string path)
{
	auto width = cast(int) data[0].length;
	auto height = cast(int) data.length;

	auto low = data.map!(row => row.filter!(a => !a.isNaN).minElement).minElement;
	auto high = data.map!(row => row.filter!(a => !a.isNaN).maxElement).maxElement;

	Image image = Image(width, height, PixelType.l16);

	foreach(int r; 0 .. height)
	{
		auto scanline = cast(ushort[]) image.scanline(r);
		foreach(c, value; data[r])
		{
			if (value.isNaN)
				value = low;

			value = (value - low) / (high - low) * ushort.max;
			scanline[c] = value.roundTo!ushort;
		}
	}

	image.saveToFile(path);
}
