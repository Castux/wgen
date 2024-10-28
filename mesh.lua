
local function dist(ax, ay, bx, by)
	local dx = ax - bx
	local dy = ay - by
	return dx * dx + dy * dy
end

local function cross(ax, ay, bx, by)
	return ax * by - ay * bx
end

local function clockwise(a, b, c)
	return (b.x - a.x) * (c.y - a.y) - (c.x - a.x) * (b.y - a.y) > 0
end

local function inCircle(ax, ay, bx, by, cx, cy, px, py)
	local dx = ax - px
	local dy = ay - py
	local ex = bx - px
	local ey = by - py
	local fx = cx - px
	local fy = cy - py

	local ap = dx * dx + dy * dy
	local bp = ex * ex + ey * ey
	local cp = fx * fx + fy * fy

	return dx * (ey * cp - bp * fy) -
		   dy * (ex * cp - bp * fx) +
		   ap * (ex * fy - ey * fx) < 0
end

local function circumradius(ax, ay, bx, by, cx, cy)
	local dx = bx - ax
	local dy = by - ay
	local ex = cx - ax
	local ey = cy - ay

	local bl = dx * dx + dy * dy
	local cl = ex * ex + ey * ey
	local d = 0.5 / (dx * ey - dy * ex)

	local x = (ey * bl - dy * cl) * d
	local y = (dx * cl - ex * bl) * d

	return x * x + y * y
end

local function circumcenter(ax, ay, bx, by, cx, cy)
	local dx = bx - ax
	local dy = by - ay
	local ex = cx - ax
	local ey = cy - ay

	local bl = dx * dx + dy * dy
	local cl = ex * ex + ey * ey
	local d = 0.5 / (dx * ey - dy * ex)

	local x = ax + (ey * bl - dy * cl) * d
	local y = ay + (dx * cl - ex * bl) * d

	return {x, y}
end

local function newTri(p1,p2,p3)
	local t = {}

	if not clockwise(p1,p2,p3) then
		error("Non clockwise triangle")
	end

	t[1] = p1
	t[2] = p2
	t[3] = p3

	t.reverse = {}

	return t
end

-- Edges are references to a triangle and the index of the starting vertex

local function mod3(i)
	return (i - 1) % 3 + 1
end

local function edge(tri,index)
	return {tri = tri, index = index}
end

local function connect(t1, e1, t2, e2)
	t1.reverse[e1] = edge(t2,e2)
	t2.reverse[e2] = edge(t1,e1)
end

local function edgeStart(e)
	return e.tri[e.index]
end

local function edgeEnd(e)
	return e.tri[mod3(e.index + 1)]
end

local function newBorder(first)
	first.next = first
	first.prev = first

	return first
end

local function borderInsert(node, new)
	new.next = node.next
	new.next.prev = new

	node.next = new
	new.prev = node

	return new
end

local function borderRemove(node)
	local next = node.next
	local prev = node.prev

	prev.next = next
	next.prev = prev

	return prev,next
end


--          p1                    pl
--        /||\                  /  \
--     e4/ || \e1            e4/    \e1
--      /  ||  \              /  t1  \
--     / t1||t2 \    flip    /________\
--   p4\   ||   /p2   =>   p4\--------/p2
--      \  ||  /              \  t2  /
--     e3\ || /e2            e3\    /e2
--        \||/                  \  /
--         p3                    p3

local function flipEdge(t1, i1)
	if not t1.reverse[i1] then return end
	local t2 = t1.reverse[i1].tri
	local i2 = t1.reverse[i1].index

	assert(t1[i1] == t2[mod3(i2 + 1)])
	assert(t1[mod3(i1 + 1)] == t2[i2])

	local p1 = t1[i1]
	local p3 = t1[mod3(i1 + 1)]
	local p4 = t1[mod3(i1 + 2)]

	local p2 = t2[mod3(i2 + 2)]

end

