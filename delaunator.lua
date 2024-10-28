
local EPSILON = math.pow(2, -52)
local EDGE_STACK = {}

-- monotonically increases with real angle, but doesn't need expensive trigonometry
local function pseudoAngle(dx, dy)
	local p = dx / (math.abs(dx) + math.abs(dy))
	return (dy > 0 and 3 - p or 1 + p) / 4 -- [0..1]
end

local function dist(ax, ay, bx, by)
	local dx = ax - bx
	local dy = ay - by
	return dx * dx + dy * dy
end

local function inCircle(ax, ay, bx, by, cx, cy, px, py)
	local dx = ax - px
	local dy = ay - py
	local ex = bx - px
	local ey = by - py
	local fx = cx - px
	local fy = cy - py

	local ap = dx * dx + dy * dy
	local bp = ex * ex + ey * ey
	local cp = fx * fx + fy * fy

	return dx * (ey * cp - bp * fy) -
		   dy * (ex * cp - bp * fx) +
		   ap * (ex * fy - ey * fx) < 0
end

local function circumradius(ax, ay, bx, by, cx, cy)
	local dx = bx - ax
	local dy = by - ay
	local ex = cx - ax
	local ey = cy - ay

	local bl = dx * dx + dy * dy
	local cl = ex * ex + ey * ey
	local d = 0.5 / (dx * ey - dy * ex)

	local x = (ey * bl - dy * cl) * d
	local y = (dx * cl - ex * bl) * d

	return x * x + y * y
end

local function circumcenter(ax, ay, bx, by, cx, cy)
	local dx = bx - ax
	local dy = by - ay
	local ex = cx - ax
	local ey = cy - ay

	local bl = dx * dx + dy * dy
	local cl = ex * ex + ey * ey
	local d = 0.5 / (dx * ey - dy * ex)

	local x = ax + (ey * bl - dy * cl) * d
	local y = ay + (dx * cl - ex * bl) * d

	return {x, y}
end

local function swap(arr, i, j)
	local tmp = arr[i]
	arr[i] = arr[j]
	arr[j] = tmp
end

local function quicksort(ids, dists, left, right)
	if (right - left <= 20) then
		for i = left + 1, right do
			local temp = ids[i]
			local tempDist = dists[temp]
			local j = i - 1
			while j >= left and dists[ids[j]] > tempDist do
				ids[j + 1] = ids[j]
				j = j - 1
			end
			ids[j + 1] = temp
		end
	else
		local median = (left + right) // 2
		local i = left + 1
		local j = right
		swap(ids, median, i)
		if dists[ids[left]] > dists[ids[right]] then swap(ids, left, right) end
		if dists[ids[i]] > dists[ids[right]] then swap(ids, i, right) end
		if dists[ids[left]] > dists[ids[i]] then swap(ids, left, i) end

		local temp = ids[i]
		local tempDist = dists[temp]
		while true do
			repeat i = i + 1 until (dists[ids[i]] >= tempDist)
			repeat j = j - 1 until (dists[ids[j]] <= tempDist)
			if (j < i) then break end
			swap(ids, i, j)
		end
		ids[left + 1] = ids[j]
		ids[j] = temp

		if right - i + 1 >= j - left then
			quicksort(ids, dists, i, right)
			quicksort(ids, dists, left, j - 1)
		else
			quicksort(ids, dists, left, j - 1)
			quicksort(ids, dists, i, right)
		end
	end
end

local function hashKey(self, x, y)
	return math.floor(pseudoAngle(x - self._cx, y - self._cy) * self._hashSize) % self._hashSize
end


-- add a new triangle given vertex indices and adjacent half-edge ids
local function addTriangle(self, i0, i1, i2, a, b, c)
	local t = self.trianglesLen

	self._triangles[t] = i0
	self._triangles[t + 1] = i1
	self._triangles[t + 2] = i2

	self._link(t, a)
	self._link(t + 1, b)
	self._link(t + 2, c)

	self.trianglesLen = self.trianglesLen + 3

	return t
end

