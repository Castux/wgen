-- As described http://www-cs-students.stanford.edu/~amitp/game-programming/polygon-map-generation/#graphs

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

	return x, y
end

local function angle(a, b)
	return math.atan2(b.y - a.y, b.x - a.x)
end

local function keys(t)
	local tmp = {}
	for k,_ in pairs(t) do
		table.insert(tmp, k)
	end
	return tmp
end

local function addIterator(t, keys)
	t.iter = function(key)
		local table = t[keys[key]]
		if not table then
			error("Bad key for graph.iter(): " .. key)
		end

		return coroutine.wrap(function()
			for i,v in ipairs(table) do
				coroutine.yield(v,i)
			end
		end)
	end
end

local function fromDelaunayHalfEdges(hedges)

	local centers = {}
	local edges = {}
	local corners = {}

	local centerKeys = {
		n = "neighbours",
		e = "edges",
		c = "corners"
	}

	local pointsToCenters = {}
	local function newCenterFromPoint(p)

		local prev = pointsToCenters[p]
		if prev then return prev end

		local center =
		{
			x = p.x,
			y = p.y,
			neighbours = {},
			edges = {},
			corners = {}
		}

		addIterator(center, centerKeys)

		pointsToCenters[p] = center
		table.insert(centers, center)
		return center
	end

	local cornerKeys = {
		n = "neighbours",
		e = "edges",
		c = "centers"
	}

	local hedgesToCorners = {}
	local function newCornerFromHalfEdge(h)

		local prev = hedgesToCorners[h]
		if prev then return prev end

		local h2 = h.next
		local h3 = h2.next

		local cx, cy = circumcenter(
			h.from.x, h.from.y,
			h2.from.x, h2.from.y,
			h3.from.x, h3.from.y
		)

		local corner =
		{
			x = cx,
			y = cy,
			neighbours = {},
			edges = {},
			centers = {}
		}

		addIterator(corner, cornerKeys)

		hedgesToCorners[h] = corner
		hedgesToCorners[h2] = corner
		hedgesToCorners[h3] = corner

		table.insert(corners, corner)
		return corner
	end

	-- Create edges and centers

	local halfEdgeToEdge = {}
	for _,h in ipairs(hedges) do
		local prev = halfEdgeToEdge[h]
		if prev then goto continue end

		local edge =
		{
			center1 = newCenterFromPoint(h.from),
			center2 = newCenterFromPoint(h.to),
			corner1 = newCornerFromHalfEdge(h)
		}

		if h.rev then
			edge.corner2 = newCornerFromHalfEdge(h.rev)
		end

		table.insert(edge.center1.edges, edge)
		table.insert(edge.center2.edges, edge)
		table.insert(edge.center1.neighbours, edge.center2)
		table.insert(edge.center2.neighbours, edge.center1)

		table.insert(edge.corner1.edges, edge)

		if edge.corner2 then
			table.insert(edge.corner2.edges, edge)
			table.insert(edge.corner1.neighbours, edge.corner2)
			table.insert(edge.corner2.neighbours, edge.corner1)
		end

		halfEdgeToEdge[h] = edge
		if h.rev then
			halfEdgeToEdge[h.rev] = edge
		end

		table.insert(edges, edge)

		::continue::
	end

	-- Finally, fill in corner-center relationships

	for _,edge in ipairs(edges) do

		edge.center1.corners[edge.corner1] = true
		edge.center2.corners[edge.corner1] = true
		edge.corner1.centers[edge.center1] = true
		edge.corner1.centers[edge.center2] = true

		if edge.corner2 then
			edge.center1.corners[edge.corner2] = true
			edge.center2.corners[edge.corner2] = true
			edge.corner2.centers[edge.center1] = true
			edge.corner2.centers[edge.center2] = true
		end
	end

	-- And flatten

	for _,center in ipairs(centers) do
		center.corners = keys(center.corners)
		table.sort(center.corners, function(a,b)
			return angle(center, a) < angle(center, b)
		end)
	end

	for _,corner in ipairs(corners) do
		corner.centers = keys(corner.centers)
		table.sort(corner.centers, function(a,b)
			return angle(corner, a) < angle(corner, b)
		end)
	end

	local graph =
	{
		centers = centers,
		edges = edges,
		corners = corners
	}

	addIterator(graph, {
		co = "corners",
		ce = "centers",
		e = "edges"
	})

	return graph
end

return
{
	fromDelaunayHalfEdges = fromDelaunayHalfEdges
}
