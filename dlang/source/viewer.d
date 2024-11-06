import std.stdio;
import std.string;
import std.exception;
import std.math;

import bindbc.glfw;
import bindbc.opengl;
import dplug.math;

alias vec2 = vec2f;
alias vec3 = vec3f;
alias mat4x4 = mat4x4f;

struct Vertex
{
	vec3 pos;
	vec3 col;
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

static const char* vertex_shader_text = `
#version 330
uniform mat4 MVP;
in vec3 vCol;
in vec3 vPos;
out vec3 color;
void main()
{
	gl_Position = MVP * vec4(vPos, 1.0);
	color = vCol;
}`;
// `


static const char* fragment_shader_text = `
#version 330
in vec3 color;
out vec4 fragment;
void main()
{
	fragment = vec4(color, 1.0);
}`;
// `

class Viewer
{
	static Viewer singleton;

	GLFWwindow* window;
	GLuint program;

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

	void setupShaders()
	{
		GLuint vertex_shader = glCreateShader(GL_VERTEX_SHADER);
		glShaderSource(vertex_shader, 1, &vertex_shader_text, null);
		glCompileShader(vertex_shader);

		GLuint fragment_shader = glCreateShader(GL_FRAGMENT_SHADER);
		glShaderSource(fragment_shader, 1, &fragment_shader_text, null);
		glCompileShader(fragment_shader);

		program = glCreateProgram();
		glAttachShader(program, vertex_shader);
		glAttachShader(program, fragment_shader);
		glLinkProgram(program);
	}

	bool run()
	{
		while (!glfwWindowShouldClose(window))
		{
			int width, height;
			glfwGetFramebufferSize(window, &width, &height);
			float ratio = width / cast(float) height;

			glViewport(0, 0, width, height);
			glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);

			auto mod =
				mat4x4.rotateZ(cast(float) glfwGetTime() / 5.0) *
				mat4x4.translation(vec3(-1000, 1000, 0));

			auto s = 2000.0;

			auto view = mat4x4.lookAt(vec3(1000, -1000, 1000), vec3(0, 0, 0), vec3(0.0, 0.0, 1.0));
			//auto proj = mat4x4.orthographic(-1000 * ratio, 1000 * ratio, -1000, 1000, 4000.0, -4000.0);
			auto proj = mat4x4.perspective(60.0 / 180.0 * PI, ratio, 100.0, 4000.0);

			auto mvp = proj * view * mod;
			mvp = mvp.transposed;

			glUseProgram(program);
			GLint mvpLocation = glGetUniformLocation(program, "MVP");
			glUniformMatrix4fv(mvpLocation, 1, GL_FALSE, cast(const(GLfloat*)) &mvp);

			foreach(model; models)
				model.draw();

			glfwSwapBuffers(window);
			glfwPollEvents();
		}

		return true;
	}

	void onKeyEvent(int key, int scancode, int action, int mods)
	{
		if (key == GLFW_KEY_ESCAPE && action == GLFW_PRESS)
			glfwSetWindowShouldClose(window, GLFW_TRUE);

		else if (key == GLFW_KEY_SPACE && action == GLFW_PRESS)
		{
			auto model = models[0];
			model.vertices[1].pos.x += 0.1;
			model.updateData();
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
		GLint vcolLocation = glGetAttribLocation(program, "vCol");

		glGenVertexArrays(1, &vertexArray);
		glBindVertexArray(vertexArray);
		glEnableVertexAttribArray(vposLocation);
		glVertexAttribPointer(vposLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.pos.offsetof);
		glEnableVertexAttribArray(vcolLocation);
		glVertexAttribPointer(vcolLocation, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.col.offsetof);
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
