local d2 = require "d2"

local points
local edges

local function update()
	points = {}
	for i = 1,1000 do
		points[i] = {x = math.random() * 2000, y = math.random() * 2000}
	end

	edges = d2.delaunay(points)
end

function love.load()
	math.randomseed(os.time())
	love.window.setMode(2000, 2000)
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

end

function love.mousepressed()

	update()
end
