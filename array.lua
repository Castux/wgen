local function new(width, height, fill)

	local t = {}
	for row = 0, height-1 do
		t[row] = {}
		for col = 0, width-1 do
			t[row][col] = fill
		end
	end

	t.width = width
	t.height = height

	local function inBounds(row, col)
		if row < 0 or row >= height or col < 0 or col >= width then
			error(string.format("Out of bounds: %d %d vs %d %d", row, col, height, width))
		end
	end

	t.get = function(row, col)
		inBounds(row, col)
		return t[row][col]
	end

	t.set = function(row, col, value)
		inBounds(row, col)
		t[row][col] = value
	end

	t.iter = function()
		return coroutine.wrap(function()
			for row = 0, height-1 do
				for col = 0, width-1 do
					coroutine.yield(row, col, t[row][col])
				end
			end
		end)
	end

	return t
end

return
{
	new = new
}
