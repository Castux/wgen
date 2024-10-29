local d2 = require "d2"

local points
local edges
local co
local foo

local function newPoints()
	points = {}
	for i = 1,6000 do
		points[i] = {x = math.random() * 1200, y = math.random() * 800,
			vx = math.random(-20,20), vy = math.random(-20,20)
		}
	end

	foo = points[1]

	local now = os.clock()
	edges = d2.delaunay(points)
	local d = (os.clock() - now)
	print("d2.delaunay(points)", d)
end

if not love then
	newPoints()
	return
end

function love.load()
	math.randomseed(os.time())
	love.window.setMode(1200, 800)

	newPoints()
end

local function drawVoronoi(edges)


	love.graphics.setLineWidth(1)
	love.graphics.setColor(0.75,0,0)
	for _,edge in ipairs(edges) do
		if edge.rev and not edge.rev.done then
			local p1,p2,p3 = edge.from, edge.next.from, edge.next.next.from
			local cx, cy = d2.circumcenter(p1.x, p1.y, p2.x, p2.y, p3.x, p3.y)

			edge = edge.rev
			local p1,p2,p3 = edge.from, edge.next.from, edge.next.next.from
			local cx2, cy2 = d2.circumcenter(p1.x, p1.y, p2.x, p2.y, p3.x, p3.y)

			love.graphics.line(cx, cy, cx2, cy2)

			edge.rev.done = true
		end
	end

end

function love.update(dt)

	for _,p in ipairs(points) do

		p.x = p.x + p.vx * dt
		if p.x < 0 or p.x > 1200 then p.vx = -p.vx end
		p.y = p.y + p.vy * dt
		if p.y < 0 or p.y > 800 then p.vy = -p.vy end

	end


	edges = d2.delaunay(points)
end

function love.draw()
	love.graphics.clear(1,1,1)
	love.graphics.setPointSize(2)
	--
	-- love.graphics.setLineWidth(2)
	-- love.graphics.setColor(0,0,0.8)
  	-- for _,edge in ipairs(edges) do
	-- 	love.graphics.line(edge.from.x, edge.from.y, edge.to.x, edge.to.y)
	-- end
	--
	-- love.graphics.setPointSize(5)
	-- love.graphics.setColor(0,0,1)
	-- for _,edge in ipairs(d2.hull(edges)) do
	-- 	love.graphics.points(edge.from.x, edge.from.y)
	-- end

	drawVoronoi(edges)

end

function love.mousepressed()

	newPoints()
end
