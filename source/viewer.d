import std.stdio;
import std.string;
import std.exception;
import std.math;
import std.algorithm;
import std.conv;
import std.range;
import std.parallelism;

import bindbc.glfw;
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
	float waterLevel;
}

extern(C) nothrow void errorCallback(int error, const(char)* description)
{
	import core.stdc.stdio;
	printf("Error: %s\n", description);
}

extern(C) nothrow void keyCallback(GLFWwindow* window, int key, int scancode, int action, int mods)
{
	if (Viewer.singleton && Viewer.singleton.window == window)
		assumeWontThrow(Viewer.singleton.onKeyEvent(key, scancode, action, mods));
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
in vec3 vNorm;
in vec3 vPos;
in vec3 vCoord;
in float vWaterLevel;
out vec3 worldPos;
out vec3 normal;
out vec3 coord;
out float waterLevel;
void main()
{
	vec4 pos = M * vec4(vPos, 1.0);
	gl_Position = PV * pos;
	normal = vNorm;
	coord = vCoord;
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

	if (mode == -1)
	{
		color = vec3(z, waterLevel, 0.0);
	}
	else if (mode == 0)
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

	GLFWwindow* window;
	GLuint program;
	bool requestExport;

	Heightmap map;
	Model mainMesh;

	int shadingMode;
	int viewMode;
	int lineMode;
	bool smoothNormals;

	RenderBuffer renderBuffer;

	float[] interpolatedHeightmap;
	float[] blurredHeightmap;
	float[] waterLevelHeightmap;
	bool useBlurred;

	Vec3 firstPersonPos;
	double firstPersonHDir;
	double firstPersonVDir;
	double lastUpdate;

	int cubeMode;
	Model cubeMesh;

	this(Heightmap map, string title)
	{
		this.map = map;

		if(loadGLFW() != glfwSupport)
			throw new Exception("Could not load GLFW library");

		if (!glfwInit())
			throw new Exception("Could not initialize GLFW");

		glfwSetErrorCallback(&errorCallback);

		glfwWindowHint(GLFW_CONTEXT_VERSION_MAJOR, 3);
		glfwWindowHint(GLFW_CONTEXT_VERSION_MINOR, 3);
		glfwWindowHint(GLFW_OPENGL_PROFILE, GLFW_OPENGL_CORE_PROFILE);
		glfwWindowHint(GLFW_SAMPLES, 2);

		window = glfwCreateWindow(1024, 768, title.toStringz, null, null);
		if (!window)
			throw new Exception("Could not create window");

		glfwMakeContextCurrent(window);
		glfwSetKeyCallback(window, &keyCallback);
		glfwSwapInterval(1);

		if(loadOpenGL() != GLSupport.gl33)
			throw new Exception("Could not load OpenGL library");
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
	}

	~this()
	{
		singleton = null;

		glfwDestroyWindow(window);
		glfwTerminate();
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

	private void setTurntableView(double ratio)
	{
		auto model = Mat4.translation(Vec3(-map.width / 2, -map.height / 2, 0));

		model = model.transposed;
		glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &model);

		auto angle = glfwGetTime() / 5.0;

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

		auto array =
			waterLevel ? waterLevelHeightmap :
			useBlurred ? blurredHeightmap :
			interpolatedHeightmap;

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

		auto now = glfwGetTime();
		if (lastUpdate.isNaN) lastUpdate = now;

		auto dt = now - lastUpdate;
		lastUpdate = now;

		double xpos, ypos;
		int width, height;
		glfwGetCursorPos(window, &xpos, &ypos);
		glfwGetWindowSize(window, &width, &height);
		glfwSetCursorPos(window, width / 2, height / 2);

		firstPersonHDir -= mouseSpeed * dt * (xpos - width / 2);
		firstPersonVDir -= mouseSpeed * dt * (ypos - height / 2);

		if (firstPersonVDir > PI / 2.0 - 0.1) firstPersonVDir = PI / 2.0 - 0.1;
		if (firstPersonVDir < -PI / 2.0 + 0.1) firstPersonVDir = -PI / 2.0 + 0.1;

		auto previousPos = firstPersonPos;

		if (glfwGetKey(window, GLFW_KEY_LSHIFT) == GLFW_PRESS)
			speed *= 10.0;
		if (glfwGetKey(window, GLFW_KEY_Z) == GLFW_PRESS)
			speed *= 10.0;

		if (glfwGetKey(window, GLFW_KEY_W) == GLFW_PRESS)
			firstPersonPos += forward * speed * dt;
		if (glfwGetKey(window, GLFW_KEY_S) == GLFW_PRESS)
			firstPersonPos -= forward * speed * dt;

		if (!map.inBounds(firstPersonPos))
			firstPersonPos = previousPos;

		if (!map.inBounds(previousPos))		// The map probably changed size with a config reload
			firstPersonPos = Vec3(map.width / 2.0, map.height / 2.0, 0.0);

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
		int width, height;
		glfwGetFramebufferSize(window, &width, &height);
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
				if (cubeMode == 2) z = z.round;

				auto translation = Mat4.translation(Vec3(pos.xy, z)).transposed;
				glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &translation);
				glUniform1f(glGetUniformLocation(program, "waterLevelOverride"), getZ(pos, waterLevel: true));

				cubeMesh.draw();
			}
		}

		glfwSwapBuffers(window);
		glfwPollEvents();

		return glfwWindowShouldClose(window) == GLFW_TRUE;
	}

	void onMapChanged()
	{
		updateMainMesh();

		if (renderBuffer is null || renderBuffer.width != map.width || renderBuffer.height != map.height)
			renderBuffer = new RenderBuffer(map.width, map.height);

		generateInterpolatedHeightmap();
		blurHeightmap();
	}

	private void generateInterpolatedHeightmap()
	{
		int width = renderBuffer.width;
		int height = renderBuffer.height;

		renderBuffer.bind();

		glUseProgram(program);
		glViewport(0, 0, width, height);
		glClearColor(0.0, 0.0, 0.0, 1.0);
		glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);

		auto model = Mat4.identity;
		glUniformMatrix4fv(glGetUniformLocation(program, "M"), 1, GL_FALSE, cast(const(GLfloat*)) &model);

		auto proj = Mat4.orthographic(
			0.0, map.width,
			0.0, map.height,
			-1e6,
			1e6
		).transposed;

		glUniformMatrix4fv(glGetUniformLocation(program, "PV"), 1, GL_FALSE, cast(const(GLfloat*)) &proj);
		glUniform1i(glGetUniformLocation(program, "mode"), -1);
		glUniform1i(glGetUniformLocation(program, "lineMode"), 0);
		glUniform1f(glGetUniformLocation(program, "waterLevelOverride"), map.lowest);

		mainMesh.draw();

		interpolatedHeightmap = new float[width * height];
		glReadPixels(0, 0, width, height, GL_RED, GL_FLOAT, interpolatedHeightmap.ptr);
		writefln("Heightmap: low %f, high %f", interpolatedHeightmap.minElement, interpolatedHeightmap.maxElement);

		waterLevelHeightmap = new float[width * height];
		glReadPixels(0, 0, width, height, GL_GREEN, GL_FLOAT, waterLevelHeightmap.ptr);
		writefln("Water level: low %f, high %f", waterLevelHeightmap.minElement, waterLevelHeightmap.maxElement);

		renderBuffer.unbind();
	}

	private static int[] binomialCoefs(int order) pure
	{
		int[] coefs = [1];
		foreach(k; 1 .. order + 1)
			coefs ~= coefs[$ - 1] * (order + 1 - k) / k;

		return coefs;
	}

	private void blurHeightmap()
	{
		const radius = map.conf.blurRadius;
		if (radius == 0)
		{
			blurredHeightmap = interpolatedHeightmap.dup;
			return;
		}

		auto tmp = new float[interpolatedHeightmap.length];
		blurredHeightmap = new float[interpolatedHeightmap.length];

		auto coefs = binomialCoefs(2 * radius)[radius .. $];

		foreach(row; iota(0, map.height).array.parallel)
		foreach(col; 0 .. map.width)
		{
			double sum = 0.0;
			int coefsum = 0;

			foreach(dcol; -radius .. radius + 1)
			{
				auto c = col + dcol;
				if (c < 0 || c >= map.width) continue;
				sum += interpolatedHeightmap[row * map.width + c] * coefs[dcol.abs];
				coefsum += coefs[dcol.abs];
			}

			tmp[row * map.width + col] = sum / coefsum;
		}

		foreach(col; iota(0, map.width).array.parallel)
		foreach(row; 0 .. map.height)
		{
			double sum = 0.0;
			int coefsum = 0;

			foreach(drow; -radius .. radius + 1)
			{
				auto r = row + drow;
				if (r < 0 || r >= map.height) continue;
				sum += tmp[r * map.width + col] * coefs[drow.abs];
				coefsum += coefs[drow.abs];
			}

			blurredHeightmap[row * map.width + col] = sum / coefsum;
		}
	}

	private static void makeSquare(Vertex[] vertices, Vec3 a, Vec3 b, Vec3 c, Vec3 d) pure
	{
		auto normal = cross(b - a, c - a).normalized;

		vertices[0] = Vertex(a, normal, Vec3(1,0,0), float.nan);
		vertices[1] = Vertex(b, normal, Vec3(0,1,0), float.nan);
		vertices[2] = Vertex(d, normal, Vec3(0,0,1), float.nan);

		vertices[3] = Vertex(b, normal, Vec3(1,0,0), float.nan);
		vertices[4] = Vertex(c, normal, Vec3(0,1,0), float.nan);
		vertices[5] = Vertex(d, normal, Vec3(0,0,1), float.nan);
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
				mainMesh.vertices[i * 3 + j] = Vertex(
					Vec3(vertex.pos),
					Vec3(triangle.normal),
					coords[j],
					waterTri ? vertex.waterLevel : map.lowest
				);
			}
		}

		mainMesh.updateData();
	}

	private void onKeyEvent(int key, int scancode, int action, int mods)
	{
		if (action != GLFW_PRESS)
			return;

		switch (key)
		{
			case GLFW_KEY_ESCAPE:
				glfwSetWindowShouldClose(window, GLFW_TRUE);
				break;

			case GLFW_KEY_TAB:
				shadingMode = (shadingMode + 1) % 3;
				break;

			case GLFW_KEY_V:
				viewMode = (viewMode + 1) % 3;
				glfwSetInputMode(window, GLFW_CURSOR, viewMode == 2 ? GLFW_CURSOR_DISABLED : GLFW_CURSOR_NORMAL);
				break;

			case GLFW_KEY_L:
				lineMode = (lineMode + 1) % 4;
				break;

			case GLFW_KEY_M:
				cubeMode = (cubeMode + 1) % 3;
				break;

			case GLFW_KEY_B:
				useBlurred = !useBlurred;
				break;

			case GLFW_KEY_ENTER:
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
		GLint vwaterLevelLocation = glGetAttribLocation(program, "vWaterLevel");

		glGenVertexArrays(1, &vertexArray);
		glBindVertexArray(vertexArray);
		glEnableVertexAttribArray(vposLocation);
		glVertexAttribPointer(vposLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.pos.offsetof);
		glEnableVertexAttribArray(vnormLocation);
		glVertexAttribPointer(vnormLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.norm.offsetof);
		glEnableVertexAttribArray(vcoordLocation);
		glVertexAttribPointer(vcoordLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.coord.offsetof);
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

class RenderBuffer
{
	int width;
	int height;
	GLuint framebuffer;

	GLuint renderbuffer;
	GLuint depthrenderbuffer;

	this(int width, int height)
	{
		this.width = width;
		this.height = height;

		glGenFramebuffers(1, &framebuffer);
		glBindFramebuffer(GL_FRAMEBUFFER, framebuffer);

		glGenRenderbuffers(1, &renderbuffer);
		glBindRenderbuffer(GL_RENDERBUFFER, renderbuffer);
		glRenderbufferStorage(GL_RENDERBUFFER, GL_RG32F, width, height);
		glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, renderbuffer);

		glGenRenderbuffers(1, &depthrenderbuffer);
		glBindRenderbuffer(GL_RENDERBUFFER, depthrenderbuffer);
		glRenderbufferStorage(GL_RENDERBUFFER, GL_DEPTH_COMPONENT, width, height);
		glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_DEPTH_ATTACHMENT, GL_RENDERBUFFER, depthrenderbuffer);

		if (glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE)
			throw new Exception("Couldn't set up render to texture");

		glBindRenderbuffer(GL_RENDERBUFFER, 0);
		glBindFramebuffer(GL_FRAMEBUFFER, 0);
	}

	~this()
	{
		glDeleteFramebuffers(1, &framebuffer);
		glDeleteRenderbuffers(1, &renderbuffer);
		glDeleteRenderbuffers(1, &depthrenderbuffer);
	}

	void bind()
	{
		glBindFramebuffer(GL_FRAMEBUFFER, framebuffer);
	}

	void unbind()
	{
		glBindFramebuffer(GL_FRAMEBUFFER, 0);
	}
}
