local tga = require "tga"
local image = require "image"
local delaunay = require "delaunay"
local graph = require "graph"


local function relaxGraph(g)

	for _,center in ipairs(g.centers) do

		local x,y = 0,0
		for _,corner in ipairs(center.corners) do
			x = x + corner.x
			y = y + corner.y
		end
		center.x = x / #center.corners
		center.y = y / #center.corners
	end

	local edges = delaunay.delaunay(g.centers)
	return graph.fromDelaunayHalfEdges(edges)
end

local function generateGraph(w, h, res)

	local points = {}

	for x = 0, w, res do
		for y = 0, h, res do
			table.insert(points, {
				x = x + math.random() * res * 1.5,
				y = y + math.random() * res * 1.5
			})
		end
	end

	local edges = delaunay.delaunay(points)
	local graph = graph.fromDelaunayHalfEdges(edges)
	graph = relaxGraph(graph)
	graph = relaxGraph(graph)

	return graph
end

local function lerp(a,b,x)
	return a * (1-x) + b * x
end

local function bilinearSample(x, y, img)

	x = x * (img.width - 1)
	y = y * (img.height - 1)

	local xint, xfrac = x // 1, x % 1
	local yint, yfrac = y // 1, y % 1

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

local function shittyOutput(graph, w, h, max)
	local img = image.new(w, h, 0x000000FF)

	for _,center in ipairs(graph.centers) do
		local x,y = math.floor(center.x * w), math.floor(center.y * h)

		local color
		if center.shore then
			color = 0xffff00ff
		elseif center.kind == "land" then
			color = image.pixel(1, 0, center.dist/max, 1, "denorm")
		elseif center.kind == "water" then
			color = image.pixel(0, 1, center.dist/max, 1, "denorm")
		elseif center.kind == "out" then
			color = 0xffffffff
		end

		img.setSafe(y,x, color)
	end

	tga.toFile("graph.tga", img)

end

local function main(args)
	local path = args[1]

	local outline = tga.fromFile(path).toGreyScale()
	local width, height = outline.width,outline.height

	local graph = generateGraph(1, 1, 4/(2048*2))
	assignLandmasses(graph, outline)
	local shores = markShores(graph)

	local max =  computeDistanceFromShore(graph, shores)
	print("Max dist", max)

	shittyOutput(graph, 2048*2, 2048*2, max)
end

main {...}
