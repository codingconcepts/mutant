package example

import "testing"

func TestCanVote(t *testing.T) {
	if !CanVote(18, true) {
		t.Error("18yo citizen can vote")
	}

	if CanVote(17, true) {
		t.Error("17yo citizen cannot vote")
	}

	if CanVote(18, false) {
		t.Error("18yo non-citizen cannot vote")
	}
}

// Weak test: only checks true cases.
// Replacing right operand with false: weekend||false = weekend, so
// IsWeekendOrHoliday(true, true) still returns true — mutation survives.
func TestIsWeekendOrHoliday_Weak(t *testing.T) {
	if !IsWeekendOrHoliday(true, false) {
		t.Error("weekend should be day off")
	}

	if !IsWeekendOrHoliday(true, true) {
		t.Error("weekend+holiday should be day off")
	}
}
