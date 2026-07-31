package example

func Add(a, b int) int { return a + b }
func Sub(a, b int) int { return a - b }
func Mul(a, b int) int { return a * b }
func Div(a, b int) int { return a / b }
func Mod(a, b int) int { return a % b }

func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}

	if v > hi {
		return hi
	}

	return v
}

func IsPositive(n int) bool { return n > 0 }
func Equal(a, b int) bool   { return a == b }

func CircleArea(r float64) float64 {
	return 3.14159 * r * r
}

func Offset(n int) int { return n + 10 }

func Max(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func Double(n int) int { return n * 2 }
