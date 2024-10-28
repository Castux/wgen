local image = require "image"

local function write(path, width, height, data)
	local fp = io.open(path, "wb")

	local function writeByte(b)
		fp:write(string.char(b))
	end

	local function writeShort(s)
		fp:write(string.char(s & 0xff))
		fp:write(string.char((s >> 8) & 0xff))
	end

	local function writeInt(i)
		fp:write(string.char((i >> 24) & 0xff))
		fp:write(string.char((i >> 16) & 0xff))
		fp:write(string.char((i >> 8) & 0xff))
		fp:write(string.char((i >> 0) & 0xff))
	end

	writeByte(0) -- idLength
	writeByte(0) -- colorMapType
	writeByte(2) -- imgType: uncompressed RGB
	writeShort(0) -- colorMapFirst
	writeShort(0) -- colorMapLength
	writeByte(0) -- colorMapEntrySize

	writeShort(0) -- xOrigin
	writeShort(0) -- yOrigin
	writeShort(width)
	writeShort(height)
	writeByte(32)
	writeByte(40) -- descriptor: 8 bit alpha, top to bottom

	for row = 0,height-1 do
		for col = 0,width-1 do
			writeInt(data[row][col])
		end
	end

	fp:close()
end

local function fromFile(path)
	local fp = io.open(path, "rb")

	local function readByte()
		return fp:read(1):byte()
	end

	local function readShort()
		return readByte() + readByte() * 256
	end

	local idLength = readByte()
	local colorMapType = readByte()
	local imgType = readByte()
	local colorMapFirst = readShort()
	local colorMapLength = readShort()
	local colorMapEntrySize = readByte()

	local xOrigin = readShort()
	local yOrigin = readShort()
	local width = readShort()
	local height = readShort()
	local depth = readByte()

	local descriptor = readByte()
	local alphaDepth = descriptor & 0x0F
	local rightToLeft = (descriptor >> 4) & 0x01
	local topToBottom = (descriptor >> 5) & 0x01

	assert(imgType == 2)
	assert(depth == 24 or depth == 32)
	assert(idLength == 0)
	assert(colorMapType == 0)

	local bpp = depth / 8

	local img = image.new(width, height)

	local rstart,rend,rdir = 0, height-1, 1
	if topToBottom ~= 1 then
		rstart,rend,rdir = rend,rstart,-1
	end

	local cstart,cend,cdir = 0, width-1, 1
	if rightToLeft == 1 then
		cstart,cend,cdir = cend,cstart,-1
	end

	for row = rstart,rend,rdir do
		for col = cstart,cend,cdir do
		 	local pixel =
				readByte() << 24 |
				readByte() << 16 |
				readByte() <<  8 |
				(bpp == 4 and readByte() or 255)

			img.setPixel(row,col,pixel)
		end
	end
	fp:close()

	return img
end

local function toFile(path, img)
	write(path, img.width, img.height, img.data)
end

return
{
	fromFile = fromFile,
	toFile = toFile
}
