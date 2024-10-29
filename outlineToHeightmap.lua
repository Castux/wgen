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

	local xint, xfrac = x // 1, x % 1
	local yint, yfrac = y // 1, y % 1

	local a = img.getDefault(xint    , yint    , 0)
	local b = img.getDefault(xint + 1, yint    , 0)
	local c = img.getDefault(xint    , yint + 1, 0)
	local d = img.getDefault(xint + 1, yint + 1, 0)

	return lerp(
		lerp(a, c, yfrac),
		lerp(b, d, yfrac),
		xfrac
	)
end

local function assignLandmasses(graph, outline)
	for _,center in ipairs(graph.centers) do
		local v = bilinearSample(center.y, center.x, outline)
		center.land = v > 0.5
	end
end

local function shittyOutput(graph, outline)
	local img = image.new(outline.width, outline.height, 0x0000FFFF)

	for _,center in ipairs(graph.centers) do
		if center.land then
			local x,y = math.floor(center.x), math.floor(center.y)
			img.setSafe(y,x, 0xFF0000FF)
		end
	end

	tga.toFile("graph.tga", img)

end

local function main(args)
	local path = args[1]

	local outline = tga.fromFile(path).toGreyScale()
	local width, height = outline.width,outline.height

	local graph = generateGraph(width, height, 4)
	assignLandmasses(graph, outline)

	shittyOutput(graph, outline)
end

main {...}
