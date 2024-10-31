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
}

private struct Triangle
{
	Edge e1, e2, e3;

	this(Point p1, Point p2, Point p3, Edge rev1 = null, Edge rev2 = null, Edge rev3 = null)
	{
		if (!clockwise(p1, p2, p3))
			throw new Exception("Non clockwise triangle");

		e1 = new Edge(p1);
		e2 = new Edge(p2);
		e3 = new Edge(p3);

		e1.next = e2;
		e2.next = e3;
		e3.next = e1;

		if (rev1) e1.link(rev1);
		if (rev2) e2.link(rev2);
		if (rev3) e3.link(rev3);
	}

	void setInitialHull()
	{
		e1.hullNext = e2;
		e2.hullNext = e3;
		e3.hullNext = e1;

		e1.hullPrev = e3;
		e2.hullPrev = e1;
		e3.hullPrev = e2;
	}
}

private Tuple!(Edge,Edge) hullRemove(Edge e)
{
	auto next = e.hullNext;
	auto prev = e.hullPrev;

	prev.hullNext = next;
	next.hullPrev = prev;

	return tuple(prev, next);
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

	Edge[] hash;
	int hashSize;

	this(Point[] points)
	{
		Point center = points.fold!((a,b) => a + b) / points.length;
		Point p1 = points.minElement!(a => (a - center).sqlen);
		Point p2 = points.minElement!(a => (a == p1) ? double.infinity : (a - center).sqlen);
		Point p3 = points.minElement!(a => (a == p1 || a == p2) ? double.infinity : (a - center).sqlen);

		if (!clockwise(p1, p2, p3))
			swap(p2, p3);

		Triangle centerTri = Triangle(p1, p2, p3);

		edges = [centerTri.e1, centerTri.e2, centerTri.e3];
	}
}
