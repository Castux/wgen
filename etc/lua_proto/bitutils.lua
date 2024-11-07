return
{
	composeInt = function(a,b,c,d)
		return a << 24 | b << 16 | c << 8 | d
	end,

	decomposeInt = function(int)
		return
			(int >> 24) & 0xff,
			(int >> 16) & 0xff,
			(int >>  8) & 0xff,
			(int >>  0) & 0xff
	end,

	decomposeShort = function(int)
		return
			(int >>  8) & 0xff,
			(int >>  0) & 0xff
	end,

	shiftMask = function(i, shift, mask)
		return (i >> shift) & mask
	end
}
