import std.math;
import std.typecons;
import std.algorithm;

public import geom;

private bool clockwise(Point a, Point b, Point c)
{
	return (b - a).cross(c - a) > 0;
//	return (b.x - a.x) * (c.y - a.y) - (c.x - a.x) * (b.y - a.y) > 0;
}

private bool inCircle(Point a, Point b, Point c, Point p)
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

private double pseudoAngle(const(Point) p) pure
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
	Edge hullPrev;	// Next edge in the hull, if this is in the hull

	this(Point from)
	{
		this.from = from;
	}

	Point to()
	{
		return next.from;
	}

	void link(Edge other)
	{
		rev = other;
		other.rev = this;
	}

	struct Pair
	{
		Edge left, right;
	}
}

private struct Triangle
{
	Edge e1, e2, e3;

	this(Point p1, Point p2, Point p3)
	{
		if (!clockwise(p1, p2, p3))
			throw new Exception("Non clockwise triangle");

		e1 = new Edge(p1);
		e2 = new Edge(p2);
		e3 = new Edge(p3);

		e1.next = e2;
		e2.next = e3;
		e3.next = e1;
	}
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
	if (left.hullNext !is right || right.hullPrev !is left)
		throw new Exception("Bad hull insert");

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
	if (!e.rev) return;

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
		checkDelaunayCondition(o3);
		checkDelaunayCondition(o4);
	}
}

struct Triangulation
{
	Edge[] edges;
	Point[] ignored;

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
		Point p1 = points.minElement!(a => a.sqdist(c));
		Point p2 = points.minElement!(a => (a == p1) ? double.infinity : a.sqdist(c));
		Point p3 = points.minElement!(a => (a == p1 || a == p2) ? double.infinity : a.sqdist(c));

		if (!clockwise(p1, p2, p3))
			swap(p2, p3);

		Triangle centerTri = Triangle(p1, p2, p3);

		edges = [centerTri.e1, centerTri.e2, centerTri.e3];

		// Initialize the hull to be these three edges

		hashSize = cast(int) ceil(sqrt(cast(double) points.length));
		hash = new Edge[hashSize];

		setInitialHull(centerTri);
		hashAdd(centerTri.e1);
		hashAdd(centerTri.e2);
		hashAdd(centerTri.e3);

		// Sort the points by distance to the center triangle

		center = (p1 + p2 + p3) / 3;
		points.sort!((a,b) => a.sqdist(center) < b.sqdist(center));

		// Add points one by one from the center out

		foreach(i,p; points)
		{
			if (p != p1 && p != p2 && p != p3)
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
			if (e !is null && e.rev is null)
			{
				startEdge = e;
				break;
			}
		}

		if (!startEdge)
		{
			ignored ~= p;
			return;
		}

		assert(startEdge.hullNext && startEdge.hullPrev);
		startEdge = startEdge.hullPrev;

		Edge found;
		Edge current = startEdge;
		do
		{
			if (clockwise(p, current.to, current.from))
			{
				found = current;
				break;
			}
			current = current.hullNext;
		} while (current !is startEdge);

		if (!found)
		{
			ignored ~= p;
			return;
		}

		// Create new triangle on that edge
		with (newTriOnEdge(p, found))
		{
			assert(right && left);

			fixHull!"right"(right);
			fixHull!"left"(left);
		}
	}

	private Edge.Pair newTriOnEdge(Point p, Edge edge)
	{
		// e1 is against the existing edge, e2 and e3 are the new ones
		with (Triangle(edge.to, edge.from, p))
		{
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
		// Occasionally, the hull has collinear points, which don't
		// Register as "clockwise", but are already correctly convex
		Triangle tri;
		try
			tri = Triangle(right.to, left.to, left.from);
		catch(Exception e)
			return null;

		// e1 and e2 rest against right and left, e3 is the new one
		with (tri)
		{
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
