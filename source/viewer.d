import std.stdio;
import std.string;
import std.exception;
import std.math;
import std.algorithm;

import bindbc.glfw;
import bindbc.opengl;
public import dplug.math;

import gamut;

import heightmap;

alias vec2 = vec2f;
alias vec3 = vec3f;
alias mat4x4 = mat4x4f;

struct Vertex
{
	vec3 pos;
	vec3 norm;
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
out vec3 position;
out vec3 normal;
void main()
{
	gl_Position = MVP * vec4(vPos, 1.0);
	position = vPos;
	normal = vNorm;
}`;
// `


static const char* fragment_shader_text = `
#version 330
uniform float lowest;
uniform float highest;
uniform int mode;
in vec3 position;
in vec3 normal;
out vec4 fragment;
void main()
{
	float z = position.z;

	vec3 color;
	if (mode == 0)
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
	if (mode != 0)
	{
		float sunAngle = dot(normal, vec3(1.0, 1.0, 1.0));
		shading = (sunAngle + 1.0) / 2.0 * 0.7 + 0.3;
	}

	// if (mod(z, 10) <= 0.2)
	// 	shading = 0.0;

	// if (mod(position.x, 10) <= 0.2 || mod(position.y, 10) <= 0.2)
	// 	shading = 0.0;

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

	Model[] models;
	RenderTexture texture;

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
		glfwWindowHint(GLFW_SAMPLES, 4);

		window = glfwCreateWindow(map.width, map.height, title.toStringz, null, null);
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

		texture = new RenderTexture(map.width, map.height);
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

	bool draw(bool toTexture = false)
	{
		int width, height;

		if (toTexture)
		{
			width = texture.width;
			height = texture.height;
			texture.bind();
		}
		else
		{
			glfwGetFramebufferSize(window, &width, &height);
		}
		auto ratio = width * 1.0 / height;

		glViewport(0, 0, width, height);
		glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);

		mat4x4 mvp;

		if (viewMode == 0)
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

			mvp = proj * view * model;
			mvp = mvp.transposed;
		}
		else if (viewMode == 1)
		{
			auto proj = mat4x4.orthographic(
				map.width / 2.0 - map.height * ratio / 2.0, map.width / 2.0 + map.height * ratio / 2.0,
				-map.height, 0.0,
				map.highest * 10.0,
				map.lowest * 10.0
			);

			mvp = proj;
			mvp = mvp.transposed;
		}

		glUseProgram(program);

		glUniformMatrix4fv(glGetUniformLocation(program, "MVP"), 1, GL_FALSE, cast(const(GLfloat*)) &mvp);
		glUniform1f(glGetUniformLocation(program, "lowest"), map.lowest);
		glUniform1f(glGetUniformLocation(program, "highest"), map.highest);
		glUniform1i(glGetUniformLocation(program, "mode"), shadingMode);

		foreach(model; models)
			model.draw();

		if (toTexture)
		{
			texture.unbind();
		}
		else
		{
			glfwSwapBuffers(window);
			glfwPollEvents();
		}

		return glfwWindowShouldClose(window) == GLFW_TRUE;
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
				viewMode = (viewMode + 1) % 2;
				break;

			case GLFW_KEY_ENTER:
				draw(toTexture: true);
				texture.save();
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

		glGenVertexArrays(1, &vertexArray);
		glBindVertexArray(vertexArray);
		glEnableVertexAttribArray(vposLocation);
		glVertexAttribPointer(vposLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.pos.offsetof);
		glEnableVertexAttribArray(vnormLocation);
		glVertexAttribPointer(vnormLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.norm.offsetof);
	}

	~this()
	{
		glDeleteVertexArrays(1, &vertexArray);
		glDeleteBuffers(1, &vertexBuffer);
	}

	void updateData()
	{
		glBindBuffer(GL_ARRAY_BUFFER, vertexBuffer);
		glBufferData(GL_ARRAY_BUFFER, Vertex.sizeof * vertices.length, cast(void*) vertices.ptr, GL_DYNAMIC_DRAW);
	}

	void draw()
	{
		glBindVertexArray(vertexArray);
		glDrawArrays(GL_TRIANGLES, 0, cast(int) vertices.length);
	}
}

class RenderTexture
{
	int width;
	int height;
	GLuint framebuffer;
	GLuint renderedTexture;
	GLuint renderbuffer;
	GLuint depthrenderbuffer;

	ubyte[] data;

	this(int width, int height)
	{
		this.width = width;
		this.height = height;

		data = new ubyte[width * height * 4];

		glGenFramebuffers(1, &framebuffer);
		glBindFramebuffer(GL_FRAMEBUFFER, framebuffer);

		glGenRenderbuffers(1, &renderbuffer);
		glBindRenderbuffer(GL_RENDERBUFFER, renderbuffer);
		glRenderbufferStorage(GL_RENDERBUFFER, GL_RGBA8, width, height);
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

	void save()
	{
		writeln("Saving image");

		bind();
		glReadPixels(0, 0, width, height, GL_RGBA, GL_UNSIGNED_BYTE, data.ptr);
		unbind();

		Image image;
		image.createViewFromData(data.ptr, width, height, PixelType.rgba8, width * 4);
		image.flipVertical();
		image.saveToFile("output.png");
	}
}
