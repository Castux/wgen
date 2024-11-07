import std.math;
import std.typecons;
import std.algorithm;

import dplug.math;

alias Point = vec2d;

const EPSILON = pow(2, -52);

double cross(Point a, Point b)
{
	return dplug.math.cross(vec3d(a, 0.0), dplug.math.vec3d(b, 0.0)).z;
}

bool clockwise(Point a, Point b, Point c)
{
	return cross(b - a, c - a) > 0;
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

double[3] barycentricCoordinates(Point a, Point b, Point c, Point p)
{
	auto x = cross(b - p, c - p);
	auto y = cross(c - p, a - p);
	auto z = cross(a - p, b - p);
	auto s = x + y + z;
	return [x / s, y / s, z / s];
}

bool inTriangle(Point a, Point b, Point c, Point p)
{
	auto coords = barycentricCoordinates(a, b, c, p);
	return coords[0] > 0 && coords[1] > 0 && coords[2] > 0;
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

double pseudoAngle(const(Point) p) pure
{
	auto a = p.x / (abs(p.x) + abs(p.y));
	return (p.y > 0 ? 3 - a : 1 + a) / 4;
}

class Edge
{
	Point from;
	Edge rev;		// The edge in the opposite direction, if any

	Edge next;		// The next edge in this triangle
	Edge hullNext;	// Next edge in the hull, if this is in the hull
	Edge hullPrev;	// Previous edge in the hull, if this is in the hull

	this(Point from)
	{
		this.from = from;
	}

	Point to() const
	{
		return next.from;
	}

	void link(Edge other)
	{
		rev = other;
		other.rev = this;
	}

	bool onHull()
	{
		auto check1 = rev is null;
		auto check2 = hullPrev !is null;
		auto check3 = hullNext !is null;

		assert(check1 == check2 && check1 == check3);
		return check1;
	}

	Triangle triangle()
	{
		auto e1 = this;
		auto e2 = e1.next;
		auto e3 = e2.next;

		return Triangle(e1, e2, e3);
	}

	Edge[] orbit()
	{
		Edge[] orbit;

		auto current = this;
		do
		{
			orbit ~= current;

			current = current.onHull ?
				current.hullPrev.next :	// Hull case
				current.rev.next;		// Normal case

		} while (current !is this);

		return orbit;
	}

	struct Pair
	{
		Edge left, right;
	}
}

struct Triangle
{
	Edge e1, e2, e3;
}

private Triangle newTriangle(Point p1, Point p2, Point p3)
{
	assert (clockwise(p1, p2, p3));

	auto e1 = new Edge(p1);
	auto e2 = new Edge(p2);
	auto e3 = new Edge(p3);

	e1.next = e2;
	e2.next = e3;
	e3.next = e1;

	return Triangle(e1, e2, e3);
}

private void setInitialHull(Triangle t)
{
	t.e1.hullNext = t.e2;
	t.e2.hullNext = t.e3;
	t.e3.hullNext = t.e1;

	t.e1.hullPrev = t.e3;
	t.e2.hullPrev = t.e1;
	t.e3.hullPrev = t.e2;
}

private Edge.Pair hullRemove(Edge e)
{
	auto next = e.hullNext;
	auto prev = e.hullPrev;

	prev.hullNext = next;
	next.hullPrev = prev;

	e.hullNext = null;
	e.hullPrev = null;

	return Edge.Pair(prev, next);
}

private void hullInsert(Edge e, Edge left, Edge right)
{
	e.hullNext = right;
	e.hullPrev = left;

	left.hullNext = e;
	right.hullPrev = e;
}


//          p1                    pl
//        /||\                  /  \
//     o4/ || \o1            o4/    \o1
//      /  ||  \              /  i1  \
//     / i1||i2 \    flip    /________\
//   p4\   ||   /p2   =>   p4\--------/p2
//      \  ||  /              \  i2  /
//     o3\ || /o2            o3\    /o2
//        \||/                  \  /
//         p3                    p3

private void checkDelaunayCondition(Edge e)
{
	if (e.onHull) return;

	auto i1 = e;
	auto i2 = e.rev;
	auto o1 = i2.next;
	auto o2 = o1.next;
	auto o3 = i1.next;
	auto o4 = o3.next;

	auto p1 = o1.from;
	auto p2 = o2.from;
	auto p3 = o3.from;
	auto p4 = o4.from;

	if (inCircle(p1, p2, p3, p4))
	{
		i1.from = p2;
		i2.from = p4;

		o1.next = i1;
		i1.next = o4;
		o4.next = o1;

		o2.next = o3;
		o3.next = i2;
		i2.next = o2;

		checkDelaunayCondition(o1);
		checkDelaunayCondition(o2);
	}
}

struct Triangulation
{
	Edge[] edges;

	private const(Point) center;
	private Edge[] hash;
	private const(int) hashSize;

	int hashKey(const(Point) p) const pure
	{
		return cast(int) floor(pseudoAngle(p - center) * (hashSize - 1)) % hashSize;
	}

	void hashAdd(Edge e)
	{
		hash[hashKey(e.from)] = e;
	}

	this(Point[] points)
	{
		if (points.length < 3)
			throw new Exception("Cannot triangulate fewer than 3 points");

		// Build the first triangle somewhere close to the center of the points

		Point c = points.fold!((a,b) => a + b) / points.length;

		// The two closest ones to the center
		Point p1 = points.minElement!(a => a.squaredDistanceTo(c));
		Point p2 = points.filter!(a => a != p1).minElement!(a => a.squaredDistanceTo(c));

		// And the one other that forms the smallest circumcircle with them
		Point p3 = points.filter!(a => a != p1 && a != p2)
			.minElement!(a => circumradius(p1, p2, a));

		if (!clockwise(p1, p2, p3))
			swap(p2, p3);

		if (!clockwise(p1, p2, p3))
			throw new Exception("Cannot triangulate this input");

		Triangle centerTri = newTriangle(p1, p2, p3);
		edges = [centerTri.e1, centerTri.e2, centerTri.e3];

		// Initialize the hull to be these three edges

		hashSize = cast(int) ceil(sqrt(cast(double) points.length));
		hash = new Edge[hashSize];

		setInitialHull(centerTri);
		hashAdd(centerTri.e1);
		hashAdd(centerTri.e2);
		hashAdd(centerTri.e3);

		// Sort the points by distance to the center triangle's circumcenter

		center = circumcenter(p1, p2, p3);
		points.sort!((a,b) => a.squaredDistanceTo(center) < b.squaredDistanceTo(center));

		// Add points one by one from the center out

		foreach(i,p; points)
		{
			// Skip the points we added already
			if (p == p1 || p == p2 || p == p3)
				continue;

			// Skip near identical to previous point
			if (i > 0 && abs(p.x - points[i-1].x) <= EPSILON && abs(p.y - points[i-1].y) <= EPSILON)
				continue;

			processPoint(p);
		}
	}

	private void processPoint(Point p)
	{
		// Find any hull edge we're on the correct side of
		// Use the hash for a good starting guess

		Edge startEdge;
		foreach(i; 0 .. hashSize)
		{
			auto e = hash[(hashKey(p) + i) % hashSize];
			if (e && e.onHull)
			{
				startEdge = e;
				break;
			}
		}

		assert(startEdge && startEdge.onHull);
		startEdge = startEdge.hullPrev;

		Edge current = startEdge;

		while (!clockwise(p, current.to, current.from))
		{
			current = current.hullNext;
			if (current is startEdge)
			{
				// Likely a near-duplicate point; skip it
				return;
			}
		}

		// Create new triangle on that edge
		with (newTriOnEdge(p, current))
		{
			assert(right && left);
			fixHull!"right"(right);
			fixHull!"left"(left);
		}
	}

	private Edge.Pair newTriOnEdge(Point p, Edge edge)
	{
		with (newTriangle(edge.to, edge.from, p))
		{
			// e1 is against the existing edge, e2 and e3 are the new ones

			e1.link(edge);

			edges ~= e1;
			edges ~= e2;
			edges ~= e3;

			with (hullRemove(edge))
			{
				hullInsert(e2, left, right);
				hullInsert(e3, e2, right);
			}

			hashAdd(e2);
			hashAdd(e3);

			checkDelaunayCondition(e1);

			return Edge.Pair(e2, e3);
		}
	}

	// Make a new triangle from two border edges that form a concavity

	private Edge newTriOnTwoEdges(Edge left, Edge right)
	{
		auto p1 = right.to;
		auto p2 = left.to;
		auto p3 = left.from;

		if (!clockwise(p1, p2, p3))
			return null;

		with (newTriangle(p1, p2, p3))
		{
			// e1 and e2 rest against right and left, e3 is the new one

			e1.link(right);
			e2.link(left);

			edges ~= e1;
			edges ~= e2;
			edges ~= e3;

			// Replace two border edges with the new one

			hullRemove(left);
			auto pair = hullRemove(right);
			hullInsert(e3, pair.left, pair.right);

			hashAdd(e3);

			checkDelaunayCondition(e1);
			checkDelaunayCondition(e2);

			return e3;
		}
	}

	private void fixHull(string direction)(Edge edge)
	{
		assert(edge.onHull);

		Edge current = edge;

		while (current)
		{
			static if (direction == "left")
				current = current.hullPrev;

			if (clockwise(current.from, current.to, current.hullNext.to))
				break;

			current = newTriOnTwoEdges(current, current.hullNext);
		}
	}
}
