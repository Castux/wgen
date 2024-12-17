import std.stdio;
import std.concurrency;
import core.thread;
import std.datetime;
import std.conv;
import std.algorithm;
import std.array;
import std.format;
import std.math;
import std.json;

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

private void respond(ref HttpRequestContext ctx, JSONValue json)
{
	respond(ctx, json.toString);
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
			return ctx.respond(`{"width": 0, "height": 0, "lowest": 0, "highest": 0, "vertices": [], "triangles": []}`);

		with (heightmap)
		{
			auto json = JSONValue(["width": width, "height": height, "lowest": lowest, "highest": highest]);

			json["vertices"] = vertices
				.map!(v => v.pos[])
				.join
				.map!(f => f.isNaN ? 0.0 : f)
				.array;

			json["triangles"] = triangles
				.map!(t => t.vertices.map!(v => v.index))
				.join;

			ctx.respond(json);
		}
	}

	void handleVertexColors(ref HttpRequestContext ctx)
	{
		if (!heightmap)
			return ctx.respond("[]");

		auto colors = heightmap.vertices.map!((Vertex v) {
			auto color = v.terrain ? v.terrain.color : vec3d(0,0,0);
			return color[0].to!int << 16 | color[1].to!int << 8 | color[2].to!int;
		});

		ctx.respond(JSONValue(colors.array));
	}

	void handleRivers(ref HttpRequestContext ctx)
	{
		if (!heightmap)
			return ctx.respond("[]");

		auto rivers = heightmap.vertices
			.map!(v => [v.downhill ? v.downhill.index.to!int : -1, v.flow])
			.join;

		ctx.respond(JSONValue(rivers));
	}

	void run(string path)
	{
		ServerConfig cfg;
		cfg.hostname = "0.0.0.0";
		PathHandler pathHandler = new PathHandler();

		pathHandler.addMapping(Method.GET, "/heightmap", &handleHeightmap);
		pathHandler.addMapping(Method.GET, "/colors", &handleVertexColors);
		pathHandler.addMapping(Method.GET, "/rivers", &handleRivers);
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
