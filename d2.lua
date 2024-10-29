local function clockwise(a, b, c)
	return (b.x - a.x) * (c.y - a.y) - (c.x - a.x) * (b.y - a.y) > 0
end

local function dist(ax, ay, bx, by)
	local dx = ax - bx
	local dy = ay - by
	return dx * dx + dy * dy
end

local function link(e1, e2)
	e1.rev = e2
	e2.rev = e1
end

local function newTri(p1, p2, p3, rev1, rev2, rev3)

	assert(clockwise(p1, p2, p3), "Non clockwise triangle")

	local e1 = {from = p1, to = p2}
	local e2 = {from = p2, to = p3}
	local e3 = {from = p3, to = p1}

	e1.next = e2
	e2.next = e3
	e3.next = e1

	if rev1 then link(e1, rev1) end
	if rev2 then link(e2, rev2) end
	if rev3 then link(e3, rev3) end

	return e1, e2, e3
end

local function setInitialHull(e1, e2, e3)
	e1.hullNext = e2
	e2.hullNext = e3
	e3.hullNext = e1

	e1.hullPrev = e3
	e2.hullPrev = e1
	e3.hullPrev = e2
end

local function hullRemove(e)
	local next = e.hullNext
	local prev = e.hullPrev

	prev.hullNext = next
	next.hullPrev = prev

	return prev,next
end

local function hullInsert(new, left, right)
	assert(left.hullNext == right and right.hullPrev == left)

	new.hullNext = right
	new.hullPrev = left

	left.hullNext = new
	right.hullPrev = new
end

local function newTriOnEdge(p, edge, edges)

	-- e1 is against the existing edge, e2 and e3 are the new ones
	local e1, e2, e3 = newTri(edge.to, edge.from, p, edge)

	table.insert(edges, e1)
	table.insert(edges, e2)
	table.insert(edges, e3)

	local left,right = hullRemove(edge)
	hullInsert(e2, left, right)
	hullInsert(e3, e2, right)

	edges.hull = e2
	return e2, e3
end

-- Make a new triangle from two border edges that form a concavity
local function newTriOnTwoEdges(left, right, edges)

	-- e1 and e2 rest against right and left, e3 is the new one
	local e1, e2, e3 = newTri(right.to, left.to, left.from, right, left)

	table.insert(edges, e1)
	table.insert(edges, e2)
	table.insert(edges, e3)

	-- Replace two border edges with the new one
	local lleft, lright = hullRemove(left)
	assert(lright == right)
	local lleft, rright = hullRemove(right)
	hullInsert(e3, lleft, rright)

	edges.hull = e3

	return e3
end

local function fixHull(edge, direction, edges)

	local current = edge

	while true do
		if direction == "left" then
			current = current.hullPrev
		end

		assert(current.to == current.hullNext.from)
		local a,b,c = current.from, current.to, current.hullNext.to
		if clockwise(a, b, c) then break end
		current = newTriOnTwoEdges(current, current.hullNext, edges)
	end

end

local function processPoint(p, edges)

	-- Find any hull edge we're on the correct side of
	local current = edges.hull
	local found
	repeat
		if clockwise(p, current.to, current.from) then
			found = current
			break
		end
		current = current.hullNext
	until current == edges.hull

	assert(found, "Could not find valid hull edge")

	-- Create new triangle on that edge
	local left, right = newTriOnEdge(p, found, edges)

	-- Fix hull to keep it convex
	fixHull(right, "right", edges)
	fixHull(left, "left", edges)
end

local function delaunay(points)

	-- Find the most central triangle to start with

	local minx,maxx,miny,maxy = math.huge,-math.huge,math.huge,-math.huge
	for _,p in ipairs(points) do
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

	local e1, e2, e3 = newTri(a, b, c)
	setInitialHull(e1, e2, e3)
	local edges = {e1, e2, e3}
	edges.hull = e1

	assert(e1.next.next.next == e1)
	assert(e1.hullNext.hullNext.hullNext == e1)
	assert(e1.hullPrev.hullPrev.hullPrev == e1)

	-- Recompute distances from a point inside this triangle
	cx = (a.x + b.x + c.x) / 3
	cy = (a.y + b.y + c.y) / 3

	for i = 1,3 do table.remove(points, 1) end

	for i,p in ipairs(points) do
		p.dist = dist(cx, cy, p.x, p.y)
	end

	table.sort(points, function(a,b) return a.dist < b.dist end)

	for _,p in ipairs(points) do
		processPoint(p, edges)
	end

	return edges
end

return
{
	delaunay = delaunay
}
