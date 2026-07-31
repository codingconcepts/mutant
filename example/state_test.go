package example

import "testing"

func TestAccumulate(t *testing.T) {
	if Accumulate([]int{1, 2, 3}) != 6 {
		t.Error("Accumulate([1,2,3]) should be 6")
	}
}

// Weak test: only checks result < input.
// /=→= mutation: ScaleDown(100,10) returns 10 (with /=) or 10 (with =).
// Wait, v=100, v/=10 → v=10. v=100, v=10 → v=10. Same result!
// Use different values: ScaleDown(100,5)=20 with /=, =5 with =.
// Test only checks result < input, so 5 < 100 passes → mutation survives.
func TestScaleDown_Weak(t *testing.T) {
	got := ScaleDown(100, 5)
	if got >= 100 {
		t.Errorf("ScaleDown(100,5) = %d, should be < 100", got)
	}
}

func TestBitwiseAccum(t *testing.T) {
	got := BitwiseAccum(0xFF, 0x0F)
	if got != 0x70 {
		t.Errorf("BitwiseAccum(0xFF, 0x0F) = 0x%02X, want 0x70", got)
	}
}
