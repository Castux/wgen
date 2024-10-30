local tga = require "tga"
local image = require "image"
local delaunay = require "delaunay"
local graph = require "graph"
local obj = require "obj"

local function relaxGraph(g, w, h)

	local new = {}
	for center in g.iter "ce" do
		if center.x < 0 or center.x > w or center.y < 0 or center.y > h then
			table.insert(new, {x = center.x, y = center.y})
		else
			local x,y = 0,0
			for corner in center.iter "c" do
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
	local margin = 100

	for x = 0, w, res do
		for y = 0, h, res do
			table.insert(points, {
				x = x + (math.random() -  0.5) * res * 1.0,
				y = y + (math.random() -  0.5) * res * 1.0
			})
		end
	end

	for x = -margin, w + margin, res do
		table.insert(points, {x = x, y = -margin})
		table.insert(points, {x = x, y = h + margin})
	end

	for y = -margin, h + margin, res do
		table.insert(points, {x = -margin, y = y})
		table.insert(points, {x = w + margin, y = y})
	end

	local edges = delaunay.delaunay(points)
	local graph = graph.fromDelaunayHalfEdges(edges)

	print("Relaxing")
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
	for center in graph.iter "ce" do
		local v = bilinearSample(center.y, center.x, outline)

		if v ~= v then	-- NaN
			center.kind = "out"
		else
			center.kind = v > 0.5 and "land" or "water"
		end
	end
	--
	-- for _,center in ipairs(graph.centers) do
	-- 	if center.kind == "out" then
	-- 		for _,n in ipairs(center.neighbours) do
	-- 			if n.kind == "land" then
	-- 				center.kind = "land"
	-- 				break
	-- 			end
	-- 		end
	--
	-- 		if center.kind == "out" then
	-- 			center.kind = "water"
	-- 		end
	-- 	end
	-- end
end

local function isLake(start)

	local added = {}
	local foundOut = false

	local queue = {start}
	added[start] = true

	local i = 1
	while i <= #queue do
		local c = queue[i]

		for neighbour in c.iter "n" do
			if neighbour.kind == "out" then
				foundOut = true

			elseif not added[neighbour] and neighbour.kind == "water" then
				table.insert(queue, neighbour)
				added[neighbour] = true
			end
		end

		i = i + 1
	end

	return not foundOut, added
end

local function markLakes(graph)

	local done = {}

	for center in graph.iter "ce" do

		if center.kind == "water" and not done[center] then
			local lake, cells = isLake(center)
			for k in pairs(cells) do
				done[k] = true
				if lake then
					k.kind = "lake"
				end
			end
		end
	end

end

