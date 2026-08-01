package mutant

import (
	"testing"
)

func TestParseDiffOutput(t *testing.T) {
	input := `diff --git a/calc.go b/calc.go
index 1234567..abcdefg 100644
--- a/calc.go
+++ b/calc.go
@@ -10,3 +10,5 @@ func Add(a, b int) int {
+	// new line
+	return a + b
@@ -20,0 +22,2 @@ func Sub(a, b int) int {
+	x := a
+	return x - b
diff --git a/logic.go b/logic.go
index 1234567..abcdefg 100644
--- a/logic.go
+++ b/logic.go
@@ -5,1 +5,1 @@ func IsValid() bool {
-	return false
+	return true
`

	result, err := parseDiffOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result))
	}

	calcRanges := result["calc.go"]
	if len(calcRanges) != 2 {
		t.Fatalf("expected 2 ranges for calc.go, got %d", len(calcRanges))
	}

	if calcRanges[0].Start != 10 || calcRanges[0].End != 14 {
		t.Errorf("calc.go range 0: got %d-%d, want 10-14", calcRanges[0].Start, calcRanges[0].End)
	}

	if calcRanges[1].Start != 22 || calcRanges[1].End != 23 {
		t.Errorf("calc.go range 1: got %d-%d, want 22-23", calcRanges[1].Start, calcRanges[1].End)
	}

	logicRanges := result["logic.go"]
	if len(logicRanges) != 1 {
		t.Fatalf("expected 1 range for logic.go, got %d", len(logicRanges))
	}

	if logicRanges[0].Start != 5 || logicRanges[0].End != 5 {
		t.Errorf("logic.go range 0: got %d-%d, want 5-5", logicRanges[0].Start, logicRanges[0].End)
	}
}

func TestParseDiffOutput_DeletionOnly(t *testing.T) {
	input := `diff --git a/calc.go b/calc.go
--- a/calc.go
+++ b/calc.go
@@ -10,3 +10,0 @@ func Add(a, b int) int {
-	line1
-	line2
-	line3
`

	result, err := parseDiffOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result["calc.go"]) != 0 {
		t.Errorf("expected 0 ranges for deletion-only hunk, got %d", len(result["calc.go"]))
	}
}

func TestParseDiffOutput_Empty(t *testing.T) {
	result, err := parseDiffOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestFilterMutationsByDiff(t *testing.T) {
	mutations := []Mutation{
		{RelFile: "calc.go", Line: 5},
		{RelFile: "calc.go", Line: 10},
		{RelFile: "calc.go", Line: 12},
		{RelFile: "calc.go", Line: 20},
		{RelFile: "logic.go", Line: 3},
		{RelFile: "other.go", Line: 1},
	}

	changedLines := map[string][]lineRange{
		"calc.go":  {{Start: 10, End: 15}},
		"logic.go": {{Start: 1, End: 5}},
	}

	filtered := FilterMutationsByDiff(mutations, changedLines)

	if len(filtered) != 3 {
		t.Fatalf("expected 3 filtered mutations, got %d", len(filtered))
	}

	want := []struct {
		file string
		line int
	}{
		{"calc.go", 10},
		{"calc.go", 12},
		{"logic.go", 3},
	}

	for i, w := range want {
		if filtered[i].RelFile != w.file || filtered[i].Line != w.line {
			t.Errorf("filtered[%d]: got %s:%d, want %s:%d", i, filtered[i].RelFile, filtered[i].Line, w.file, w.line)
		}
	}
}

func TestFilterMutationsByDiff_NoChanges(t *testing.T) {
	mutations := []Mutation{
		{RelFile: "calc.go", Line: 5},
	}

	filtered := FilterMutationsByDiff(mutations, map[string][]lineRange{})
	if filtered != nil {
		t.Errorf("expected nil, got %v", filtered)
	}
}

func TestChangedPackages(t *testing.T) {
	changedLines := map[string][]lineRange{
		"calc.go":           {{Start: 1, End: 5}},
		"pkg/util/helper.go": {{Start: 10, End: 20}},
	}

	pkgs := ChangedPackages(changedLines)

	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}

	pkgSet := make(map[string]bool)
	for _, p := range pkgs {
		pkgSet[p] = true
	}

	if !pkgSet["./"] {
		t.Error("expected ./ package for root-level file")
	}

	if !pkgSet["./pkg/util"] {
		t.Error("expected ./pkg/util package")
	}
}
