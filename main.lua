
local mesh = require "mesh"

local points
local tris
local border

local function update()
	points = {}
	for i = 1,100000 do
		points[i] = {x = math.random() * 2000, y = math.random() * 2000}
	end

	tris,border = mesh.delaunay(points)
end

function love.load()
	math.randomseed(os.time())
	love.window.setMode(2000, 2000)
	update()
end

function love.draw()
	love.graphics.setPointSize(2)
	love.graphics.setColor(1,1,1)
	-- for i,p in ipairs(points) do
	-- 	love.graphics.points(p.x, p.y)
	-- end

	love.graphics.setLineWidth(1)
  for i,tri in ipairs(tris) do
	if i == 1 then
		love.graphics.setColor(1,0,0)
	else
		love.graphics.setColor(1,1,1)
	end
	love.graphics.polygon("line", tri[1].x, tri[1].y, tri[2].x, tri[2].y, tri[3].x, tri[3].y)

	-- for j,p in ipairs(tri) do
	-- 	love.graphics.print(i .. ":" .. j, p.x, p.y + (i%2) * 10)
	-- end

	-- love.graphics.setPointSize(5)
	-- love.graphics.setColor(0,1,0)
	-- local curr = border
	-- repeat
	-- 	local p = curr.tri[curr.index]
	-- 	love.graphics.points(p.x, p.y)
	-- 	curr = curr.next
	--
	-- until curr == border
  end

end

function love.mousepressed()

	update()
end
