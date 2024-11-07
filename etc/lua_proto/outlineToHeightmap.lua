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

local colors = {
	[image.pixel(66, 66, 125)] = "sea",

	[image.pixel(135, 168, 81)] = "plains",
	[image.pixel(209, 184, 134)] = "hills",
	[image.pixel(101, 72, 31)] = "mountains",

	[image.pixel(109, 148, 194)] = "lake",
	[image.pixel(153, 153, 153)] = "flat",
	[image.pixel(148, 10, 0)] = "cliffs",

}

local function assignCellTypes(graph, outline)
	for center in graph.iter "ce" do

		local row, col = math.floor(center.y), math.floor(center.x)
		local v = outline.getDefault(row, col)
		if not v then
			center.kind = "out"
		else
			center.kind = colors[v]
		end

		if not center.kind then
			local r,g,b,a = image.rgba(v)
			error(string.format("Bad color %d %d %d %d at %d %d", r, g, b, a, row, col))
		end
	end
end

local function markShores(graph)
	local shores = {}
	for center in graph.iter "ce" do
		if center.kind ~= "out" and center.kind ~= "sea" then
			for neighbour in center.iter "n" do
				if neighbour.kind == "sea" then
					table.insert(shores, center)
					center.shore = true
					break
				end
			end
		end
	end

	return shores
end

local gradients = {
	sea = 0.5/3,
	plains = 1/3,
	hills = 2/3,
	mountains = 4/3,
	lake = 0.001/3,
	flat = 0.2/3,
	cliffs = 8/3
}

local gradientSmoothing = 0.6

local function computeElevation(graph, shores, resolution)

	assert(#shores > 0)

	local queue = {}

	for _,v in ipairs(shores) do
		v.z = 0
		v.gradient = gradients[v.kind]
		table.insert(queue, v)
	end

	local i = 1
	while i <= #queue do
		local v = queue[i]
		for neigh in v.iter "n" do
			local gradient = gradients[neigh.kind]
			if gradient then

				if neigh.kind ~= "lake" then
					gradient = lerp(gradient, v.gradient, gradientSmoothing)
				end

				assert(gradient > 0)

				local new = v.z + gradient * resolution
				if not neigh.z or neigh.z > new then
					neigh.gradient = gradient
					neigh.z = new
					queue[#queue + 1] = neigh
				end
			end
		end

		i = i + 1
	end

	local min,max = math.huge, -math.huge
	for center in graph.iter "ce" do
		if center.kind == "sea" then
			center.z = -center.z
		end
		if center.z then
			min = math.min(min, center.z)
			max = math.max(max, center.z)
		end
	end

	return min,max
end

local function computeRiverFlow(graph, shores)

	-- Find the steepest downhill from every point

	for center in graph.iter "ce" do
		if not center.z then
			goto skip
		end

		local lowest
		for neighbour in center.iter "n" do
			if neighbour.z and neighbour.z < center.z 		-- It should acctually be downhill (avoid rivers along shores for ex.)
				and (not lowest or neighbour.z < lowest.z) then
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

	-- Go up from the shores to compute flows

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

	for _,center in ipairs(shores) do
		rec(center)
	end
end

local function output(graph, w, h, maxElevation, resolution, path)
	local svg = require "EzSVG"
	local doc = svg.Document(w,h, "darkblue")

	local land = svg.Group()
	local rivers = svg.Group()
	local sea = svg.Group()

	for center in graph.iter "ce" do

		local coords = {}
		for corner in center.iter "c" do
			table.insert(coords, corner.x)
			table.insert(coords, corner.y)
		end

		local color
		local group

		if center.kind == "lake" then
			color = "#0E443D"
			group = land

		elseif center.kind == "sea" then
			local f = -center.z / maxElevation
			color = svg.rgb(
				lerp(95, 0, f),
				lerp(132, 10, f),
				lerp(255, 100, f)
			)
			group = sea

		elseif center.kind == "out" then
			color = "pink"
			group = sea

		else
			local f = center.z / maxElevation
			color = svg.rgb(
				lerp(84, 255, f),
				lerp(169, 255, f),
				lerp(50, 255, f)
			)
			group = land
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

	os.execute(string.format("rsvg-convert %s > %s", path, path:gsub(".svg", ".png")))
end

local function outputHeightmap(graph, w, h, path)

	local svg = require "EzSVG"
	local doc = svg.Document(w,h, "black")

	local lowest,highest = math.huge, -math.huge
	for center in graph.iter "ce" do
		if center.z then
			lowest = math.min(lowest, center.z)
			highest = math.max(highest, center.z)
		end
	end

	local newSeaLevel
	for center in graph.iter "ce" do

		local coords = {}
		for corner in center.iter "c" do
			table.insert(coords, corner.x)
			table.insert(coords, corner.y)
		end

		local color
		if center.z then
			local d = (center.z - lowest) / (highest - lowest) * 255
			if center.shore then newSeaLevel = d end
			color = svg.rgb(d, d, d)
		else
			color = "pink"
		end
		doc:add(svg.Polygon(coords, {fill = color, stroke = color}))
	end

	print(string.format("Scaled [%f,%f] to [0,255]. Sea level %.2f.", lowest, highest, newSeaLevel))

	doc:writeTo(path)
end

local function main(args)
	local path = args[1]
	local resolution = args[2] or 10
	local heightmap = args[3] == "-h"

	print("Loading " .. path)
	local outline = tga.fromFile(path)
	local width, height = outline.width, outline.height

	print("Generating graph...")
	local graph = generateGraph(width, height, resolution)
	print(#graph.centers .. " points")

	print("Assigning landmasses")
	assignCellTypes(graph, outline)

	print("Marking shores")
	local shores = markShores(graph)

	print("Computing elevation")
	local min,max = computeElevation(graph, shores, resolution)
	print("Elevation range:", min, max)

	print("Generating rivers")
	computeRiverFlow(graph, shores)

	print("Outputing")

	output(graph, width, height, max, resolution, path:gsub(".tga", ".svg"))
	outputHeightmap(graph, width, height, path:gsub(".tga", "-h.svg"))
	obj.toFile(graph, path:gsub(".tga", ".obj"), width, height)
end

main {...}
