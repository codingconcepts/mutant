package example

func FindFirst(items []int, pred func(int) bool) (int, bool) {
	for _, v := range items {
		if pred(v) {
			return v, true
		}
	}

	return 0, false
}

func SkipNegatives(items []int) []int {
	var out []int

	for _, v := range items {
		if v < 0 {
			continue
		}

		out = append(out, v)
	}

	return out
}

func CountUp(n int) int {
	count := 0
	for range n {
		count++
	}

	return count
}

func Sum(items []int) int {
	total := 0
	for _, v := range items {
		total += v
	}

	return total
}
