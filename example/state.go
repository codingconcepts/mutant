package example

func Accumulate(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}

	return total
}

func ScaleDown(v, factor int) int {
	v /= factor
	return v
}

func BitwiseAccum(v, mask uint) uint {
	v &= mask
	v |= 0x01
	v ^= 0xFF
	v <<= 1
	v >>= 1
	v &^= 0x80

	return v
}
