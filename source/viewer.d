import std.stdio;
import std.string;
import std.exception;
import std.math;
import std.algorithm;

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
uniform int colorMode;
uniform int lightMode;
in vec3 position;
in vec3 normal;
out vec4 fragment;
void main()
{
	float z = position.z;

	vec3 color;
	if (colorMode == 0)
	{
		color = vec3(1.0, 1.0, 1.0);
	}
	else if (colorMode == 1)
	{
		float f = (z - lowest) / (highest - lowest);
		color = vec3(f, f, f);
	}
	else if (colorMode == 2)
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
	if (lightMode == 1)
	{
		float sunAngle = dot(normal, vec3(1.0, 1.0, 1.0));
		shading = (sunAngle + 1.0) / 2.0 * 0.7 + 0.3;
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

	int colorMode;
	int lightMode;
	int viewMode;

	Model[] models;

	this(int w, int h, string title)
	{
		if(loadGLFW() != glfwSupport)
			throw new Exception("Could not load GLFW library");

		if (!glfwInit())
			throw new Exception("Could not initialize GLFW");

		glfwSetErrorCallback(&errorCallback);

		glfwWindowHint(GLFW_CONTEXT_VERSION_MAJOR, 3);
		glfwWindowHint(GLFW_CONTEXT_VERSION_MINOR, 3);
		glfwWindowHint(GLFW_OPENGL_PROFILE, GLFW_OPENGL_CORE_PROFILE);
		glfwWindowHint(GLFW_SAMPLES, 4);

		window = glfwCreateWindow(w, h, title.toStringz, null, null);
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

	bool draw(Heightmap map)
	{
		int width, height;
		glfwGetFramebufferSize(window, &width, &height);
		float ratio = width / cast(float) height;

		glViewport(0, 0, width, height);
		glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);

		mat4x4 mvp;

		if (viewMode == 0)
		{
			auto model =
				mat4x4.rotateZ(cast(float) glfwGetTime() / 5.0) *
				mat4x4.translation(vec3(-map.width / 2, map.height / 2, 0));

			auto s = 2000.0;

			auto view = mat4x4.lookAt(
				vec3(map.width / 2.0, -map.height / 2.0, max(map.width, map.height) / 2.0),
				vec3(0, 0, 0),
				vec3(0.0, 0.0, 1.0)
			);

			//auto proj = mat4x4.orthographic(-1000 * ratio, 1000 * ratio, -1000, 1000, 4000.0, -4000.0);
			auto proj = mat4x4.perspective(60.0 / 180.0 * PI, ratio, 100.0, max(map.width, map.height) * 2.0);

			mvp = proj * view * model;
			mvp = mvp.transposed;
		}



		glUseProgram(program);

		glUniformMatrix4fv(glGetUniformLocation(program, "MVP"), 1, GL_FALSE, cast(const(GLfloat*)) &mvp);
		glUniform1f(glGetUniformLocation(program, "lowest"), map.lowest);
		glUniform1f(glGetUniformLocation(program, "highest"), map.highest);
		glUniform1i(glGetUniformLocation(program, "colorMode"), colorMode);
		glUniform1i(glGetUniformLocation(program, "lightMode"), lightMode);

		foreach(model; models)
			model.draw();

		glfwSwapBuffers(window);
		glfwPollEvents();

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

			case GLFW_KEY_C:
				colorMode = (colorMode + 1) % 3;
				break;

			case GLFW_KEY_L:
				lightMode = (lightMode + 1) % 2;
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
