import std.stdio;
import std.string;
import std.exception;

import bindbc.glfw;
import bindbc.opengl;
import dplug.math;

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

alias vec2 = vec2f;
alias vec3 = vec3f;
alias mat4x4 = mat4x4f;

struct Vertex
{
	vec2 pos;
	vec3 col;
}

static const Vertex[] vertices =
[
	Vertex( vec2(-0.6, -0.4), vec3(1.0, 0.0, 0.0) ),
	Vertex( vec2( 0.6, -0.4), vec3(0.0, 1.0, 0.0) ),
	Vertex( vec2( 0.0,  0.6), vec3(0.0, 0.0, 1.0) )
];

static const char* vertex_shader_text = `
#version 330
uniform mat4 MVP;
in vec3 vCol;
in vec2 vPos;
out vec3 color;
void main()
{
	gl_Position = MVP * vec4(vPos, 0.0, 1.0);
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
		
		singleton = this;
	}

	~this()
	{
		singleton = null;

		glfwDestroyWindow(window);
		glfwTerminate();
	}

	bool run()
	{
		GLuint vertex_buffer;
		glGenBuffers(1, &vertex_buffer);
		glBindBuffer(GL_ARRAY_BUFFER, vertex_buffer);
		glBufferData(GL_ARRAY_BUFFER, Vertex.sizeof * vertices.length, cast(void*) vertices.ptr, GL_STATIC_DRAW);

		GLuint vertex_shader = glCreateShader(GL_VERTEX_SHADER);
		glShaderSource(vertex_shader, 1, &vertex_shader_text, null);
		glCompileShader(vertex_shader);

		GLuint fragment_shader = glCreateShader(GL_FRAGMENT_SHADER);
		glShaderSource(fragment_shader, 1, &fragment_shader_text, null);
		glCompileShader(fragment_shader);

		GLuint program = glCreateProgram();
		glAttachShader(program, vertex_shader);
		glAttachShader(program, fragment_shader);
		glLinkProgram(program);

		GLint mvp_location = glGetUniformLocation(program, "MVP");
		GLint vpos_location = glGetAttribLocation(program, "vPos");
		GLint vcol_location = glGetAttribLocation(program, "vCol");

		GLuint vertex_array;
		glGenVertexArrays(1, &vertex_array);
		glBindVertexArray(vertex_array);
		glEnableVertexAttribArray(vpos_location);
		glVertexAttribPointer(vpos_location, 2, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.pos.offsetof);
		glEnableVertexAttribArray(vcol_location);
		glVertexAttribPointer(vcol_location, 3, GL_FLOAT, GL_FALSE, Vertex.sizeof, cast(void*) Vertex.col.offsetof);

		while (!glfwWindowShouldClose(window))
		{
			int width, height;
			glfwGetFramebufferSize(window, &width, &height);
			float ratio = width / cast(float) height;

			glViewport(0, 0, width, height);
			glClear(GL_COLOR_BUFFER_BIT);

			auto m = mat4x4.identity;
			m = m.rotateZ(cast(float) glfwGetTime());

			auto p = mat4x4.orthographic(-ratio, ratio, -1.0, 1.0, 1.0, -1.0);
			auto mvp = p * m;
			mvp = mvp.transposed;

			glUseProgram(program);
			glUniformMatrix4fv(mvp_location, 1, GL_FALSE, cast(const(GLfloat*)) &mvp);
			glBindVertexArray(vertex_array);
			glDrawArrays(GL_TRIANGLES, 0, 3);

			glfwSwapBuffers(window);
			glfwPollEvents();
		}

		return true;
	}

	void onKeyEvent(int key, int scancode, int action, int mods)
	{
		if (key == GLFW_KEY_ESCAPE && action == GLFW_PRESS)
			glfwSetWindowShouldClose(window, GLFW_TRUE);
	}
}
