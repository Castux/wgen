local tga = require "tga"
local image = require "image"
local delaunay = require "delaunay"
local graph = require "graph"

local function relaxGraph(g, w, h)

	local new = {}
	for _,center in ipairs(g.centers) do
		if center.x < 0 or center.x > w or center.y < 0 or center.y > h then
			table.insert(new, {x = center.x, y = center.y})
		else
			local x,y = 0,0
			for _,corner in ipairs(center.corners) do
				x = x + corner.x
				y = y + corner.y
			end

			table.insert(new, {
				x = x / #center.corners,
				y = y / #center.corners
			})
		end
	end

	local edges = delaunay.delaunay(new)
	return graph.fromDelaunayHalfEdges(edges)
end

local function generateGraph(w, h, res)

	local points = {}
	local margin = 30

	for x = -margin, w + margin, res do
		for y = -margin, h + margin, res do
			table.insert(points, {
				x = x + (math.random() -  0.5) * res * 0.5,
				y = y + (math.random() -  0.5) * res * 0.5
			})
		end
	end

	local edges = delaunay.delaunay(points)
	local graph = graph.fromDelaunayHalfEdges(edges)
	graph = relaxGraph(graph, w, h)

	return graph
end

local function lerp(a,b,x)
	return a * (1-x) + b * x
end

local function bilinearSample(x, y, img)

	local xint, xfrac = math.floor(x), x % 1
	local yint, yfrac = math.floor(y), y % 1

	local a = img.getDefault(xint    , yint    , 0/0)
	local b = img.getDefault(xint + 1, yint    , 0/0)
	local c = img.getDefault(xint    , yint + 1, 0/0)
	local d = img.getDefault(xint + 1, yint + 1, 0/0)

	local value = lerp(
		lerp(a, c, yfrac),
		lerp(b, d, yfrac),
		xfrac
	)
	if x < 0 or y < 0 then assert(value ~= value) end
	return value
end

local function assignLandmasses(graph, outline)
	for _,center in ipairs(graph.centers) do
		local v = bilinearSample(center.y, center.x, outline)

		if v ~= v then	-- NaN
			center.kind = "out"
		else
			center.kind = v > 0.5 and "land" or "water"
		end
	end
end

local function markShores(graph)
	local shores = {}
	for _,center in ipairs(graph.centers) do
		if center.kind == "land" then
			for _,neighbour in ipairs(center.neighbours) do
				if neighbour.kind == "water" then
					table.insert(shores, center)
					center.shore = true
					break
				end
			end
		end
	end

	return shores
end

local function computeDistanceFromShore(graph, shores)

	local queue = shores
	local max = 0

	for _,v in ipairs(shores) do
		v.dist = 0
	end

	local i = 1
	while i < #queue do
		local v = queue[i]
		max = math.max(max, v.dist)

		if v.kind == "out" then
			print(v.dist)
		end

		for _,neigh in ipairs(v.neighbours) do
			if not neigh.dist or neigh.dist > v.dist + 1 then
				neigh.dist = v.dist + 1
				if neigh.kind ~= "out" then
					queue[#queue + 1] = neigh
				end
			end
		end

		i = i + 1
	end

	return max
end

local function computeRiverFlow(graph)

	for _,center in ipairs(graph.centers) do
		if center.kind ~= "land" or center.shore then
			goto skip
		end

		local lowest
		for _,neighbour in ipairs(center.neighbours) do
			if not lowest or neighbour.dist < lowest.dist then
				lowest = neighbour
			end
		end

		if lowest then
			center.downhill = lowest
			lowest.uphill = lowest.uphill or {}
			table.insert(lowest.uphill, center)
		end

		::skip::
	end

	local function rec(center)

		if not center.uphill or #center.uphill == 0 then
			center.flow = 1

		else
			local counts = {}
			local max = 0

			for _,up in ipairs(center.uphill) do
				local ups = rec(up)
				counts[ups] = (counts[ups] or 0) + 1
				max = math.max(max, ups)
			end

			assert(counts[max] > 0)
			if counts[max] == 1 then
				center.flow = max
			else
				center.flow = max + 1
			end
		end

		return center.flow
	end

	for _,center in ipairs(graph.centers) do
		if center.shore then
			rec(center)
		end
	end
end

local function line(ax, ay, bx, by, img, color)
	local steps = math.max(math.abs(ax-bx), math.abs(ay-by))

	for i = 0,steps do
		x = math.floor(lerp(ax, bx, i/steps))
		y = math.floor(lerp(ay, by, i/steps))
		img.setSafe(y, x, color)
	end
end

local function basicGraphOutput(graph, w, h)

	local img = image.new(w, h, 0x5555ffFF)

	for _,edge in ipairs(graph.edges) do
		if edge.corner2 then
			local ax,ay = edge.corner1.x, edge.corner1.y
			local bx,by = edge.corner2.x, edge.corner2.y
			line(ax, ay, bx, by, img, 0xff00ffff)
		end

		line(edge.center1.x, edge.center1.y, edge.center2.x, edge.center2.y, img, 0x000000FF)
	end

	tga.toFile("graph.tga", img)
end

local palette = {}
for i = 1,100 do
	palette[i] = image.pixel(i * 20, 255 - i * 20, 0)
end

local function shittyOutput(graph, w, h, max, s)
	s = s or 1
	local img = image.new(w * s, h * s, 0x5555ffFF)

	for _,center in ipairs(graph.centers) do

		if center.downhill and center.flow then
			local ax,ay = center.x * s, center.y * s
			local bx,by = center.downhill.x * s, center.downhill.y * s
			line(ax, ay, bx, by, img, palette[center.flow])
		end

	end

	for _,edge in ipairs(graph.edges) do
		if edge.corner2 then
			local ax,ay = edge.corner1.x * s, edge.corner1.y * s
			local bx,by = edge.corner2.x * s, edge.corner2.y * s
			line(ax, ay, bx, by, img, 0x101010ff)
		end

		--line(edge.center1.x, edge.center1.y, edge.center2.x, edge.center2.y, img, 0x000000FF)
	end

	tga.toFile("graph.tga", img)

end

local function main(args)
	local path = args[1]

	print("Loading " .. path)
	local outline = tga.fromFile(path).toGreyScale()
	local width, height = outline.width,outline.height

	print("Generating graph")
	local graph = generateGraph(width, height, 4)

	print("Assigning landmasses")
	assignLandmasses(graph, outline)
	local shores = markShores(graph)

	print("Computing distance from shore")
	local max = computeDistanceFromShore(graph, shores)
	print("Max dist", max)

	print("Generating rivers")
	computeRiverFlow(graph)

	print("Outputing")
	shittyOutput(graph, width, height, max, 2)
end

main {...}
