import std.math;

struct Point
{
	double x;
	double y;

	Point opBinary(string op)(Point other) const
	{
		static if(op == "+")
			return Point(x + other.x, y + other.y);
		else static if(op == "-")
			return Point(x - other.x, y - other.y);
		else
			static assert(0, "Operator "~op~" not implemented");
	}

	Point opBinary(string op)(double other) const
	{
		static if(op == "*")
			return Point(x*other, y*other);
		else static if(op == "/")
			return Point(x/other, y/other);
		else
			static assert(0, "Operator "~op~" not implemented");

	}

	double dot(const(Point) other) const
	{
		return x*other.x + y*other.y;
	}

	double cross(const(Point) other) const
	{
		return x*other.y - y*other.x;
	}

	double sqlen() const
	{
		return this.dot(this);
	}

	double len() const
	{
		return sqrt(sqlen());
	}

	double dir() const
	{
		return atan2(y,x);
	}

	Point unit() const
	{
		return this/len;
	}

	Point rotate(double angle) const
	{
		return Point(cos(angle)*x - sin(angle)*y, cos(angle)*y + sin(angle)*x);
	}

	double dist(const(Point) other) const
	{
		return (other-this).len;
	}

	double sqdist(const(Point) other) const
	{
		return (other-this).sqlen;
	}

	Point normal() const
	{
		return Point(-y,x).unit;
	}

}
//
// alias Point Vector;
//
// // A general class to hold counterclockwise convex polygons
//
// struct Convex
// {
// 	Point[] verts;
//
// 	this(Point[] v)
// 	{
// 		verts = v;
// 	}
//
// 	// Check convexity and counterclockwiseness
//
// 	bool valid()
// 	{
// 		if(verts.length < 3)
// 			return false;
//
// 		if(verts.length == 3)
// 			return (verts[1] - verts[0]).cross(verts[2]-verts[0]) >= 0;
//
// 		foreach(i; 0 .. verts.length)
// 			if ((verts[(i+1) % verts.length] - verts[i]).cross(verts[(i+2) % verts.length] - verts[i]) < 0)
// 				return false;
//
// 		return true;
// 	}
//
// 	bool contains(Point p)
// 	{
// 		foreach(i; 0 .. verts.length)
// 			if ((verts[(i+1) % verts.length] - verts[i]).cross(p - verts[i]) < 0)
// 				return false;
//
// 		return true;
// 	}
//
// 	double area()
// 	{
// 		double sum = 0;
//
// 		foreach(i; 0 .. verts.length)
// 			sum += verts[i].cross(verts[(i+1) % verts.length]);
//
// 		return sum / 2.0;
// 	}
//
// 	Convex[] tesselate(Point p)
// 	{
// 		assert(valid);
//
// 		Convex[] tris;
//
// 		if(!contains(p))
// 			return tris;
//
// 		foreach(i; 0 .. verts.length)
// 		{
// 			auto tri = Convex([p, verts[i], verts[(i+1) % verts.length]]);
// 			tris ~= tri;
// 		}
//
// 		return tris;
// 	}
//
// 	Circle circumcircle()
// 	{
// 		if(verts.length != 3)
// 			return Circle();	// Could check later if the polygon is circular. Not very hard.
//
// 		auto l1 = Segment(verts[0],verts[1]).bisector;
// 		auto l2 = Segment(verts[0],verts[2]).bisector;
//
// 		auto c = l1.intersection(l2);
// 		return Circle(c, c.dist(verts[0]));
// 	}
// }
//
// struct Line
// {
// 	// Line defined as all x that satisfy n.x = c
// 	// In practice n can (should be?) a unit vector
//
// 	Vector	n;
// 	double		c;
//
// 	Point intersection(Line o)
// 	{
// 		// Pretty simple, we solve
// 		// n1.I = c1
// 		// n2.I = c2
// 		//
// 		// That's
// 		// [n1,n2]^T * I = b, or
// 		// [m1,m2] * I = b, with M = N^T
// 		// with b = [c1,c2]^T
// 		//
// 		// Crammer's rule gives:
// 		// I = [ |b,m2|, |m1,b| ]^T / |m1,m2|
//
// 		auto n1 = n;
// 		auto n2 = o.n;
// 		auto b = Vector(c, o.c);
//
// 		auto m1 = Vector(n1.x, n2.x);
// 		auto m2 = Vector(n1.y, n2.y);
//
// 		double det = m1.cross(m2);
// 		if(det == 0)
// 			return Point();
//
// 		return Point(b.cross(m2), m1.cross(b)) / det;
// 	}
//
// 	static Line fromPointAndDir(Point orig, Vector dir)
// 	{
// 		auto ndir = dir.normal;
// 		auto c = orig.dot(ndir);
//
// 		return Line(ndir,c);
// 	}
// }
//
// struct Segment
// {
// 	Point a,b;
//
// 	double len()
// 	{
// 		return a.dist(b);
// 	}
//
// 	Point middle()
// 	{
// 		return (a+b)/2;
// 	}
//
// 	Vector dir()
// 	{
// 		return (b-a).unit;
// 	}
//
// 	Line bisector()
// 	{
// 		return Line.fromPointAndDir(middle, dir.normal);
// 	}
// }
//
// struct Circle
// {
// 	Point	c;
// 	double		r;
//
// 	double area()
// 	{
// 		return 2*PI*r^^2;
// 	}
//
// 	bool contains(Point p)
// 	{
// 		return c.sqdist(p) <= r^^2;
// 	}
// }
