import std.stdio;
import std.string;
import std.exception;
import std.math;
import std.algorithm;
import std.conv;
import std.range;
import std.parallelism;
import std.datetime.stopwatch;
import std.string;

import bindbc.sfml;
import bindbc.opengl;
public import dplug.math;

import heightmap;
import utils;
import viewerutils;

static const char* vertexShader = `
#version 330
uniform mat4 PV;
uniform mat4 M;
uniform float waterLevelOverride;
uniform vec3 colorOverride;
in vec3 vNorm;
in vec3 vPos;
in vec3 vCoord;
in vec3 vColor;
in float vWaterLevel;
out vec3 worldPos;
out vec3 normal;
out vec3 coord;
out vec3 terrainColor;
out float waterLevel;
void main()
{
	vec4 pos = M * vec4(vPos, 1.0);
	gl_Position = PV * pos;
	normal = vNorm;
	coord = vCoord;
	terrainColor = vColor.x < 0 ? colorOverride : vColor;
	worldPos = pos.xyz;
	waterLevel = max(waterLevelOverride, vWaterLevel);
}`;
// `


static const char* fragmentShader = `
#version 330
uniform float lowest;
uniform float highest;
uniform int mode;
uniform int lineMode;
uniform vec4 fpsCenter;
in vec3 worldPos;
in vec3 normal;
in vec3 coord;
in vec3 terrainColor;
in float waterLevel;
out vec4 fragment;

float distToInt(float x)
{
	x = mod(x, 1.0);
	if (x > 0.5) x = x - 1.0;
	return abs(x);
}

void main()
{
	float z = worldPos.z;

	if (length(worldPos - fpsCenter.xyz) < fpsCenter.w)
		discard;

	vec3 color = vec3(1.0, 0.0, 1.0);
	float f = (z - lowest) / (highest - lowest);

	if (mode == 0)
	{
		color = vec3(f, f, f);
	}
	else if (mode == 1)
	{
		color = vec3(1.0, 1.0, 1.0);
	}
	else if (mode == 2)
	{
		if (z < waterLevel)
		{
			color = mix(vec3(0, 10, 100), vec3(95, 132, 255), f) / 255.0;
		}
		else
		{
			color = mix(vec3(42, 84, 25), vec3(200, 255, 200), f) / 255.0;
		}
	}
	else if (mode == 3)
	{
		color = terrainColor;
	}

	float shading = 1.0;
	if (mode > 0)
	{
		vec3 sun = normalize(vec3(-1, -0.65, 0.5));
		float sunAngle = dot(normal, sun);
		shading = (sunAngle + 1.0) / 2.0 * 0.7 + 0.3;
	}

	if (lineMode == 1)
	{
		shading = shading * step(0.25, distToInt(z / 10) * 10.0);
	}
	else if (lineMode == 2)
	{
		shading = shading * step(0.075, min(distToInt(worldPos.x), distToInt(worldPos.y)));
	}
	else if (lineMode == 3)
	{
		float edgeDist = min(coord.x, min(coord.y, coord.z));
		float fw = fwidth(edgeDist);
		float thickness = 0.5;
		shading = shading * smoothstep(thickness * fw, (thickness + 1) * fw, edgeDist);
	}

	vec3 shaded = color * shading;
	fragment = vec4(shaded, 1.0);
}`;
// `

private struct Map
{
	int width;
	int height;
	double lowest;
	double highest;
	const(double)[] heightmap;
	const(double)[] waterLevel;
	vec3f[] outline;

	int changeCount = -1;

	bool inBounds(V)(V p) const
	{
		return p.x >= 0 && p.x < width && p.y >= 0 && p.y < width;
	}

	void update(const(Heightmap) hmap)
	{
		width = hmap.width;
		height = hmap.height;
		lowest = hmap.lowest;
		highest = hmap.highest;
		heightmap = hmap.heightmap;
		waterLevel = hmap.waterLevel;
		outline = hmap.outline.to!(vec3f[]);
	}

	auto getOutline(vec2f pos)
	{
		return safeGet(outline, width, height, pos) / 255.0;
	}

	auto getZ(vec2f pos, bool smooth = false)
	{
		if (smooth)
			return safeGetInterpolated(heightmap, width, height, pos);
		else
			return safeGet(heightmap, width, height, pos);
	}

	auto getWaterLevel(vec2f pos)
	{
		return safeGetInterpolated(waterLevel, width, height, pos);
	}
}

class Viewer
{
	static Viewer singleton;

