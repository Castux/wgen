import std.stdio;
import std.format;
import std.array;
import std.conv;
import std.algorithm;
import std.math;

import heightmap;

void exportOBJ(Heightmap m, string path)
{
	string[] lines;
	ulong[Center] centerIDs;

	auto baseName = path.split(".")[0..$-1].join(".");

	lines ~= "mtllib %s.mtl".format(baseName);
	lines ~= "usemtl material0";

	foreach(i, center; m.centers)
	{
		auto z = center.z;
		if (z == double.infinity)
			z = 0;


		centerIDs[center] = i + 1;
		lines ~= "v %.6f %.6f %.6f".format(center.x, z, center.y);
		lines ~= "vt %.6f %.6f".format(center.x / m.width, 1 - center.y / m.height);
	}

	foreach(corner; m.corners)
	{
		if (corner.centers.any!(c => c.terrain == Terrain.none))
			continue;

		auto ids = corner.centers.map!(c => centerIDs[c]).array.reverse;

		lines ~= "f " ~ ids.map!(i => "%d/%d".format(i,i)).join(" ");
	}

	toFile(lines.join("\n"), path);

	toFile("newmtl material0\nmap_Kd %s.png".format(baseName), baseName ~ ".mtl");
}
