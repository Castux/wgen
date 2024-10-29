local d2 = require "d2"
local graph = require "graph"

local points
local g
local co
local foo


local function relax()

	local points = {}
	for _,center in ipairs(g.centers) do

		local x,y = 0,0
		for _,corner in ipairs(center.corners) do
			x = x + corner.x
			y = y + corner.y
		end

		table.insert(points, {
			x = x / #center.corners,
			y = y / #center.corners
		})
	end

	g = graph.fromDelaunayHalfEdges(d2.delaunay(points))
	print"relaxed"
end


local function newPoints()
	points = {}
	while #points < 1000000 do
		local p = {x = math.random() * 1200, y = math.random() * 800,
			vx = math.random(-20,20), vy = math.random(-20,20)
		}
		--if (p.x - 600)^2 + (p.y - 400)^2 < 400^2 then
			table.insert(points, p)
		--end
	end
	local b = 20
	table.insert(points, {x = -b, y = -b, vx = 0, vy = 0})
	table.insert(points, {x = -b, y = 800+b, vx = 0, vy = 0})
	table.insert(points, {x = 1200+b, y = -b, vx = 0, vy = 0})
	table.insert(points, {x = 1200+b, y = 800+b, vx = 0, vy = 0})

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
	love.window.setMode(1200, 800)

	newPoints()
end

function love.update(dt)

	-- for _,p in ipairs(points) do
	--
	-- 	p.x = p.x + p.vx * dt
	-- 	if p.x < 0 or p.x > 1200 then p.vx = -p.vx end
	-- 	p.y = p.y + p.vy * dt
	-- 	if p.y < 0 or p.y > 800 then p.vy = -p.vy end
	--
	-- end


--	relax()
end

function love.draw()
	love.graphics.clear(1,1,1)

	for i,center in ipairs(g.centers) do

		-- love.graphics.setPointSize(4)
		-- love.graphics.points(center.x, center.y)

		if #center.corners >= 3 then
			local coords = {}
			for _,c in ipairs(center.corners) do
				table.insert(coords, c.x)
				table.insert(coords, c.y)
			end

			love.graphics.setColor(0,1,1)
			love.graphics.polygon("line", coords)
		end

	end


--	drawVoronoi(edges)

end

function love.mousepressed()

	--newPoints()
	relax()
end
