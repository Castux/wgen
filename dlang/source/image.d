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

	Image image = Image(width, height, PixelType.rgba8);

	foreach(int r; 0..height)
	{
		auto scanline = cast(uint[]) image.scanline(r);
		foreach(c, value; data[r])
		{
			if (value.isNaN)
				value = 0;

			value = (value - low) / (high - low) * 255;
			ubyte grey = value.to!ubyte;
			scanline[c] = (0xFF << 24) | (grey << 16) | (grey << 8) | grey;
		}
	}

	image.saveToFile(path);
}
