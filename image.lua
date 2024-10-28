local function clamp(x,a,b)
	if x < a then return a end
	if x > b then return b end
	return x
end

local function pixel(r,g,b,a,denormalize)

	if denormalize then
		r = math.floor(r * 255)
		g = math.floor(g * 255)
		b = math.floor(b * 255)
		if a then a = math.floor(a * 255) end
	end

	r = clamp(r,0,255)
	g = clamp(g,0,255)
	b = clamp(b,0,255)
	a = clamp(a or 255,0,255)

	return
		b << 24 | g << 16 | r << 8 | a
end

local function new(width, height, fill)
	fill = fill or pixel(0,0,0,255)

	local data = {}
	for row = 0,height-1 do
		data[row] = {}
		for col = 0,width-1 do
			data[row][col] = fill
		end
	end

	local img = {
		width = width,
		height = height,
		data = data
	}

	img.getRGBA = function(row,col,normalize)
		local int = data[row][col]
		local r,g,b,a =
			(int >>  8) & 0xff,
			(int >> 16) & 0xff,
			(int >> 24) & 0xff,
			(int >>  0) & 0xff

		if normalize then
			r,g,b,a = r / 255, g / 255, b / 255, a / 255
		end

		return r,g,b,a
	end

	img.setPixel = function(row,col,pixel)
		data[row][col] = pixel
	end

	img.setRGBA = function(row,col,r,g,b,a,denormalize)
		img.setPixel(row,col,pixel(r,g,b,a,denormalize))
	end

	img.iter = function()
		return coroutine.wrap(function()
			for row = 0,img.height-1 do
				for col = 0,img.width-1 do
					coroutine.yield(row,col,data[row][col])
				end
			end
		end)
	end

	img.iterRGBA = function(normalize)
		return coroutine.wrap(function()
			for row = 0,img.height-1 do
				for col = 0,img.width-1 do
					coroutine.yield(row,col,img.getRGBA(row,col,normalize))
				end
			end
		end)
	end

	return img
end

return
{
	new = new,
	pixel = pixel
}
