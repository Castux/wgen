local array = require "array"

local function clamp(x, a, b)
	if x < a then return a end
	if x > b then return b end
	return x
end

local function pixel(r, g, b, a, denormalize)

	if denormalize then
		r = math.floor(r * 255)
		g = math.floor(g * 255)
		b = math.floor(b * 255)
		if a then a = math.floor(a * 255) end
	end

	r = clamp(r, 0, 255)
	g = clamp(g, 0, 255)
	b = clamp(b, 0, 255)
	a = clamp(a or 255, 0, 255)

	return
		r << 24 | g << 16 | b << 8 | a
end

local function rgba(int, normalize)
	local r, g, b, a =
		(int >> 24) & 0xff,
		(int >> 16) & 0xff,
		(int >>  8) & 0xff,
		(int >>  0) & 0xff

	if normalize then
		r, g, b, a = r / 255, g / 255, b / 255, a / 255
	end

	return r, g, b, a
end

local function new(width, height, fill)
	fill = fill or pixel(0, 0, 0, 255)

	local img = array.new(width, height, fill)

	img.getRGBA = function(row, col, normalize)
		return rgba(img.get(row, col), normalize)
	end

	img.setPixel = function(row, col, pixel)
		img.set(row, col, pixel)
	end

	img.setRGBA = function(row, col, r, g, b, a, denormalize)
		img.setPixel(row, col, pixel(r, g, b, a, denormalize))
	end

	img.iterRGBA = function(normalize)
		return coroutine.wrap(function()
			for row, col, pixel in img.iter() do
				coroutine.yield(row, col, rgba(pixel, normalize))
			end
		end)
	end

	img.toGreyScale = function()
		return array.new(img.width, img.height, function(r,c)
			local r,g,b,a = img.getRGBA(r,c)
			return (r + g + b) / 3 / 255
		end)
	end

	return img
end

return
{
	new = new,
	pixel = pixel
}
