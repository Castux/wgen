
local mesh = require "mesh"

local points
local tris

local function update()
	points = {}

	for x = 0,999 do
		for y = 0,999 do
			if math.random() < 1/(200^2) then
				table.insert(points, {x=x, y=y})
			end
		end
	end
	print(#points)
	tris = mesh.delaunay(points)
end

function love.load()
	math.randomseed(os.time())
	love.window.setMode(1000, 1000)
	update()
end

function love.draw()
	love.graphics.setPointSize(3)
	for i,p in ipairs(points) do
		love.graphics.points(p.x, p.y)
	end

  for i,tri in ipairs(tris) do
	if i == 1 then
		love.graphics.setColor(1,0,0)
	else
		love.graphics.setColor(1,1,1)
	end
	love.graphics.polygon("line", tri[1].x, tri[1].y, tri[2].x, tri[2].y, tri[3].x, tri[3].y)

	for j,p in ipairs(tri) do
		love.graphics.print(i .. ":" .. j, p.x, p.y + (i%2) * 10)
	end
  end

end

function love.mousepressed()

	update()
end
