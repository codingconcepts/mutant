package example

func SetBit(v, pos uint) uint   { return v | (1 << pos) }
func ClearBit(v, pos uint) uint { return v &^ (1 << pos) }
func HasBit(v, pos uint) bool   { return v&(1<<pos) != 0 }
func FlipBit(v, pos uint) uint  { return v ^ (1 << pos) }
func ShiftLeft(v, n uint) uint  { return v << n }
func ShiftRight(v, n uint) uint { return v >> n }
