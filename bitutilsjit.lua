local bit = require "bit"

local band, bor = bit.band, bit.bor
local ls, rs = bit.lshift, bit.rshift

return
{
	composeInt = function(a,b,c,d)
		return bor(ls(a, 24), ls(b, 16), ls(c, 8), d)
	end,

	decomposeInt = function(int)
		return
			band(rs(int, 24), 0xff),
			band(rs(int, 16), 0xff),
			band(rs(int,  8), 0xff),
			band(   int     , 0xff)
	end,

	decomposeShort = function(int)
		return
			band(rs(int, 8), 0xff),
			band(   int    , 0xff)
	end,

	shiftMask = function(i, shift, mask)
		return band(rs(i, shift), mask)
	end
}
