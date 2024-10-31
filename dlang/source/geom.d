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

	int opCmp(ref const Point p) const
	{
		if (x < p.x)
			return -1;

		if (y < p.y)
			return -1;

		if (y == p.y)
			return 0;

		return 1;
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

bool clockwise(Point a, Point b, Point c)
{
	return (b - a).cross(c - a) > 0;
}

bool inCircle(Point a, Point b, Point c, Point p)
{
	auto dx = a.x - p.x;
	auto dy = a.y - p.y;
	auto ex = b.x - p.x;
	auto ey = b.y - p.y;
	auto fx = c.x - p.x;
	auto fy = c.y - p.y;

	auto ap = dx * dx + dy * dy;
	auto bp = ex * ex + ey * ey;
	auto cp = fx * fx + fy * fy;

	return dx * (ey * cp - bp * fy) -
		   dy * (ex * cp - bp * fx) +
		   ap * (ex * fy - ey * fx) > 0;
}

Point circumcenter(Point a, Point b, Point c)
{
	auto dx = b.x - a.x;
	auto dy = b.y - a.y;
	auto ex = c.x - a.x;
	auto ey = c.y - a.y;

	auto bl = dx * dx + dy * dy;
	auto cl = ex * ex + ey * ey;
	auto d = 0.5 / (dx * ey - dy * ex);

	auto x = a.x + (ey * bl - dy * cl) * d;
	auto y = a.y + (dx * cl - ex * bl) * d;

	return Point(x, y);
}

double circumradius(Point a, Point b, Point c)
{
	auto dx = b.x - a.x;
	auto dy = b.y - a.y;
	auto ex = c.x - a.x;
	auto ey = c.y - a.y;

	auto bl = dx * dx + dy * dy;
	auto cl = ex * ex + ey * ey;
	auto d = 0.5 / (dx * ey - dy * ex);

	auto x = (ey * bl - dy * cl) * d;
	auto y = (dx * cl - ex * bl) * d;

	return x * x + y * y;
}
