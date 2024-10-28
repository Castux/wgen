local tga = require "tga"
local image = require "image"

local img = tga.fromFile "pink.tga"

--local img = image.new(1024,1024)

for row,col,r,g,b,a in img.iterRGBA() do
	img.setRGBA(row,col,
		(r+b)//2,
		r,
		0
	)
end

tga.toFile("out.tga", img)
