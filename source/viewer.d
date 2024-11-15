import std.stdio;
import std.string;
import std.exception;
import std.math;
import std.algorithm;
import std.conv;

import bindbc.glfw;
import bindbc.opengl;
public import dplug.math;

import heightmap;

alias vec2 = vec2f;
alias vec3 = vec3f;
alias mat4x4 = mat4x4f;

struct Vertex
{
	vec3 pos;
	vec3 norm;
	vec3 coord;
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
uniform mat4 MVP;
in vec3 vNorm;
in vec3 vPos;
in vec3 vCoord;
out vec3 position;
out vec3 normal;
out vec3 coord;
void main()
{
	gl_Position = MVP * vec4(vPos, 1.0);
	position = vPos;
	normal = vNorm;
	coord = vCoord;
}`;
// `


static const char* fragment_shader_text = `
#version 330
uniform float lowest;
uniform float highest;
uniform int mode;
uniform int lineMode;
in vec3 position;
in vec3 normal;
in vec3 coord;
out vec4 fragment;
void main()
{
	float z = position.z;

	vec3 color;
	if (mode == -1)
	{
		color = vec3(z, z, z);
	}
	else if (mode == 0)
	{
		float f = (z - lowest) / (highest - lowest);
		color = vec3(f, f, f);
	}
	else if (mode == 1)
	{
		color = vec3(1.0, 1.0, 1.0);
	}
	else if (mode == 2)
	{
		if (z >= 0)
		{
			float f = z / highest;
			color = mix(vec3(84, 169, 50), vec3(255, 255, 255), f) / 255.0;
		}
		else
		{
			float f = z / lowest;
			color = mix(vec3(95, 132, 255), vec3(0, 10, 100), f) / 255.0;
		}
	}

	float shading = 1.0;
	if (mode > 0)
	{
		float sunAngle = dot(normal, vec3(1.0, 1.0, 1.0));
		shading = (sunAngle + 1.0) / 2.0 * 0.7 + 0.3;
	}

	if (lineMode == 1)
	{
		shading = shading * step(0.25, mod(z, 10));
	}
	else if (lineMode == 2)
	{
		shading = shading * step(0.075, min(mod(position.x, 1), mod(position.y, 1)));
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

	Heightmap map;

	int shadingMode;
	int viewMode;
	int lineMode;

	Model[] models;
	RenderBuffer renderBuffer;

	float[] interpolatedHeightmap;
	vec3 firstPersonPos;
	double firstPersonDir;
	double lastUpdate;

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

		if (singleton)
			throw new Exception("Multiple viewer instances");

		glEnable(GL_DEPTH_TEST);

		singleton = this;

		setupShaders();

		renderBuffer = new RenderBuffer(map.width, map.height);

		firstPersonPos.x = map.width / 2.0;
		firstPersonPos.y = -map.height / 2.0;
		firstPersonDir = 0.0;
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

	private mat4x4 getTurntableView(double ratio)
	{
		auto model =
			mat4x4.rotateZ(cast(float) glfwGetTime() / 5.0) *
			mat4x4.translation(vec3(-map.width / 2, map.height / 2, 0));

		auto view = mat4x4.lookAt(
			vec3(map.width / 2.0, -map.height / 2.0, max(map.width, map.height) / 2.0),
			vec3(0, 0, 0),
			vec3(0.0, 0.0, 1.0)
		);

		auto proj = mat4x4.perspective(60.0 / 180.0 * PI, ratio, 100.0, max(map.width, map.height) * 2.0);
		return (proj * view * model).transposed;
	}

	private mat4x4 getTopView(double ratio)
	{
		auto proj = mat4x4.orthographic(
				map.width / 2.0 - map.height * ratio / 2.0, map.width / 2.0 + map.height * ratio / 2.0,
				-map.height, 0.0,
				-(map.highest + 10.0),
				(map.highest - map.lowest) + 20.0
			);

		return proj.transposed;
	}

	private double lerp(double a, double b, double x)
	{
		return a * (1-x) + b * x;
	}

	double getZ(vec2 pos)
	{
		real x, y;
		real xfrac = modf(firstPersonPos.x, x);
		real yfrac = modf(-firstPersonPos.y, y);

		auto xint = x.lrint;
		auto yint = y.lrint;

		auto z00 = interpolatedHeightmap[(map.height - (yint + 0)) * map.width + (xint + 0)];
		auto z01 = interpolatedHeightmap[(map.height - (yint + 0)) * map.width + (xint + 1)];
		auto z10 = interpolatedHeightmap[(map.height - (yint + 1)) * map.width + (xint + 0)];
		auto z11 = interpolatedHeightmap[(map.height - (yint + 1)) * map.width + (xint + 1)];

		return lerp(
			lerp(z00, z01, xfrac),
			lerp(z10, z11, xfrac),
			yfrac
		);
	}

	private mat4x4 getFirstPersonView(double ratio)
	{
		auto speed = 4.0;
		const rot = 0.75;

		auto now = glfwGetTime();
		auto dt = now - lastUpdate;
		lastUpdate = now;

		if (glfwGetKey(window, GLFW_KEY_LEFT) == GLFW_PRESS)
			firstPersonDir -= rot * dt;
		if (glfwGetKey(window, GLFW_KEY_RIGHT) == GLFW_PRESS)
			firstPersonDir += rot * dt;

		auto forward = vec3(cos(firstPersonDir), -sin(firstPersonDir), 0.0);
		auto previousPos = firstPersonPos;

		if (glfwGetKey(window, GLFW_KEY_LSHIFT) == GLFW_PRESS)
			speed *= 10.0;
		if (glfwGetKey(window, GLFW_KEY_Z) == GLFW_PRESS)
			speed *= 10.0;

		if (glfwGetKey(window, GLFW_KEY_UP) == GLFW_PRESS)
			firstPersonPos += forward * speed * dt;
		if (glfwGetKey(window, GLFW_KEY_DOWN) == GLFW_PRESS)
			firstPersonPos -= forward * speed * dt;

		if (!map.inBounds(vec2d(firstPersonPos.x, -firstPersonPos.y)))
			firstPersonPos = previousPos;

		firstPersonPos.z = getZ(firstPersonPos.xy) + 1.75;

		auto view = mat4x4.lookAt(
			firstPersonPos,
			firstPersonPos + forward,
			vec3(0.0, 0.0, 1.0)
		);

		auto proj = mat4x4.perspective(60.0 / 180.0 * PI, ratio, 0.1, max(map.width, map.height) * 2.0);
		return (proj * view).transposed;
	}

	bool draw()
	{
		int width, height;
		glfwGetFramebufferSize(window, &width, &height);
		auto ratio = width * 1.0 / height;

		glViewport(0, 0, width, height);
		glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);

		mat4x4 mvp;

		if (viewMode == 0)
			mvp = getTurntableView(ratio);
		else if (viewMode == 1)
			mvp = getTopView(ratio);
		else if (viewMode == 2)
			mvp = getFirstPersonView(ratio);

		glUseProgram(program);

		glUniformMatrix4fv(glGetUniformLocation(program, "MVP"), 1, GL_FALSE, cast(const(GLfloat*)) &mvp);
		glUniform1f(glGetUniformLocation(program, "lowest"), map.lowest);
		glUniform1f(glGetUniformLocation(program, "highest"), map.highest);
		glUniform1i(glGetUniformLocation(program, "mode"), shadingMode);
		glUniform1i(glGetUniformLocation(program, "lineMode"), lineMode);

		foreach(model; models)
			model.draw();

		glfwSwapBuffers(window);
		glfwPollEvents();

		return glfwWindowShouldClose(window) == GLFW_TRUE;
	}

	float[] generateInterpolatedHeightmap()
	{
		int width = renderBuffer.width;
		int height = renderBuffer.height;

		renderBuffer.bind();

		glViewport(0, 0, width, height);
		glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);

		auto proj = mat4x4.orthographic(
			0.0, map.width,
			-map.height, 0.0,
			map.highest * 10.0,
			map.lowest * 10.0
		).transposed;

		glUseProgram(program);

		glUniformMatrix4fv(glGetUniformLocation(program, "MVP"), 1, GL_FALSE, cast(const(GLfloat*)) &proj);
		glUniform1i(glGetUniformLocation(program, "mode"), -1);
		glUniform1i(glGetUniformLocation(program, "lineMode"), 0);

		foreach(model; models)
			model.draw();

		interpolatedHeightmap.length = width * height;
		glReadPixels(0, 0, width, height, GL_RED, GL_FLOAT, interpolatedHeightmap.ptr);

		renderBuffer.unbind();

		return interpolatedHeightmap;
	}

	void onKeyEvent(int key, int scancode, int action, int mods)
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
				break;

			case GLFW_KEY_L:
				lineMode = (lineMode + 1) % 4;
				break;

			default:
				break;
		}
	}

	Model newModel()
	{
		Model model = new Model(program);
		models ~= model;

		return model;
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

		glGenVertexArrays(1, &vertexArray);
		glBindVertexArray(vertexArray);
		glEnableVertexAttribArray(vposLocation);
		glVertexAttribPointer(vposLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.pos.offsetof);
		glEnableVertexAttribArray(vnormLocation);
		glVertexAttribPointer(vnormLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.norm.offsetof);
		glEnableVertexAttribArray(vcoordLocation);
		glVertexAttribPointer(vcoordLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.coord.offsetof);
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

	float[] data;

	this(int width, int height)
	{
		this.width = width;
		this.height = height;

		data = new float[width * height * 1];

		glGenFramebuffers(1, &framebuffer);
		glBindFramebuffer(GL_FRAMEBUFFER, framebuffer);

		GLuint renderbuffer;
		GLuint depthrenderbuffer;

		glGenRenderbuffers(1, &renderbuffer);
		glBindRenderbuffer(GL_RENDERBUFFER, renderbuffer);
		glRenderbufferStorage(GL_RENDERBUFFER, GL_R32F, width, height);
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

	void bind()
	{
		glBindFramebuffer(GL_FRAMEBUFFER, framebuffer);
	}

	void unbind()
	{
		glBindFramebuffer(GL_FRAMEBUFFER, 0);
	}
}
