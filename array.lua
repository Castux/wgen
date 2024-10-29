local function new(width, height, fill)

	local t = {}
	for row = 0, height-1 do
		t[row] = {}
		for col = 0, width-1 do
			t[row][col] = type(fill) == "function" and fill(row,col) or fill
		end
	end

	t.width = width
	t.height = height

	local function outBounds(row, col)
		return row < 0 or row >= height or col < 0 or col >= width
	end

	local function checkBounds(row, col)
		if outBounds(row, col) then
			error(string.format("Out of bounds: %d %d vs %d %d", row, col, height, width))
		end
	end

	t.get = function(row, col)
		checkBounds(row, col)
		return t[row][col]
	end

	t.set = function(row, col, value)
		checkBounds(row, col)
		t[row][col] = value
	end

	t.getDefault = function(row, col, default)
		if outBounds(row, col) then return default end
		return t[row][col]
	end

	t.setSafe = function(row, col, value)
		if outBounds(row, col) then return end
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

	t.copy = function()
		return new(width, height, function(row,col) return t[row][col] end)
	end

	return t
end

return
{
	new = new
}
