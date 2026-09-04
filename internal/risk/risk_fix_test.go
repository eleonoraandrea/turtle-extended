package risk

import "testing"

// First pyramid add must trigger at step×ATR×units (0.5 ATR with units=1),
// not at step×ATR×(units+1) which delays the first add to 1.0 ATR.
func TestCanPyramidFirstAddAtStepATR(t *testing.T) {
	// entry 100, ATR 2, step 0.5 → first add threshold = 100 + 0.5×2×1 = 101
	if !CanPyramid(100, 101, 2, 1, 1, 4, 0.5) {
		t.Fatal("first add must trigger at 0.5×ATR move (units=1)")
	}
	// short side
	if !CanPyramid(100, 99, 2, -1, 1, 4, 0.5) {
		t.Fatal("first short add must trigger at 0.5×ATR move")
	}
	// below threshold: no add yet
	if CanPyramid(100, 100.5, 2, 1, 1, 4, 0.5) {
		t.Fatal("no add below 0.5×ATR threshold")
	}
	// cap respected
	if CanPyramid(100, 120, 2, 1, 4, 4, 0.5) {
		t.Fatal("units >= max must not add")
	}
	// no ATR → no add
	if CanPyramid(100, 120, 0, 1, 1, 4, 0.5) {
		t.Fatal("ATR<=0 must not add")
	}
}
