import std.stdio;
import std.concurrency;
import core.thread;
import std.datetime;
import std.conv;

import handy_httpd;
import handy_httpd.components.websocket;
import handy_httpd.handlers.path_handler;
import handy_httpd.handlers.file_resolving_handler;

import heightmap;

class Server: HttpRequestHandler
{
	HttpServer server;
	Thread serverThread;

	Heightmap heightmap;
	int lastVersion;

	void handle(ref HttpRequestContext ctx)
	{
		ctx.response.writeBodyString("Hello dummy " ~ lastVersion.to!string);
	}

	void run(string path)
	{
		ServerConfig cfg;
		PathHandler pathHandler = new PathHandler();

		pathHandler.addMapping(Method.GET, "/dummy", this);

		// auto staticFiles = new FileResolvingHandler("static", DirectoryResolutionStrategies.none);
		auto staticFiles = new FileResolvingHandler("static");
		pathHandler.addMapping(Method.GET, "**", staticFiles);

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
