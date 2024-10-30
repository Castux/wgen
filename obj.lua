local function toFile(graph, path, width, height)

	local mtlPath = path:gsub(".obj", ".mtl")
	local pngPath = path:gsub(".obj", ".png"):gsub(".*/", "")

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

	local lines = {
		string.format("mtllib %s", mtlPath:gsub(".*/", "")),
		"usemtl material0"
	}

	for center,id in graph.iter "ce" do
		centerToIndex[center] = id

		local z = 0
		if center.z then
			z = (center.z - lowest) / (highest - lowest) * 255
			if center.shore then
				seaLevel = z
			end
		end

		table.insert(lines, string.format("v %f %f %f", center.x, z, center.y))
		table.insert(lines, string.format("vt %f %f", center.x / width, 1 - center.y / height))
	end

	for corner in graph.iter "co" do
		local ids = {}
		local out = false
		for center in corner.iter "c" do
			table.insert(ids, 1, centerToIndex[center])		-- reverse order, OBJ wants counter-clockwise
			if center.kind == "out" then
				out = true
			end
		end

		for i,v in ipairs(ids) do
			ids[i] = v .. "/" .. v
		end

		if not out then
			local line = "f " .. table.concat(ids, " ")
			table.insert(lines, line)
		end
	end

	local fp = io.open(path, "w")
	for _,line in ipairs(lines) do
		fp:write(line, "\n")
	end
	fp:close()

	-- Material file
	local fp = io.open(mtlPath, "w")
	fp:write("newmtl material0\n")
	fp:write("map_Kd ", pngPath)
	fp:close()
end


return
{
	toFile = toFile
}
