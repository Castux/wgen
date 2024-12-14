import std.stdio;
import std.concurrency;
import core.thread;
import std.datetime;
import std.conv;
import std.algorithm;
import std.array;
import std.format;

import dplug.math;

import handy_httpd;
import handy_httpd.components.websocket;
import handy_httpd.handlers.path_handler;
import handy_httpd.handlers.file_resolving_handler;

import heightmap;

private HttpRequestHandler toHandler(void delegate(ref HttpRequestContext ctx) fun)
{
	class Handler: HttpRequestHandler
	{
		void handle(ref HttpRequestContext ctx)
		{
			fun(ctx);
		}
	}

	return new Handler();
}

class Server
{
	HttpServer server;
	Thread serverThread;

	Heightmap heightmap;
	int lastVersion;

	private static respond(ref HttpRequestContext ctx, string json)
	{
		import std.zlib;

		auto compressed = compress(json);
		ctx.response.addHeader("Content-Encoding", "deflate");
		ctx.response.writeBodyBytes(compressed, "application/json");
	}

	void handleHeightmap(ref HttpRequestContext ctx)
	{
		auto json = heightmap.toJson;
		respond(ctx, json);
	}

	void handleVertexColors(ref HttpRequestContext ctx)
	{
		auto colors = heightmap.vertices.map!((Vertex v) {
			auto color = v.terrain ? v.terrain.color : vec3d(0,0,0);
			return "%d".format(color[0].to!int << 16 | color[1].to!int << 8 | color[2].to!int);
		});
		auto json = "[" ~ colors.join(",") ~ "]";
		respond(ctx, json);
	}

	void run(string path)
	{
		ServerConfig cfg;
		PathHandler pathHandler = new PathHandler();

		pathHandler.addMapping(Method.GET, "/heightmap", toHandler(&handleHeightmap));
		pathHandler.addMapping(Method.GET, "/colors", toHandler(&handleVertexColors));
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
