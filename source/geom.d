import std.math;

alias double num;

struct Point
{
	num x;
	num y;

	this(num a, num b)
	{
		x = a;
		y = b;
	}

	Point opBinary(string op)(Point other)
	{
		static if(op == "+")
			return Point(x + other.x, y + other.y);
		else static if(op == "-")
			return Point(x - other.x, y - other.y);
		else
			static assert(0, "Operator "~op~" not implemented");
	}

	Point opBinary(string op)(num other)
	{
		static if(op == "*")
			return Point(x*other, y*other);
		else static if(op == "/")
			return Point(x/other, y/other);
		else
			static assert(0, "Operator "~op~" not implemented");

	}

	num dot(Point other)
	{
		return x*other.x + y*other.y;
	}

	num cross(Point other)
	{
		return x*other.y - y*other.x;
	}

	num sqlen()
	{
		return this.dot(this);
	}

	num len()
	{
		return sqrt(sqlen());
	}

	num dir()
	{
		return atan2(y,x);
	}

	Point unit()
	{
		return this/len;
	}

	Point rotate(num angle)
	{
		return Point(cos(angle)*x - sin(angle)*y, cos(angle)*y + sin(angle)*x);
	}

	num dist(Point other)
	{
		return (other-this).len;
	}

	num sqdist(Point other)
	{
		return (other-this).sqlen;
	}

	Vector normal()
	{
		return Point(-y,x).unit;
	}

	static num angle(Point a, Point b, Point c)
	{
		auto l = a - b;
		auto r = c - b;
		return acos(l.dot(r) / l.len / r.len);
	}
}

alias Point Vector;

// A general class to hold counterclockwise convex polygons

struct Convex
{
	Point[] verts;

	this(Point[] v)
	{
		verts = v;
	}

	// Check convexity and counterclockwiseness

	bool valid()
	{
		if(verts.length < 3)
			return false;

		if(verts.length == 3)
			return (verts[1] - verts[0]).cross(verts[2]-verts[0]) >= 0;

		foreach(i; 0 .. verts.length)
			if ((verts[(i+1) % verts.length] - verts[i]).cross(verts[(i+2) % verts.length] - verts[i]) < 0)
				return false;

		return true;
	}

	bool contains(Point p)
	{
		foreach(i; 0 .. verts.length)
			if ((verts[(i+1) % verts.length] - verts[i]).cross(p - verts[i]) < 0)
				return false;

		return true;
	}

	num area()
	{
		num sum = 0;

		foreach(i; 0 .. verts.length)
			sum += verts[i].cross(verts[(i+1) % verts.length]);

		return sum / 2.0;
	}

	Convex[] tesselate(Point p)
	{
		assert(valid);

		Convex[] tris;

		if(!contains(p))
			return tris;

		foreach(i; 0 .. verts.length)
		{
			auto tri = Convex([p, verts[i], verts[(i+1) % verts.length]]);
			tris ~= tri;
		}

		return tris;
	}

	Circle circumcircle()
	{
		if(verts.length != 3)
			return Circle();	// Could check later if the polygon is circular. Not very hard.

		auto l1 = Segment(verts[0],verts[1]).bisector;
		auto l2 = Segment(verts[0],verts[2]).bisector;

		auto c = l1.intersection(l2);
		return Circle(c, c.dist(verts[0]));
	}

	bool insideCircumcirle(Point p)
	{
		if(verts.length != 3)
			return false;

		foreach(i; 0 .. verts.length)
		{
			auto s1 = verts[i];
			auto s2 = verts[(i+1) % verts.length];
			auto opposite = verts[(i+2) % verts.length];

			if ((p - s1).cross(s2 - s1) < 0)
			{
				auto angle = Point.angle(s1, p, s2);
				auto angle2 = Point.angle(s2, opposite, s1);

				return angle + angle2 < PI;
			}
		}

		assert(false);
	}
}

struct Line
{
	// Line defined as all x that satisfy n.x = c
	// In practice n can (should be?) a unit vector

	Vector	n;
	num		c;

	Point intersection(Line o)
	{
		// Pretty simple, we solve
		// n1.I = c1
		// n2.I = c2
		//
		// That's
		// [n1,n2]^T * I = b, or
		// [m1,m2] * I = b, with M = N^T
		// with b = [c1,c2]^T
		//
		// Crammer's rule gives:
		// I = [ |b,m2|, |m1,b| ]^T / |m1,m2|

		auto n1 = n;
		auto n2 = o.n;
		auto b = Vector(c, o.c);

		auto m1 = Vector(n1.x, n2.x);
		auto m2 = Vector(n1.y, n2.y);

		num det = m1.cross(m2);
		if(det == 0)
			return Point();

		return Point(b.cross(m2), m1.cross(b)) / det;
	}

	static Line fromPointAndDir(Point orig, Vector dir)
	{
		auto ndir = dir.normal;
		auto c = orig.dot(ndir);

		return Line(ndir,c);
	}
}

struct Segment
{
	Point a,b;

	num len()
	{
		return a.dist(b);
	}

	Point middle()
	{
		return (a+b)/2;
	}

	Vector dir()
	{
		return (b-a).unit;
	}

	Line bisector()
	{
		return Line.fromPointAndDir(middle, dir.normal);
	}
}

struct Circle
{
	Point	c;
	num		r;

	num area()
	{
		return 2*PI*r^^2;
	}

	bool contains(Point p)
	{
		return c.sqdist(p) <= r^^2;
	}
}