local function update(self)
	local coords = self.coords
	local hullPrev = self._hullPrev
	local hullNext = self._hullNext
	local hullTri = self._hullTri
	local hullHash = self._hullHash

	local n = (#coords.length + 1) / 2

	-- populate an array of point indices calculate input data bbox
	local minX = math.huge
	local minY = math.huge
	local maxX = -math.huge
	local maxY = -math.huge

	for i = 0, n-1 do
		local x = coords[2 * i]
		local y = coords[2 * i + 1]
		if x < minX then minX = x end
		if y < minY then minY = y end
		if x > maxX then maxX = x end
		if y > maxY then maxY = y end
		self._ids[i] = i
	end
	local cx = (minX + maxX) / 2
	local cy = (minY + maxY) / 2

	local i0, i1, i2

	-- pick a seed point close to the center
	local minDist = math.huge
	for i = 0, n-1 do
		local d = dist(cx, cy, coords[2 * i], coords[2 * i + 1])
		if d < minDist then
			i0 = i
			minDist = d
		end
	end
	local i0x = coords[2 * i0]
	local i0y = coords[2 * i0 + 1]

	-- find the point closest to the seed
	local minDist = math.huge
	for i = 0, n-1 do
		if i ~= i0 then
			local d = dist(i0x, i0y, coords[2 * i], coords[2 * i + 1])
			if d < minDist and d > 0 then
				i1 = i
				minDist = d
			end
		end
	end
	local i1x = coords[2 * i1]
	local i1y = coords[2 * i1 + 1]

	local minRadius = math.huge

	-- find the third point which forms the smallest circumcircle with the first two
	for i = 0,n-1 do
		if i ~= i0 and i ~= i1 then
			local r = circumradius(i0x, i0y, i1x, i1y, coords[2 * i], coords[2 * i + 1])
			if r < minRadius then
				i2 = i
				minRadius = r
			end
		end
	end
	local i2x = coords[2 * i2]
	local i2y = coords[2 * i2 + 1]

	if minRadius == math.huge then
		return nil, "No triangulation for this input"
	end

	-- swap the order of the seed points for counter-clockwise orientation
	if orient2d(i0x, i0y, i1x, i1y, i2x, i2y) < 0 then
		local i = i1
		local x = i1x
		local y = i1y
		i1 = i2
		i1x = i2x
		i1y = i2y
		i2 = i
		i2x = x
		i2y = y
	end

	local center = circumcenter(i0x, i0y, i1x, i1y, i2x, i2y)
	self._cx = center.x
	self._cy = center.y

	for i = 0, n-1 do
		self._dists[i] = dist(coords[2 * i], coords[2 * i + 1], center.x, center.y)
	end

	-- sort the points by distance from the seed triangle circumcenter
	quicksort(self._ids, self._dists, 0, n - 1)

	-- set up the seed triangle as the starting hull
	self._hullStart = i0
	local hullSize = 3

	hullNext[i0], hullPrev[i2] = i1, i1
	hullNext[i1], hullPrev[i0] = i2, i2
	hullNext[i2], hullPrev[i1] = i0, i0

	hullTri[i0] = 0
	hullTri[i1] = 1
	hullTri[i2] = 2

	hullHash = {}
	hullHash[hashKey(this, i0x, i0y)] = i0
	hullHash[hashKey(this, i1x, i1y)] = i1
	hullHash[hashKey(this, i2x, i2y)] = i2

	self.trianglesLen = 0
	addTriangle(self, i0, i1, i2, -1, -1, -1)

	local xp, yp = 0, 0

	for k = 0, #self._ids - 1 do
		local i = self._ids[k]
		local x = coords[2 * i]
		local y = coords[2 * i + 1]

		-- skip near-duplicate points
		if k > 0 and math.abs(x - xp) <= EPSILON and math.abs(y - yp) <= EPSILON then
			goto continue
		end
		xp = x
		yp = y

		-- skip seed triangle points
		if i == i0 or i == i1 or i == i2 then goto continue end

		-- find a visible edge on the convex hull using edge hash
		local start = 0
		local key = hashKey(self, x, y)
		for j = 0, this._hashSize - 1 do
			start = hullHash[(key + j) % self._hashSize]
			if start ~= nil and start ~= hullNext[start] then break end
		end

		start = hullPrev[start]
		local e = start, q
		while true do
			q = hullNext[e]
			if orient2d(x, y, coords[2 * e], coords[2 * e + 1], coords[2 * q], coords[2 * q + 1]) < 0 then
				break
			end
			e = q
			if (e == start) then
				e = -1
				break
			end
		end
		if e == -1 then goto continue end -- likely a near-duplicate point skip it

		-- add the first triangle from the point
		local t = addTriangle(self, e, i, hullNext[e], -1, -1, hullTri[e])

		-- recursively flip triangles from the point until they satisfy the Delaunay condition
		hullTri[i] = legalize(self, t + 2)
		hullTri[e] = t -- keep track of boundary triangles on the hull
		hullSize = hullSize + 1

		-- walk forward through the hull, adding more triangles and flipping recursively
		local n = hullNext[e]
		while true do
			q = hullNext[n]
			if orient2d(x, y, coords[2 * n], coords[2 * n + 1], coords[2 * q], coords[2 * q + 1]) >= 0 then
				break
			end
			t = addTriangle(self, n, i, q, hullTri[i], -1, hullTri[n])
			hullTri[i] = legalize(self, t + 2)
			hullNext[n] = n -- mark as removed
			hullSize = hullSize - 1
			n = q
		end

		-- walk backward from the other side, adding more triangles and flipping
		if e == start then
			while true do
				q = hullPrev[e]
				if orient2d(x, y, coords[2 * q], coords[2 * q + 1], coords[2 * e], coords[2 * e + 1]) >= 0 then
					break
				end

				t = addTriangle(self, q, i, e, -1, hullTri[e], hullTri[q])
				legalize(self, t + 2)
				hullTri[q] = t
				hullNext[e] = e -- mark as removed
				hullSize = hullSize - 1
				e = q
			end
		end

		-- update the hull indices
		self._hullStart, hullPrev[i] = e, e
		hullNext[e], hullPrev[n] = i, i
		hullNext[i] = n

		-- save the two new edges in the hash table
		hullHash[hashKey(self, x, y)] = i
		hullHash[hashKey(self, coords[2 * e], coords[2 * e + 1])] = e

		::continue::
	end

	self.hull = {}
	local e = self._hullStart
	for i = 0, hullSize - 1 do
		self.hull[i] = e
		e = hullNext[e]
	end

	-- trim typed triangle mesh arrays
	self.triangles = table.move(self._triangles, 0, self.trianglesLen - 1, 1, {})
	self.halfedges = table.move(self._halfedges, 0, self.trianglesLen - 1, 1, {})
end


--
-- 	_legalize(a) {
-- 		local {_triangles: triangles, _halfedges: halfedges, coords} = this
--
-- 		let i = 0
-- 		let ar = 0
--
-- 		-- recursion eliminated with a fixed-size stack
-- 		while (true) {
-- 			local b = halfedges[a]
--
-- 			/* if the pair of triangles doesn't satisfy the Delaunay condition
-- 			 * (p1 is inside the circumcircle of [p0, pl, pr]), flip them,
-- 			 * then do the same check/flip recursively for the new pair of triangles
-- 			 *
-- 			 *           pl                    pl
-- 			 *          /||\                  /  \
-- 			 *       al/ || \bl            al/    \a
-- 			 *        /  ||  \              /      \
-- 			 *       /  a||b  \    flip    /___ar___\
-- 			 *     p0\   ||   /p1   =>   p0\---bl---/p1
-- 			 *        \  ||  /              \      /
-- 			 *       ar\ || /br             b\    /br
-- 			 *          \||/                  \  /
-- 			 *           pr                    pr
-- 			 */
-- 			local a0 = a - a % 3
-- 			ar = a0 + (a + 2) % 3
--
-- 			if (b === -1) { -- convex hull edge
-- 				if (i === 0) break
-- 				a = EDGE_STACK[--i]
-- 				continue
-- 			}
--
-- 			local b0 = b - b % 3
-- 			local al = a0 + (a + 1) % 3
-- 			local bl = b0 + (b + 2) % 3
--
-- 			local p0 = triangles[ar]
-- 			local pr = triangles[a]
-- 			local pl = triangles[al]
-- 			local p1 = triangles[bl]
--
-- 			local illegal = inCircle(
-- 				coords[2 * p0], coords[2 * p0 + 1],
-- 				coords[2 * pr], coords[2 * pr + 1],
-- 				coords[2 * pl], coords[2 * pl + 1],
-- 				coords[2 * p1], coords[2 * p1 + 1])
--
-- 			if (illegal) {
-- 				triangles[a] = p1
-- 				triangles[b] = p0
--
-- 				local hbl = halfedges[bl]
--
-- 				-- edge swapped on the other side of the hull (rare) fix the halfedge reference
-- 				if (hbl === -1) {
-- 					let e = this._hullStart
-- 					do {
-- 						if (this._hullTri[e] === bl) {
-- 							this._hullTri[e] = a
-- 							break
-- 						}
-- 						e = this._hullPrev[e]
-- 					} while (e !== this._hullStart)
-- 				}
-- 				this._link(a, hbl)
-- 				this._link(b, halfedges[ar])
-- 				this._link(ar, bl)
--
-- 				local br = b0 + (b + 1) % 3
--
-- 				-- don't worry about hitting the cap: it can only happen on extremely degenerate input
-- 				if (i < EDGE_STACK.length) {
-- 					EDGE_STACK[i++] = br
-- 				}
-- 			} else {
-- 				if (i === 0) break
-- 				a = EDGE_STACK[--i]
-- 			}
-- 		}
--
-- 		return ar
-- 	}
--
-- 	_link(a, b) {
-- 		this._halfedges[a] = b
-- 		if (b !== -1) this._halfedges[b] = a
-- 	}
--
-- }


local function new(coords)

	assert(#coords % 2 == 0)
	local self = {}
	self.coords = coords

	-- arrays that will store the triangulation graph
	self._triangles = {}
	self._halfedges = {}

	-- temporary arrays for tracking the edges of the advancing convex hull
	self._hashSize = math.ceil(math.sqrt(n))
	self._hullPrev = {} -- edge to prev edge
	self._hullNext = {} -- edge to next edge
	self._hullTri = {} -- edge to adjacent triangle
	self._hullHash = {} -- angular edge hash

	-- temporary arrays for sorting points
	self._ids = {}
	self._dists = {}

	update(self)
end

local function from(points)
	local coords = {}

	for i,p in ipairs(points) do
		local i = i - 1
		coords[2 * i] = p.x
		coords[2 * i + 1] = p.y
	end

	return new(coords)
end
