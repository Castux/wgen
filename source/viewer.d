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

alias Vec2 = vec2f;
alias Vec3 = vec3f;
alias Mat4 = mat4x4f;

struct Vertex
{
	Vec3 pos;
	Vec3 norm;
	Vec3 coord;
	Vec3 color;
	float waterLevel;
}

void checkError(string error)
{
	GLint r = glGetError();
	if (r != GL_NO_ERROR)
	{
		throw new Exception(error);
	}
}

static const char* vertex_shader_text = `
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


static const char* fragment_shader_text = `
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

class Viewer
{
	static Viewer singleton;

	sfRenderWindow* window;
	GLuint program;
	bool requestExport;

	Heightmap map;
	Model mainMesh;

	int shadingMode;
	int viewMode;
	int lineMode;

	Vec3 firstPersonPos;
	double firstPersonHDir;
	double firstPersonVDir;
	double lastUpdate;

	int cubeMode;
	Model cubeMesh;

	StopWatch sw;

	this(Heightmap map, string title)
	{
		this.map = map;

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

		setupShaders();

		firstPersonPos.x = map.width / 2.0;
		firstPersonPos.y = map.height / 2.0;
		firstPersonHDir = 0.0;
		firstPersonVDir = 0.0;

		cubeMesh = new Model(program);
		makeCubeMesh();

		mainMesh = new Model(program);
		onMapChanged();

		sw.start();
	}

	~this()
	{
		singleton = null;
		sfRenderWindow_destroy(window);
	}

	private void bindbcError()
	{
		import bindbc.loader.sharedlib;
		foreach(info; errors)
		{
			writefln("%s: %s", info.error.fromStringz, info.message.fromStringz);
		}
	}

	private static void checkShader(GLint shader)
	{
		GLint compiled;
		glGetShaderiv(shader, GL_COMPILE_STATUS, &compiled);
		if (compiled != GL_TRUE)
		{
			GLsizei logLength = 0;
			GLchar[1024] message;
			glGetShaderInfoLog(shader, 1024, &logLength, message.ptr);
			throw new Exception("Shader error: %s".format(message[0 .. logLength]));
		}
	}

	private static void checkProgram(GLint program)
	{
		GLint programLinked;
		glGetProgramiv(program, GL_LINK_STATUS, &programLinked);
		if (programLinked != GL_TRUE)
		{
			GLsizei logLength = 0;
			GLchar[1024] message;
			glGetProgramInfoLog(program, 1024, &logLength, message.ptr);
			throw new Exception("Program error: %s".format(message[0 .. logLength]));
		}
	}

	private void setupShaders()
	{
		GLuint vertex_shader = glCreateShader(GL_VERTEX_SHADER);
		glShaderSource(vertex_shader, 1, &vertex_shader_text, null);
		glCompileShader(vertex_shader);
		checkShader(vertex_shader);

		GLuint fragment_shader = glCreateShader(GL_FRAGMENT_SHADER);
		glShaderSource(fragment_shader, 1, &fragment_shader_text, null);
		glCompileShader(fragment_shader);
		checkShader(fragment_shader);

		program = glCreateProgram();
		glAttachShader(program, vertex_shader);
		glAttachShader(program, fragment_shader);
		glLinkProgram(program);
		checkProgram(program);
	}

	private double time()
	{
		return sw.peek.total!"msecs" / 1000.0;
	}

	private void setTurntableView(double ratio)
	{
		auto model = Mat4.translation(Vec3(-map.width / 2, -map.height / 2, 0));

		model = model.transposed;
		glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &model);

		auto angle = time / 5.0;

		auto view = Mat4.lookAt(
			Vec3(map.width / 2.0 * cos(angle), map.height / 2.0 * sin(angle), max(map.width, map.height) / 2.0),
			Vec3(0, 0, 0),
			Vec3(0.0, 0.0, 1.0)
		);

		auto proj = Mat4.perspective(60.0 / 180.0 * PI, ratio, 100.0, max(map.width, map.height) * 2.0);