	sfRenderWindow* window;
	sfView* view;
	sfFont* font;
	sfText* text;
	string[] currentText;
	double lastFpsUpdate = 0;
	int framesCount;
	double fps = 0;

	GLuint program;
	bool requestExport;

	Map map;
	Model mainMesh;

	int shadingMode;
	int viewMode;
	int lineMode;

	vec3f firstPersonPos;
	double firstPersonHDir;
	double firstPersonVDir;
	double lastUpdate;
	bool walking;

	int cubeMode;
	Model cubeMesh;
	int cubesRadius = 100;

	StopWatch sw;

	this(Heightmap hmap, string title)
	{
		if(!loadSFML())
		{
			bindbcError();
			throw new Exception("Could not load SFML library");
		}

		auto mode = sfVideoMode(1024, 768, 32);
		sfContextSettings settings;
		settings.depthBits = 24;
		settings.stencilBits = 8;
		settings.antialiasingLevel = 2;
		settings.majorVersion = 3;
		settings.minorVersion = 3;

		window = sfRenderWindow_create(mode, "wgen", sfResize | sfClose, &settings);
		if (!window)
			throw new Exception("Could not open SFML window");
		sfRenderWindow_setVerticalSyncEnabled(window, true);
		view = sfView_createFromRect(sfFloatRect(0, 0, mode.width, mode.height));

		resetText();

		if(loadOpenGL() != GLSupport.gl33)
		{
			bindbcError();
			throw new Exception("Could not load OpenGL library");
		}
		glEnable(GL_DEPTH_TEST);
		glEnable(GL_CULL_FACE);

		if (singleton)
			throw new Exception("Multiple viewer instances");
		singleton = this;

		program = setupShaders(vertexShader, fragmentShader);

		cubeMesh = Model.makeCubeMesh(program);
		mainMesh = new Model(program);

		update(hmap);

		firstPersonPos.x = map.width / 2.0;
		firstPersonPos.y = map.height / 2.0;
		firstPersonPos.z = map.getZ(firstPersonPos.xy, smooth: true);
		firstPersonHDir = 0.0;
		firstPersonVDir = 0.0;

		sw.start();
		lastFpsUpdate = time;
	}

	~this()
	{
		singleton = null;

		sfText_destroy(text);
		sfFont_destroy(font);
		sfView_destroy(view);
		sfRenderWindow_destroy(window);
	}

	void update(const(Heightmap) hmap)
	{
		if (hmap.changeCount == map.changeCount)
			return;

		writeln("Updating visuals");

		map.update(hmap);
		updateMainMesh(hmap);

		map.changeCount = hmap.changeCount;
	}

	private void bindbcError()
	{
		import bindbc.loader.sharedlib;
		foreach(info; errors)
		{
			writefln("%s: %s", info.error.fromStringz, info.message.fromStringz);
		}
	}

	private double time()
	{
		return sw.peek.total!"msecs" / 1000.0;
	}

	private void resetText()
	{
		if (font)
			sfFont_destroy(font);

		font = sfFont_createFromFile("CascadiaMono.ttf");
		if (!font)
			throw new Exception("Could not load font");

		if (text)
			sfText_destroy(text);

		text = sfText_create();
		sfText_setFont(text, font);
		sfText_setPosition(text, sfVector2f(16, 16));
		sfText_setCharacterSize(text, 16);
		sfText_setColor(text, sfBlack);
	}

	private void setTurntableView(double ratio)
	{
		auto model = mat4f.translation(vec3f(-map.width / 2, -map.height / 2, 0));

		model = model.transposed;
		glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &model);

		auto angle = time / 5.0;

		auto view = mat4f.lookAt(
			vec3f(map.width / 2.0 * cos(angle), map.height / 2.0 * sin(angle), max(map.width, map.height) / 2.0),
			vec3f(0, 0, 0),
			vec3f(0.0, 0.0, 1.0)
		);

		auto proj = mat4f.perspective(60.0 / 180.0 * PI, ratio, 100.0, max(map.width, map.height) * 2.0);

