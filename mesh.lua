
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

local function new_tri(p1,p2,p3)
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
	return e.tri[e.index % 3 + 1]
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

	local start = new_tri(a,b,c)
	local triangles = {start}

	local border = {
		edge(start, 1),
		edge(start, 2),
		edge(start, 3)
	}

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
		if foo == 4 then break end
		-- Find any border we're on the correct side of
		local found, index
		for i,e in ipairs(border) do
			if clockwise(p, edgeEnd(e), edgeStart(e)) then
				found = e
				index = i
				break
			end
		end

		if index == nil then
			print "Could not find valid border edge"
			break
		end

		-- Create new triangle
		-- Connect it to the edge we inserted in
		local new = new_tri(edgeStart(found), p, edgeEnd(found))
		connect(new, 3, found.tri, found.index)

		-- Replace edge with two new edges
		border[index] = edge(new, 1)
		table.insert(border, index + 1, edge(new, 2))

		-- Save triangle
		table.insert(triangles, new)
	end

	return triangles
end

return
{
	new_tri = new_tri,
	delaunay = delaunay
}