		auto PV = (proj * view).transposed;
		glUniformMatrix4fv(glGetUniformLocation(program, "PV"), 1, GL_FALSE, cast(const(GLfloat*)) &PV);
	}

	private void setTopView(double ratio)
	{
		auto model = Mat4.identity;
		glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &model);

		auto proj = Mat4.orthographic(
				map.width / 2.0 - map.height * ratio / 2.0, map.width / 2.0 + map.height * ratio / 2.0,
				0.0, map.height,
				-1e6,
				1e6
			).transposed;

		glUniformMatrix4fv(glGetUniformLocation(program, "PV"), 1, GL_FALSE, cast(const(GLfloat*)) &proj);
	}

	private double lerp(double a, double b, double x)
	{
		return a * (1-x) + b * x;
	}

	double getZ(Vec2 pos, bool waterLevel = false)
	{
		real x, y;
		real xfrac = modf(pos.x, x);
		real yfrac = modf(pos.y, y);

		auto xint = x.lrint;
		auto yint = y.lrint;

		const array = waterLevel ? map.waterLevel : map.heightmap;

		auto z00 = array[(yint + 0) * map.width + (xint + 0)];
		auto z01 = array[(yint + 0) * map.width + (xint + 1)];
		auto z10 = array[(yint + 1) * map.width + (xint + 0)];
		auto z11 = array[(yint + 1) * map.width + (xint + 1)];

		return lerp(
			lerp(z00, z01, xfrac),
			lerp(z10, z11, xfrac),
			yfrac
		);
	}

	private Vec3 forward() const
	{
		return Vec3(
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

			if (sfKeyboard_isKeyPressed(sfKeyLShift))
				speed *= 10.0;
			if (sfKeyboard_isKeyPressed(sfKeyZ))
				speed *= 10.0;

			if (sfKeyboard_isKeyPressed(sfKeyW))
				firstPersonPos += forward * speed * dt;
			if (sfKeyboard_isKeyPressed(sfKeyS))
				firstPersonPos -= forward * speed * dt;

			if (!map.inBounds(firstPersonPos))
				firstPersonPos = previousPos;

			if (!map.inBounds(previousPos))		// The map probably changed size with a config reload
				firstPersonPos = Vec3(map.width / 2.0, map.height / 2.0, 0.0);
		}

		firstPersonPos.z = getZ(firstPersonPos.xy) + 1.62;

		auto view = Mat4.lookAt(
			firstPersonPos,
			firstPersonPos + forward,
			Vec3(0.0, 0.0, 1.0)
		);

		auto proj = Mat4.perspective(60.0 / 180.0 * PI, ratio, 0.1, max(map.width, map.height) * 2.0);
		auto PV = (proj * view).transposed;
		glUniformMatrix4fv(glGetUniformLocation(program, "PV"), 1, GL_FALSE, cast(const(GLfloat*)) &PV);

		auto model = Mat4.identity;
		glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &model);
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
		glClearColor(156.0/255, 196.0/255, 240.0/255, 1.0);
		glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);

		glUseProgram(program);

		if (viewMode == 0)
			setTurntableView(ratio);
		else if (viewMode == 1)
			setTopView(ratio);
		else if (viewMode == 2)
			setFirstPersonView(ratio);

		auto cubesRadius = 175;

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

			auto c = Vec2(firstPersonPos.x.round, firstPersonPos.y.round);
			foreach(dx; - cubesRadius .. cubesRadius)
			foreach(dy; - cubesRadius .. cubesRadius)
			{
				auto pos = c + Vec2(dx, dy);
				if (!map.inBounds(pos) || pos.squaredDistanceTo(c) >= cubesRadius * cubesRadius)
					continue;

				if (dot(forward, Vec3(dx, dy, 0)) < 0)
					continue;

				auto z = getZ(pos);
				if (cubeMode == 2) z = z.floor;

				auto translation = Mat4.translation(Vec3(pos.xy, z)).transposed;
				glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &translation);
				glUniform1f(glGetUniformLocation(program, "waterLevelOverride"), getZ(pos, waterLevel: true));

				auto color = map.getPixel(pos.y.to!int, pos.x.to!int);
				float[3] normalized = [color.r / 255.0, color.g / 255.0, color.b / 255.0];
				glUniform3fv(glGetUniformLocation(program, "colorOverride"), 1, normalized.ptr);

				cubeMesh.draw();
			}
		}

		sfRenderWindow_display(window);
		return sfRenderWindow_isOpen(window) == sfFalse;
	}

	void onMapChanged()
	{
		updateMainMesh();
	}

	private static void makeSquare(Vertex[] vertices, Vec3 a, Vec3 b, Vec3 c, Vec3 d) pure
	{
		auto normal = cross(b - a, c - a).normalized;

		vertices[0] = Vertex(a, normal, coord: Vec3(1,0,0), color: Vec3(-1,-1,-1), waterLevel: float.nan);
		vertices[1] = Vertex(b, normal, coord: Vec3(0,1,0), color: Vec3(-1,-1,-1), waterLevel: float.nan);
		vertices[2] = Vertex(d, normal, coord: Vec3(0,0,1), color: Vec3(-1,-1,-1), waterLevel: float.nan);

		vertices[3] = Vertex(b, normal, coord: Vec3(1,0,0), color: Vec3(-1,-1,-1), waterLevel: float.nan);
		vertices[4] = Vertex(c, normal, coord: Vec3(0,1,0), color: Vec3(-1,-1,-1), waterLevel: float.nan);
		vertices[5] = Vertex(d, normal, coord: Vec3(0,0,1), color: Vec3(-1,-1,-1), waterLevel: float.nan);
	}

	private void makeCubeMesh()
	{
		cubeMesh.vertices.length = 6 * 5;

		auto d = Vec3(-0.5, -0.5, 0.0);
		auto c = Vec3(-0.5, +0.5, 0.0);
		auto b = Vec3(+0.5, +0.5, 0.0);
		auto a = Vec3(+0.5, -0.5, 0.0);

		makeSquare(cubeMesh.vertices[ 0 ..  6], a, b, c, d);
		makeSquare(cubeMesh.vertices[ 6 .. 12], b, a, Vec3(a.xy, -10.0), Vec3(b.xy, -10.0));
		makeSquare(cubeMesh.vertices[12 .. 18], c, b, Vec3(b.xy, -10.0), Vec3(c.xy, -10.0));
		makeSquare(cubeMesh.vertices[18 .. 24], d, c, Vec3(c.xy, -10.0), Vec3(d.xy, -10.0));
		makeSquare(cubeMesh.vertices[24 .. 30], a, d, Vec3(d.xy, -10.0), Vec3(a.xy, -10.0));

		cubeMesh.updateData();
	}

	private void updateMainMesh()
	{
		mainMesh.vertices.length = map.triangles.length * 3;
		foreach(i, triangle; map.triangles)
		{
			const Vec3[3] coords = [
				Vec3(1,0,0),
				Vec3(0,1,0),
				Vec3(0,0,1)
			];

			auto waterTri = triangle.vertices.all!"a.isWater";

			foreach (j, vertex; triangle.vertices)
			{
				import config;
				auto color = vertex.terrain ? vertex.terrain.color : Pixel(0,255,255);

				mainMesh.vertices[i * 3 + j] = Vertex(
					Vec3(vertex.pos),
					Vec3(triangle.normal),
					coords[j],
					Vec3(color.r, color.g, color.b) / 255.0,
					waterTri ? vertex.waterLevel : map.lowest
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

			default:
				break;
		}
	}
}

class Model
{
	Vertex[] vertices;

	GLuint vertexBuffer;
	GLuint vertexArray;

	this(GLuint program)
	{
		glGenBuffers(1, &vertexBuffer);
		glBindBuffer(GL_ARRAY_BUFFER, vertexBuffer);

		GLint vposLocation = glGetAttribLocation(program, "vPos");
		GLint vnormLocation = glGetAttribLocation(program, "vNorm");
		GLint vcoordLocation = glGetAttribLocation(program, "vCoord");
		GLint vcolorLocation = glGetAttribLocation(program, "vColor");
		GLint vwaterLevelLocation = glGetAttribLocation(program, "vWaterLevel");

		glGenVertexArrays(1, &vertexArray);
		glBindVertexArray(vertexArray);
		glEnableVertexAttribArray(vposLocation);
		glVertexAttribPointer(vposLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.pos.offsetof);
		glEnableVertexAttribArray(vnormLocation);
		glVertexAttribPointer(vnormLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.norm.offsetof);
		glEnableVertexAttribArray(vcoordLocation);
		glVertexAttribPointer(vcoordLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.coord.offsetof);
		glEnableVertexAttribArray(vcolorLocation);
		glVertexAttribPointer(vcolorLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.color.offsetof);
		glEnableVertexAttribArray(vwaterLevelLocation);
		glVertexAttribPointer(vwaterLevelLocation, 1, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.waterLevel.offsetof);
	}

	~this()
	{
		glDeleteVertexArrays(1, &vertexArray);
		glDeleteBuffers(1, &vertexBuffer);
	}

	void updateData()
	{
		glBindBuffer(GL_ARRAY_BUFFER, vertexBuffer);
		glBufferData(GL_ARRAY_BUFFER, Vertex.sizeof * vertices.length, cast(void*) vertices.ptr, GL_STATIC_DRAW);
	}

	void draw()
	{
		glBindVertexArray(vertexArray);
		glDrawArrays(GL_TRIANGLES, 0, cast(int) vertices.length);
	}
}
