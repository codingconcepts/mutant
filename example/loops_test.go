package example

import "testing"

// Weak test: only checks that a value was found, not that it's the FIRST match.
// break→continue mutation still returns the correct value (last match happens
// to equal first when only one matches).
func TestFindFirst_Weak(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}

	v, ok := FindFirst(items, func(n int) bool { return n == 3 })
	if !ok {
		t.Error("should find a match")
	}

	if v != 3 {
		t.Errorf("got %d, want 3", v)
	}
}

func TestSkipNegatives(t *testing.T) {
	got := SkipNegatives([]int{1, -2, 3, -4, 5})

	want := []int{1, 3, 5}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestCountUp(t *testing.T) {
	if CountUp(5) != 5 {
		t.Error("CountUp(5) should be 5")
	}

	if CountUp(0) != 0 {
		t.Error("CountUp(0) should be 0")
	}
}

func TestSum(t *testing.T) {
	if Sum([]int{1, 2, 3}) != 6 {
		t.Error("Sum([1,2,3]) should be 6")
	}

	if Sum([]int{}) != 0 {
		t.Error("Sum([]) should be 0")
	}
}
