import std.format;

import bindbc.opengl;
import dplug.math;

struct VertexData
{
	vec3f pos;
	vec3f norm;
	vec3f coord;
	vec3f color;
	float waterLevel;
}

class Model
{
	VertexData[] vertices;

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
		glVertexAttribPointer(vposLocation, 3, GL_FLOAT, GL_FALSE, VertexData.sizeof, cast(void*) VertexData.pos.offsetof);
		glEnableVertexAttribArray(vnormLocation);
		glVertexAttribPointer(vnormLocation, 3, GL_FLOAT, GL_FALSE, VertexData.sizeof, cast(void*) VertexData.norm.offsetof);
		glEnableVertexAttribArray(vcoordLocation);
		glVertexAttribPointer(vcoordLocation, 3, GL_FLOAT, GL_FALSE, VertexData.sizeof, cast(void*) VertexData.coord.offsetof);
		glEnableVertexAttribArray(vcolorLocation);
		glVertexAttribPointer(vcolorLocation, 3, GL_FLOAT, GL_FALSE, VertexData.sizeof, cast(void*) VertexData.color.offsetof);
		glEnableVertexAttribArray(vwaterLevelLocation);
		glVertexAttribPointer(vwaterLevelLocation, 1, GL_FLOAT, GL_FALSE, VertexData.sizeof, cast(void*) VertexData.waterLevel.offsetof);

		glBindVertexArray(0);
		glBindBuffer(GL_ARRAY_BUFFER, 0);
	}

	~this()
	{
		glDeleteVertexArrays(1, &vertexArray);
		glDeleteBuffers(1, &vertexBuffer);
	}

	void updateData()
	{
		glBindBuffer(GL_ARRAY_BUFFER, vertexBuffer);
		glBufferData(GL_ARRAY_BUFFER, VertexData.sizeof * vertices.length, cast(void*) vertices.ptr, GL_STATIC_DRAW);
		glBindBuffer(GL_ARRAY_BUFFER, 0);
	}

	void draw()
	{
		glBindVertexArray(vertexArray);
		glDrawArrays(GL_TRIANGLES, 0, cast(int) vertices.length);
		glBindVertexArray(0);
	}

	static void makeSquare(VertexData[] vertices, vec3f a, vec3f b, vec3f c, vec3f d) pure
	{
		auto normal = cross(b - a, c - a).normalized;

		vertices[0] = VertexData(a, normal, coord: vec3f(1,0,0), color: vec3f(-1,-1,-1), waterLevel: float.nan);
		vertices[1] = VertexData(b, normal, coord: vec3f(0,1,0), color: vec3f(-1,-1,-1), waterLevel: float.nan);
		vertices[2] = VertexData(d, normal, coord: vec3f(0,0,1), color: vec3f(-1,-1,-1), waterLevel: float.nan);

		vertices[3] = VertexData(b, normal, coord: vec3f(1,0,0), color: vec3f(-1,-1,-1), waterLevel: float.nan);
		vertices[4] = VertexData(c, normal, coord: vec3f(0,1,0), color: vec3f(-1,-1,-1), waterLevel: float.nan);
		vertices[5] = VertexData(d, normal, coord: vec3f(0,0,1), color: vec3f(-1,-1,-1), waterLevel: float.nan);
	}

	static Model makeCubeMesh(GLuint program)
	{
		auto cube = new Model(program);
		cube.vertices.length = 6 * 5;

		auto d = vec3f(-0.5, -0.5, 0.0);
		auto c = vec3f(-0.5, +0.5, 0.0);
		auto b = vec3f(+0.5, +0.5, 0.0);
		auto a = vec3f(+0.5, -0.5, 0.0);

		Model.makeSquare(cube.vertices[ 0 ..  6], a, b, c, d);
		Model.makeSquare(cube.vertices[ 6 .. 12], b, a, vec3f(a.xy, -10.0), vec3f(b.xy, -10.0));
		Model.makeSquare(cube.vertices[12 .. 18], c, b, vec3f(b.xy, -10.0), vec3f(c.xy, -10.0));
		Model.makeSquare(cube.vertices[18 .. 24], d, c, vec3f(c.xy, -10.0), vec3f(d.xy, -10.0));
		Model.makeSquare(cube.vertices[24 .. 30], a, d, vec3f(d.xy, -10.0), vec3f(a.xy, -10.0));

		cube.updateData();
		return cube;
	}
}

private void checkShader(GLint shader)
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

private void checkProgram(GLint program)
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

GLuint setupShaders(const(char*) vertexShader, const(char*) fragmentShader)
{
	GLuint vShaderId = glCreateShader(GL_VERTEX_SHADER);
	glShaderSource(vShaderId, 1, &vertexShader, null);
	glCompileShader(vShaderId);
	checkShader(vShaderId);

	GLuint fShaderId = glCreateShader(GL_FRAGMENT_SHADER);
	glShaderSource(fShaderId, 1, &fragmentShader, null);
	glCompileShader(fShaderId);
	checkShader(fShaderId);

	auto program = glCreateProgram();
	glAttachShader(program, vShaderId);
	glAttachShader(program, fShaderId);
	glLinkProgram(program);
	checkProgram(program);

	return program;
}