local function markShores(graph)
	local shores = {}
	for center in graph.iter "ce" do
		if center.kind == "land" then
			for neighbour in center.iter "n" do
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

	assert(#shores > 0)

	local queue = shores
	local max = 0

	for _,v in ipairs(shores) do
		v.dist = 0
	end

	local i = 1
	while i <= #queue do
		local v = queue[i]
		max = math.max(max, v.dist)

		for neigh in v.iter "n" do

			local new
			if neigh.kind == "lake" then
				new = v.dist + 0.001
			else
				new = v.dist + 1
			end

			if not neigh.dist or neigh.dist > new then
				neigh.dist = new
				if neigh.kind ~= "out" then
					queue[#queue + 1] = neigh
				end
			end
		end

		i = i + 1
	end

	for center in graph.iter "ce" do
		if center.kind == "water" then
			center.dist = -center.dist
		end
	end

	return max
end

local function computeRiverFlow(graph)

	for center in graph.iter "ce" do
		if center.kind ~= "land" and center.kind ~= "lake" then
			goto skip
		end

		local lowest
		for neighbour in center.iter "n" do
			if not lowest or (neighbour.dist and neighbour.dist < lowest.dist) then
				lowest = neighbour
			end
		end

		if lowest then
			center.downhill = lowest
			lowest.uphill = lowest.uphill or {}
			table.insert(lowest.uphill, center)

		else
			print "===="
			print(center.dist)
			for _,n in ipairs(center.neighbours) do print(n.dist) end
		end

		::skip::
	end

	local function rec(center)

		if not center.uphill or #center.uphill == 0 then
			center.flow = 1

		else
			local sum = 1
			for _,up in ipairs(center.uphill) do
				sum = sum + rec(up)
			end
			center.flow = sum
		end

		return center.flow
	end

	for center in graph.iter "ce" do
		if center.shore then
			rec(center)
		end
	end
end

local function output(graph, w, h, maxDist, resolution, path)
	local svg = require "EzSVG"
	local doc = svg.Document(w,h, "darkblue")


	local land = svg.Group()
	local rivers = svg.Group()
	local sea = svg.Group()

	maxDist = maxDist * 0.75

	for center in graph.iter "ce" do

		local coords = {}
		for corner in center.iter "c" do
			table.insert(coords, corner.x)
			table.insert(coords, corner.y)
		end

		local color
		local group

		if center.kind == "land" then

			local f = (center.dist / maxDist)^2
			color = svg.rgb(
				lerp(84, 255, f),
				lerp(169, 255, f),
				lerp(50, 255, f)
			)
			group = land
		elseif center.kind == "lake" then
			color = "#0E443D"
			group = land

		elseif center.kind == "water" then
			local f = (-center.dist / maxDist)^0.25
			color = svg.rgb(
				lerp(95, 0, f),
				lerp(132, 10, f),
				lerp(255, 100, f)
			)
			group = sea
		else
			color = "pink"
			group = sea
		end

		group:add(svg.Polygon(coords, {fill = color, stroke = color}))

		if center.downhill and center.flow then

			local width = center.flow^0.5 * (resolution/30)^2

			rivers:add(svg.Line(center.x, center.y,
				center.downhill.x, center.downhill.y,
				{stroke = "#0E443D", stroke_width = width}
				--{stroke = "blue", stroke_width = width}
			))
		end
	end

	-- for edge in graph.iter "e" do
	-- 	if edge.corner2 then
	-- 		local ax,ay = edge.corner1.x, edge.corner1.y
	-- 		local bx,by = edge.corner2.x, edge.corner2.y
	-- 		doc:add(svg.Line(ax, ay, bx, by, {stroke = "blue"}))
	-- 	end
	--
	-- 	land:add(svg.Line(edge.center1.x, edge.center1.y, edge.center2.x, edge.center2.y, {stroke = "yellow"}))
	-- end

	local container = svg.Group()
	container:add(land)
	container:add(rivers)
	container:add(sea)
	--container:scale(0.5):translate(w/2, h/2)

	doc:add(container)
	doc:writeTo(path)
end

local function outputHeightmap(graph, w, h, path)

	local svg = require "EzSVG"
	local doc = svg.Document(w,h, "black")

	local lowest,highest = math.huge, -math.huge
	for center in graph.iter "ce" do
		if center.dist then
			lowest = math.min(lowest, center.dist)
			highest = math.max(highest, center.dist)
		end
	end

	local newSeaLevel
	for center in graph.iter "ce" do

		local coords = {}
		for corner in center.iter "c" do
			table.insert(coords, corner.x)
			table.insert(coords, corner.y)
		end

		local d = 0
		if center.dist then
			d = (center.dist - lowest) / (highest - lowest) * 255
			if center.shore then newSeaLevel = d end
		end
		local color = svg.rgb(d, d, d)
		doc:add(svg.Polygon(coords, {fill = color, stroke = color}))
	end

	print(string.format("Scaled [%d,%d] to [0,255]. Sea level %.2f.", lowest, highest, newSeaLevel))

	doc:writeTo(path)
end

local function main(args)
	local path = args[1]
	local resolution = args[2] or 10
	local heightmap = args[3] == "-h"

	print("Loading " .. path)
	local outline = tga.fromFile(path).toGreyScale()
	local width, height = outline.width,outline.height

	print("Generating graph...")
	local graph = generateGraph(width, height, resolution)
	print(#graph.centers .. " points")

	print("Assigning landmasses")
	assignLandmasses(graph, outline)

	print("Marking lakes")
	markLakes(graph)

	print("Marking shores")
	local shores = markShores(graph)

	print("Computing distance from shore")
	local max = computeDistanceFromShore(graph, shores)
	print("Max dist", max)

	print("Generating rivers")
	computeRiverFlow(graph)

	print("Outputing")

	output(graph, width, height, max, resolution, path:gsub(".tga", ".svg"))
	outputHeightmap(graph, width, height, path:gsub(".tga", "-h.svg"))
	obj.toFile(graph, path:gsub(".tga", "-h.obj"), width, height)
end

main {...}
