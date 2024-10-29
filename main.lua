local d2 = require "d2"

local points
local edges

local function update()
	points = {}
	for i = 1,100000 do
		points[i] = {x = math.random() * 1200, y = math.random() * 800}
	end

	edges = d2.delaunay(points)
end

function love.load()
	math.randomseed(os.time())
	love.window.setMode(1200, 800)
	update()
end

function love.draw()
	love.graphics.setPointSize(2)
	love.graphics.setColor(1,1,1)
	for i,p in ipairs(points) do
		love.graphics.points(p.x, p.y)
	end

	love.graphics.setLineWidth(1)
	love.graphics.setColor(1,1,1)
  	for _,edge in ipairs(edges) do
		love.graphics.line(edge.from.x, edge.from.y, edge.to.x, edge.to.y)
	end

	love.graphics.setPointSize(4)
	love.graphics.setColor(0,1,0)
	local curr = edges.hull
	repeat
		love.graphics.points(curr.from.x, curr.from.y)
		curr = curr.hullNext
	until curr == edges.hull

end

function love.mousepressed()

	update()
end
