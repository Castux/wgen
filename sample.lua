local function nearest(img, u, v)
	local x = math.floor(u * (img.height-1))
	local y = math.floor(v * (img.width-1))
	return img.getRGBA(x,y)
end



return
{
	nearest = nearest
}