		auto PV = (proj * view).transposed;
		glUniformMatrix4fv(glGetUniformLocation(program, "PV"), 1, GL_FALSE, cast(const(GLfloat*)) &PV);
	}

	private void setTopView(double ratio)
	{
		auto model = mat4f.identity;
		glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &model);

		auto proj = mat4f.orthographic(
				map.width / 2.0 - map.height * ratio / 2.0, map.width / 2.0 + map.height * ratio / 2.0,
				0.0, map.height,
				-1e6,
				1e6
			).transposed;

		glUniformMatrix4fv(glGetUniformLocation(program, "PV"), 1, GL_FALSE, cast(const(GLfloat*)) &proj);
	}

	private vec3f forward() const
	{
		return vec3f(
			cos(firstPersonVDir) * cos(firstPersonHDir),
			cos(firstPersonVDir) * sin(firstPersonHDir),
			sin(firstPersonVDir)
		);
	}

	private void setFirstPersonView(double ratio)
	{
		auto speed = 4.0;
		const mouseSpeed = 0.15;

		auto now = time;
		if (lastUpdate.isNaN) lastUpdate = now;

		auto dt = now - lastUpdate;
		lastUpdate = now;

		if (sfRenderWindow_hasFocus(window))
		{
			double xpos, ypos;
			int width, height;

			with (sfRenderWindow_getSize(window))
			{
				width = x;
				height = y;
			}

			with (sfMouse_getPosition(cast(sfWindow*) window))
			{
				xpos = x;
				ypos = y;
			}

			sfMouse_setPosition(sfVector2i(width / 2, height / 2), cast(sfWindow*) window);

			firstPersonHDir -= mouseSpeed * dt * (xpos - width / 2);
			firstPersonVDir -= mouseSpeed * dt * (ypos - height / 2);

			if (firstPersonVDir > PI / 2.0 - 0.1) firstPersonVDir = PI / 2.0 - 0.1;
			if (firstPersonVDir < -PI / 2.0 + 0.1) firstPersonVDir = -PI / 2.0 + 0.1;

			auto previousPos = firstPersonPos;

			if (sfMouse_isButtonPressed(sfMouseLeft))
				speed *= 10.0;
			if (sfMouse_isButtonPressed(sfMouseRight))
				speed *= 100.0;

			auto right = cross(forward, vec3f(0,0,1));

			if (sfKeyboard_isKeyPressed(sfKeyW))
				firstPersonPos += forward * speed * dt;
			if (sfKeyboard_isKeyPressed(sfKeyS))
				firstPersonPos -= forward * speed * dt;
			if (sfKeyboard_isKeyPressed(sfKeyA))
				firstPersonPos -= right * speed * dt;
			if (sfKeyboard_isKeyPressed(sfKeyD))
				firstPersonPos += right * speed * dt;

			if (!map.inBounds(firstPersonPos))
				firstPersonPos = previousPos;

			if (!map.inBounds(previousPos))		// The map probably changed size with a config reload
				firstPersonPos = vec3f(map.width / 2.0, map.height / 2.0, 0.0);
		}

		auto pos = firstPersonPos;
		if (walking)
			pos.z = map.getZ(pos.xy, smooth: true);

		auto camera = pos + vec3f(0, 0, 1.63);
		auto view = mat4f.lookAt(
			camera,
			camera + forward,
			vec3f(0.0, 0.0, 1.0)
		);

		auto proj = mat4f.perspective(60.0 / 180.0 * PI, ratio, 0.1, max(map.width, map.height) * 2.0);
		auto PV = (proj * view).transposed;
		glUniformMatrix4fv(glGetUniformLocation(program, "PV"), 1, GL_FALSE, cast(const(GLfloat*)) &PV);

		auto model = mat4f.identity;
		glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &model);

		currentText ~= "x=%d y=%d z=%d".format(
			pos.x.roundTo!int,
			pos.y.roundTo!int,
			pos.z.roundTo!int);
	}

	bool draw()
	{
		sfEvent event;
		while (sfRenderWindow_pollEvent(window, &event))
		{
			if (event.type == sfEvtClosed)
				sfRenderWindow_close(window);
			else if (event.type == sfEvtKeyPressed)
				onKeyPressed(event.key.code);
		}

		int width, height;
		with(sfRenderWindow_getSize(window))
		{
			width = x;
			height = y;
		}
		auto ratio = width * 1.0 / height;

		glViewport(0, 0, width, height);
		sfView_reset(view, sfFloatRect(0, 0, width, height));
		sfRenderWindow_setView(window, view);

		glClearColor(156.0/255, 196.0/255, 240.0/255, 1.0);
		glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);

		currentText = [];
		glUseProgram(program);

		if (viewMode == 0)
			setTurntableView(ratio);
		else if (viewMode == 1)
			setTopView(ratio);
		else if (viewMode == 2)
			setFirstPersonView(ratio);

		glUniform1f(glGetUniformLocation(program, "lowest"), map.lowest);
		glUniform1f(glGetUniformLocation(program, "highest"), map.highest);
		glUniform1i(glGetUniformLocation(program, "mode"), shadingMode);
		glUniform1i(glGetUniformLocation(program, "lineMode"), lineMode);
		glUniform1f(glGetUniformLocation(program, "waterLevelOverride"), map.lowest);

		glUniform4f(glGetUniformLocation(program, "fpsCenter"), firstPersonPos.x, firstPersonPos.y, firstPersonPos.z,
			viewMode == 2 && cubeMode != 0 ? cubesRadius - 2.0 : 0.0);
		mainMesh.draw();

		if (viewMode == 2 && cubeMode != 0)
		{
			glUniform4f(glGetUniformLocation(program, "fpsCenter"), firstPersonPos.x, firstPersonPos.y, firstPersonPos.z, 0.0);

			auto c = vec2f(firstPersonPos.x.round, firstPersonPos.y.round);
			foreach(dx; - cubesRadius .. cubesRadius)
			foreach(dy; - cubesRadius .. cubesRadius)
			{
				auto pos = c + vec2f(dx, dy);
				if (!map.inBounds(pos) || pos.squaredDistanceTo(c) >= cubesRadius * cubesRadius)
					continue;

				if (dot(forward, vec3f(dx, dy, 0)) < 0)
					continue;

				float z = map.getZ(pos);
				if (cubeMode == 2) z = z.floor;

				auto translation = mat4f.translation(vec3f(pos.xy, z)).transposed;
				glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &translation);
				glUniform1f(glGetUniformLocation(program, "waterLevelOverride"), map.getWaterLevel(pos));

				auto color = map.getOutline(pos);
				glUniform3fv(glGetUniformLocation(program, "colorOverride"), 1, cast(float*) &color);

				cubeMesh.draw();

			}

			currentText ~= "cubesradius=%d".format(cubesRadius);
		}

		currentText ~= "width=%d height=%d minz=%.2f maxz=%.2f".format(
			map.width, map.height,
			map.lowest, map.highest
		);

		framesCount++;
		auto now = time;
		auto diff = now - lastFpsUpdate;
		if (diff >= 1.0)
		{
			fps = framesCount / diff;
			lastFpsUpdate = now;
			framesCount = 0;
		}

		currentText ~= "%.1f fps".format(fps);

		sfRenderWindow_pushGLStates(window);
		sfText_setString(text, currentText.join("\n").toStringz);
		sfRenderWindow_drawText(window, text, null);
		sfRenderWindow_popGLStates(window);

		sfRenderWindow_display(window);
		return sfRenderWindow_isOpen(window) == sfFalse;
	}

	private void updateMainMesh(const(Heightmap) hmap)
	{
		mainMesh.vertices.length = hmap.triangles.length * 3;
		foreach(i, triangle; hmap.triangles)
		{
			const vec3f[3] coords = [
				vec3f(1,0,0),
				vec3f(0,1,0),
				vec3f(0,0,1)
			];

			auto waterTri = triangle.vertices.all!(v => v.isWater);

			foreach (j, vertex; triangle.vertices)
			{
				import config;
				auto color = vertex.terrain ? vertex.terrain.color : vec3d(0,255,255);

				mainMesh.vertices[i * 3 + j] = VertexData(
					vec3f(vertex.pos),
					vec3f(triangle.normal),
					coords[j],
					vec3f(color.r, color.g, color.b) / 255.0,
					waterTri ? vertex.waterLevel : hmap.lowest
				);
			}
		}

		mainMesh.updateData();
	}

	private void onKeyPressed(sfKeyCode code)
	{
		switch (code)
		{
			case sfKeyEscape:
				sfRenderWindow_close(window);
				break;

			case sfKeyTab:
				shadingMode = (shadingMode + 1) % 4;
				break;

			case sfKeyV:
				viewMode = (viewMode + 1) % 3;
				sfRenderWindow_setMouseCursorVisible(window, viewMode != 2);
				break;

			case sfKeyL:
				lineMode = (lineMode + 1) % 4;
				break;

			case sfKeyM:
				cubeMode = (cubeMode + 1) % 3;
				break;

			case sfKeyEnter:
				requestExport = true;
				break;

			case sfKeyUp:
				cubesRadius += 10;
				break;

			case sfKeyDown:
				if (cubesRadius >= 10)
					cubesRadius -= 10;
				break;

			case sfKeySpace:
				walking = !walking;
				break;

			default:
				break;
		}
	}
}
