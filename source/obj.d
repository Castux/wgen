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
	ulong[Vertex] vertexIDs;

	foreach(i, vertex; m.vertices)
	{
		auto z = vertex.z;
		if (z == double.infinity)
			z = 0;

		vertexIDs[vertex] = i + 1;
		lines ~= "v %.6f %.6f %.6f".format(vertex.x, z, vertex.y);
		lines ~= "vt %.6f %.6f".format(vertex.x / m.width, 1 - vertex.y / m.height);
	}

	foreach(triangle; m.triangles)
	{
		if (triangle.vertices.any!(c => c.terrain is null))
			continue;

		auto ids = triangle.vertices.map!(c => vertexIDs[c]).array.reverse;

		lines ~= "f " ~ ids.map!(i => "%d/%d".format(i,i)).join(" ");
	}

	toFile(lines.join("\n"), path);
}
