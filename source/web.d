import std.stdio;
import std.concurrency;
import core.thread;
import std.datetime;
import std.conv;
import std.algorithm;
import std.array;
import std.format;
import std.math;

import dplug.math;

import handy_httpd;
import handy_httpd.components.websocket;
import handy_httpd.handlers.path_handler;
import handy_httpd.handlers.file_resolving_handler;

import heightmap;

private void respond(ref HttpRequestContext ctx, string json)
{
	import std.zlib;

	auto compressed = compress(json);
	ctx.response.addHeader("Content-Encoding", "deflate");
	ctx.response.writeBodyBytes(compressed, "application/json");
}

class Server
{
	HttpServer server;
	Thread serverThread;

	Heightmap heightmap;
	int lastVersion;

	void handleHeightmap(ref HttpRequestContext ctx)
	{
		if (!heightmap)
		{
			ctx.respond(`{"width": 0, "height": 0, "lowest": 0, "highest": 0, "vertices": [], "triangles": []}`);
			return;
		}

		auto json = appender!string;

		with (heightmap)
		{
			json.put("{");
			json.put(`"width": %f, "height": %f,`.format(width, height));
			json.put(`"lowest": %f, "highest": %f,`.format(lowest, highest));
			json.put(`"vertices":[`);

			foreach(v, vertex; vertices)
			{
				json.put("%s,%s,%s".format(
					vertex[0].isNaN ? `"0"` : "%.2f".format(vertex[0]),
					vertex[1].isNaN ? `"0"` : "%.2f".format(vertex[1]),
					vertex[2].isNaN ? `"0"` : "%.2f".format(vertex[2])
				));
				if (v < vertices.length - 1)
					json.put(',');
			}

			json.put(`], "triangles":[`);

			foreach(t, tri; triangles)
			{
				json.put("%d,%d,%d".format(
					tri.vertices[0].index,
					tri.vertices[1].index,
					tri.vertices[2].index
				));
				if (t < triangles.length - 1)
					json.put(',');
			}

			json.put(`]}`);
		}

		ctx.respond(json[]);
	}

	void handleVertexColors(ref HttpRequestContext ctx)
	{
		if (!heightmap)
		{
			ctx.respond("[]");
			return;
		}

		auto colors = heightmap.vertices.map!((Vertex v) {
			auto color = v.terrain ? v.terrain.color : vec3d(0,0,0);
			return "%d".format(color[0].to!int << 16 | color[1].to!int << 8 | color[2].to!int);
		});
		auto json = "[" ~ colors.join(",") ~ "]";
		ctx.respond(json);
	}

	void run(string path)
	{
		ServerConfig cfg;
		cfg.hostname = "0.0.0.0";
		PathHandler pathHandler = new PathHandler();

		pathHandler.addMapping(Method.GET, "/heightmap", &handleHeightmap);
		pathHandler.addMapping(Method.GET, "/colors", &handleVertexColors);
		pathHandler.addMapping(Method.GET, "**", new FileResolvingHandler("static"));

		server = new HttpServer(pathHandler, cfg);
		serverThread = server.startInNewThread();

		heightmap = new Heightmap(path);
		lastVersion = heightmap.changeCount;

		while (true)
		{
			heightmap.checkConfigUpdate();
			if (heightmap.changeCount != lastVersion)
			{
				writeln("Changed!");
				lastVersion = heightmap.changeCount;
			}

			Thread.sleep(100.msecs);
		}
	}
}
