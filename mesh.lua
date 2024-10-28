
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

	return t
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

	local border = {a,b,c}

	-- Recompute distances from a point inside this triangle
	cx = (a.x + b.x + c.x) / 3
	cy = (a.y + b.y + c.y) / 3

	for i = 1,3 do table.remove(points, 1) end

	for i,p in ipairs(points) do
		p.dist = dist(cx, cy, p.x, p.y)
	end

	table.sort(points, function(a,b) return a.dist < b.dist end)

	-- Add points one at a time

	for ind,p in ipairs(points) do
		--if ind == 4 then break end
		-- Find any border we're on the right side of
		local b1,b2,borderIndex
		for i = 1, #border do
			b1 = border[i]
			b2 = border[i % #border + 1]

			if clockwise(p, b2, b1) then
				borderIndex = i
				break
			end
		end

		if borderIndex == nil then
			print "Could not find closest border edge"
			break
		end

		-- Create new triangle
		-- Its first edge is the reverse of the border edge we found
		local new = new_tri(p, b2, b1)

		-- Insert new point on border
		table.insert(border, borderIndex + 1, p)
		table.insert(triangles, new)
	end

	return triangles
end

return
{
	new_tri = new_tri,
	delaunay = delaunay
}
