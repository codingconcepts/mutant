package example

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("Add(2,3) should be 5")
	}
}

func TestSub(t *testing.T) {
	if Sub(5, 3) != 2 {
		t.Error("Sub(5,3) should be 2")
	}
}

func TestMul(t *testing.T) {
	if Mul(4, 3) != 12 {
		t.Error("Mul(4,3) should be 12")
	}
}

func TestDiv(t *testing.T) {
	if Div(6, 2) != 3 {
		t.Error("Div(6,2) should be 3")
	}
}

// Weak test: only checks result is positive.
// Mod mutation (% -> *) survives: Mod(7,3)=1 > 0, and 7*3=21 > 0.
func TestMod_Weak(t *testing.T) {
	if Mod(7, 3) <= 0 {
		t.Error("Mod(7,3) should be positive")
	}
}

func TestClamp(t *testing.T) {
	if Clamp(5, 0, 10) != 5 {
		t.Error("in-range value should pass through")
	}

	if Clamp(-1, 0, 10) != 0 {
		t.Error("below-range should clamp to lo")
	}

	if Clamp(99, 0, 10) != 10 {
		t.Error("above-range should clamp to hi")
	}
}

func TestIsPositive(t *testing.T) {
	if !IsPositive(1) {
		t.Error("1 is positive")
	}

	if IsPositive(-1) {
		t.Error("-1 is not positive")
	}

	if IsPositive(0) {
		t.Error("0 is not positive")
	}
}

func TestEqual(t *testing.T) {
	if !Equal(5, 5) {
		t.Error("5 == 5")
	}

	if Equal(5, 6) {
		t.Error("5 != 6")
	}
}

func TestCircleArea(t *testing.T) {
	got := CircleArea(1.0)
	if got < 3.14 || got > 3.15 {
		t.Errorf("CircleArea(1.0) = %f, want ~3.14159", got)
	}
}

func TestOffset(t *testing.T) {
	if Offset(5) != 15 {
		t.Error("Offset(5) should be 15")
	}
}

// Weak test: only tests with equal inputs.
// Max(3,3): > -> >= mutation doesn't change result (>=3 is true,
// returns a=3; same as original).
//
// comparison and comparison_invert mutations on > survive here.
func TestMax_Weak(t *testing.T) {
	if Max(3, 3) != 3 {
		t.Error("Max(3,3) should be 3")
	}
}

// Weak test: Double(0)=0 with both * and / (0*2=0, 0/2=0).
// arithmetic mutation * -> / survives.
func TestDouble_Weak(t *testing.T) {
	if Double(0) != 0 {
		t.Error("Double(0) should be 0")
	}
}
