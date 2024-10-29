local d2 = require "delauney"
local graph = require "graph"

local points
local g
local co
local foo


local function relax()
	for _,center in ipairs(g.centers) do

		local x,y = 0,0
		for _,corner in ipairs(center.corners) do
			x = x + corner.x
			y = y + corner.y
		end
		center.x = x / #center.corners
		center.y = y / #center.corners
	end

	g = graph.fromDelaunayHalfEdges(d2.delaunay(g.centers))
end


local function newPoints()
	points = {}
-- 	while #points < 4*10000 do
-- 		local p = {
-- 			x = math.random() * love.graphics.getWidth(),
-- 			y = math.random() * love.graphics.getHeight(),
-- --			vx = math.random(-20,20), vy = math.random(-20,20)
-- 		}
-- 		table.insert(points, p)
-- 	end
	local res = 16
	for x = 0,love.graphics.getWidth(), res do
		for y = 0,love.graphics.getHeight(), res do
			table.insert(points, {
				x = x + math.random() * res * 1.5,
				y = y + math.random() * res * 1.5
			})
		end
	end

	print(#points)

	local now = os.clock()
	edges = d2.delaunay(points)
	local d = (os.clock() - now)
	print("d2.delaunay(points)", d)

	local now = os.clock()
	g = graph.fromDelaunayHalfEdges(edges)
	local d = (os.clock() - now)
	print("graph.fromDelaunayHalfEdges(edges)", d)
end

if not love then
	newPoints()

	local now = os.clock()
	relax()
	local d = (os.clock() - now)
	print("relax()", d)

	return
end

function love.load()
	math.randomseed(os.time())
	love.window.setMode(1024*16, 1024*16)

	newPoints()
	relax()
	relax()

	love.graphics.captureScreenshot("out.png")
end

local voronoi = false
function love.draw()
	love.graphics.clear(1,1,1)

	-- for i,center in ipairs(g.centers) do
	--
	-- 	-- love.graphics.setPointSize(4)
	-- 	-- love.graphics.points(center.x, center.y)
	--
	-- 	-- if #center.corners >= 3 then
	-- 	-- 	local coords = {}
	-- 	-- 	for _,c in ipairs(center.corners) do
	-- 	-- 		table.insert(coords, c.x)
	-- 	-- 		table.insert(coords, c.y)
	-- 	-- 	end
	-- 	--
	-- 	-- 	love.graphics.setColor(0,1,1)
	-- 	-- 	love.graphics.polygon("line", coords)
	-- 	-- end
	--
	--
	-- end

	love.graphics.setColor(0,1,1)

	for _,edge in ipairs(g.edges) do

		if voronoi then
			love.graphics.line(edge.center1.x, edge.center1.y, edge.center2.x, edge.center2.y)
		else
			if edge.corner2 then
				love.graphics.line(edge.corner1.x, edge.corner1.y, edge.corner2.x, edge.corner2.y)
			end
		end
	end


--	drawVoronoi(edges)

end

function love.keypressed()
	voronoi = not voronoi
end

function love.mousepressed()

	--newPoints()
	relax()
end
