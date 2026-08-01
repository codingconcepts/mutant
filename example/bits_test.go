package example

import "testing"

func TestSetBit(t *testing.T) {
	if SetBit(0, 2) != 4 {
		t.Error("SetBit(0,2) should be 4")
	}
}

func TestClearBit(t *testing.T) {
	if ClearBit(7, 1) != 5 {
		t.Error("ClearBit(7,1) should be 5")
	}
}

func TestHasBit(t *testing.T) {
	if !HasBit(4, 2) {
		t.Error("4 has bit 2 set")
	}

	if HasBit(4, 0) {
		t.Error("4 does not have bit 0 set")
	}
}

func TestFlipBit(t *testing.T) {
	if FlipBit(0, 3) != 8 {
		t.Error("FlipBit(0,3) should be 8")
	}

	if FlipBit(8, 3) != 0 {
		t.Error("FlipBit(8,3) should be 0")
	}
}

// Weak test: only checks result > input.
// ShiftLeft(4,1)=8 > 4 passes. But ShiftRight(4,1)=2 > 4 fails,
// so << -> >> mutation IS caught here. Let's use a value where it survives:
// ShiftLeft(1,0)=1 > 0 is checked, ShiftRight(1,0)=1 > 0 also passes.
func TestShiftLeft_Weak(t *testing.T) {
	if ShiftLeft(1, 0) < 1 {
		t.Error("ShiftLeft(1,0) should be >= 1")
	}
}

func TestShiftRight(t *testing.T) {
	if ShiftRight(8, 2) != 2 {
		t.Error("ShiftRight(8,2) should be 2")
	}
}