-- Make a new triangle from a point and a border edge
local function pointWithEdge(p, e, triangles)

	-- Create triangle
	local new = newTri(edgeStart(e), p, edgeEnd(e))
	connect(new, 3, e.tri, e.index)

	-- Replace border edge with two new edges
	e = borderRemove(e)
	e = borderInsert(e, edge(new,1))
	e = borderInsert(e, edge(new,2))

	table.insert(triangles, new)

	flipEdge(new, 3)

	return e.prev, e
end

-- Make a new triangle from two border edges that form a concavity
local function edgeWithEdge(e, triangles)

	local a,b,c = edgeStart(e),edgeEnd(e),edgeEnd(e.next)
	local new = newTri(a,c,b)
	connect(new, 2, e.next.tri, e.next.index)
	connect(new, 3, e.tri, e.index)

	-- Replace two border edges with the new one
	borderRemove(e.next)
	e = borderRemove(e)
	e = borderInsert(e, edge(new, 1))

	table.insert(triangles, new)

	return e
end


local function processPoint(p, triangles, border)

	-- Find any border we're on the correct side of
	local first = border
	local found
	repeat
		if clockwise(p, edgeEnd(border), edgeStart(border)) then
			found = true
			break
		end
		border = border.next
	until border == first

	if not found then
		print "Could not find valid border edge"
		return
	end

	-- Create new triangle
	-- Connect it to the edge we inserted in
	local left,right = pointWithEdge(p, border, triangles)
	assert(edgeEnd(left) == p)
	assert(edgeStart(right) == p)
	assert(left ~= right)

	-- Fix hull to keep it convex
	local current = right
	while true do
		assert(edgeEnd(current) == edgeStart(current.next))
		local a,b,c = edgeStart(current),edgeEnd(current),edgeEnd(current.next)
		if clockwise(a,b,c) then break end
		current = edgeWithEdge(current, triangles)
	end

	-- Same in the other direction
	local current = left.prev
	while true do
		assert(edgeEnd(current) == edgeStart(current.next))
		local a,b,c = edgeStart(current),edgeEnd(current),edgeEnd(current.next)
		if clockwise(a,b,c) then break end
		current = edgeWithEdge(current, triangles)
		current = current.prev
	end

	return current
end

local function delaunay(points)

	-- Find the most central triangle to start with

	local minx,maxx,miny,maxy = math.huge,-math.huge,math.huge,-math.huge
	for i,p in ipairs(points) do
		minx = math.min(minx, p.x)
		maxx = math.max(maxx, p.x)
		miny = math.min(miny, p.y)
		maxy = math.max(maxy, p.y)
	end

	local cx,cy = (maxx + minx) / 2, (maxy + miny) / 2

	for i,p in ipairs(points) do
		p.dist = dist(cx, cy, p.x, p.y)
	end

	table.sort(points, function(a,b) return a.dist < b.dist end)

	local a,b,c = points[1], points[2], points[3]
	if not clockwise(a,b,c) then
		a,b,c = c,b,a
	end

	local start = newTri(a,b,c)
	local triangles = {start}

	local border = newBorder(edge(start,1))
	border = borderInsert(border, edge(start,2))
	border = borderInsert(border, edge(start,3))

	assert(border.next.next.next == border)
	assert(border.prev.prev.prev == border)

	-- Recompute distances from a point inside this triangle
	cx = (a.x + b.x + c.x) / 3
	cy = (a.y + b.y + c.y) / 3

	for i = 1,3 do table.remove(points, 1) end

	for i,p in ipairs(points) do
		p.dist = dist(cx, cy, p.x, p.y)
	end

	table.sort(points, function(a,b) return a.dist < b.dist end)

	-- Add points one at a time

	for foo,p in ipairs(points) do
		border = processPoint(p, triangles, border)
	end

	return triangles, border
end

return
{
	newTri = newTri,
	delaunay = delaunay
}
