local function toFile(graph, path, width, height)

	local lowest,highest = math.huge, -math.huge
	for center in graph.iter "ce" do
		if center.z then
			lowest = math.min(lowest, center.z)
			highest = math.max(highest, center.z)
		end
	end

	local lines = {}
	local centerToIndex = {}
	local seaLevel

	for center,id in graph.iter "ce" do
		centerToIndex[center] = id

		local z = 0
		if center.z then
			z = (center.z - lowest) / (highest - lowest) * 255
			if center.shore then
				seaLevel = z
			end
		end

		local line = string.format("v %f %f %f", center.x, z, center.y)
		table.insert(lines, line)
	end

	-- Add water plane corners
	table.insert(lines, string.format("v %f %f %f", 0, seaLevel, 0))
	table.insert(lines, string.format("v %f %f %f", 0, seaLevel, height))
	table.insert(lines, string.format("v %f %f %f", width, seaLevel, height))
	table.insert(lines, string.format("v %f %f %f", width, seaLevel, 0))

	for corner in graph.iter "co" do
		local ids = {}
		local out = false
		for center in corner.iter "c" do
			table.insert(ids, 1, centerToIndex[center])		-- reverse order, OBJ wants counter-clockwise
			if center.kind == "out" then
				out = true
			end
		end

		if not out then
			local line = "f " .. table.concat(ids, " ")
			table.insert(lines, line)
		end
	end

	-- Add water plane
	local id = #graph.centers + 1
	table.insert(lines, string.format("f %d %d %d %d", id, id + 1, id + 2, id + 3))

	local fp = io.open(path, "w")
	for _,line in ipairs(lines) do
		fp:write(line, "\n")
	end
	fp:close()
end


return
{
	toFile = toFile
}
