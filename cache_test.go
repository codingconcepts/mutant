package mutant

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeCacheKey_Deterministic(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	key1, err := ComputeCacheKey(dir)
	if err != nil {
		t.Fatal(err)
	}

	key2, err := ComputeCacheKey(dir)
	if err != nil {
		t.Fatal(err)
	}

	if key1 != key2 {
		t.Errorf("cache key not deterministic: %s != %s", key1, key2)
	}
}

func TestComputeCacheKey_ChangesOnFileEdit(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	key1, err := ComputeCacheKey(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nvar x = 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	key2, err := ComputeCacheKey(dir)
	if err != nil {
		t.Fatal(err)
	}

	if key1 == key2 {
		t.Error("cache key should change when file contents change")
	}
}

func TestSaveAndLoadCoverageCache(t *testing.T) {
	dir := t.TempDir()
	key := "test-key-abc123"

	original := &CoverageResult{
		Map: &CoverageMap{
			lineToTests: map[string]map[int][]TestRef{
				"calc.go": {
					10: {{Name: "TestAdd", Package: "./"}},
					11: {{Name: "TestAdd", Package: "./"}, {Name: "TestSub", Package: "./"}},
				},
			},
		},
		Duration: 5 * time.Second,
		PerTest:  500 * time.Millisecond,
	}

	if err := SaveCoverageCache(dir, key, original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, ok := LoadCoverageCache(dir, key)
	if !ok {
		t.Fatal("load returned false")
	}

	refs := loaded.Map.TestsForLine("calc.go", 10)
	if len(refs) != 1 || refs[0].Name != "TestAdd" {
		t.Errorf("unexpected refs for line 10: %v", refs)
	}

	refs = loaded.Map.TestsForLine("calc.go", 11)
	if len(refs) != 2 {
		t.Errorf("expected 2 refs for line 11, got %d", len(refs))
	}

	if loaded.Duration != 5*time.Second {
		t.Errorf("duration: got %v, want 5s", loaded.Duration)
	}

	if loaded.PerTest != 500*time.Millisecond {
		t.Errorf("perTest: got %v, want 500ms", loaded.PerTest)
	}
}

func TestLoadCoverageCache_MissingFile(t *testing.T) {
	dir := t.TempDir()

	_, ok := LoadCoverageCache(dir, "any-key")
	if ok {
		t.Error("expected false for missing cache file")
	}
}

func TestLoadCoverageCache_KeyMismatch(t *testing.T) {
	dir := t.TempDir()

	result := &CoverageResult{
		Map:      &CoverageMap{lineToTests: map[string]map[int][]TestRef{}},
		Duration: time.Second,
		PerTest:  time.Millisecond,
	}

	if err := SaveCoverageCache(dir, "key-a", result); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	_, ok := LoadCoverageCache(dir, "key-b")
	if ok {
		t.Error("expected false for key mismatch")
	}
}

func TestSaveAndLoadResultCache(t *testing.T) {
	dir := t.TempDir()
	key := "test-key-abc123"

	original := map[string]CachedResult{
		"calc.go:10:arithmetic:replaced + with -": {
			Status:     Killed,
			TestsRun:   []string{"TestAdd"},
			TestOutput: "FAIL",
			DurationMs: 500,
		},
		"calc.go:20:comparison:replaced < with <=": {
			Status:     Survived,
			TestsRun:   []string{"TestClamp"},
			TestOutput: "PASS",
			DurationMs: 300,
		},
	}

	if err := SaveResultCache(dir, key, original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, ok := LoadResultCache(dir, key)
	if !ok {
		t.Fatal("load returned false")
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 results, got %d", len(loaded))
	}

	r1 := loaded["calc.go:10:arithmetic:replaced + with -"]
	if r1.Status != Killed {
		t.Errorf("expected Killed, got %v", r1.Status)
	}
	if len(r1.TestsRun) != 1 || r1.TestsRun[0] != "TestAdd" {
		t.Errorf("unexpected TestsRun: %v", r1.TestsRun)
	}
	if r1.DurationMs != 500 {
		t.Errorf("expected 500ms, got %d", r1.DurationMs)
	}

	r2 := loaded["calc.go:20:comparison:replaced < with <="]
	if r2.Status != Survived {
		t.Errorf("expected Survived, got %v", r2.Status)
	}
}

func TestLoadResultCache_MissingFile(t *testing.T) {
	dir := t.TempDir()

	_, ok := LoadResultCache(dir, "any-key")
	if ok {
		t.Error("expected false for missing cache file")
	}
}

func TestLoadResultCache_KeyMismatch(t *testing.T) {
	dir := t.TempDir()

	results := map[string]CachedResult{
		"x.go:1:arithmetic:replaced + with -": {Status: Killed},
	}

	if err := SaveResultCache(dir, "key-a", results); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	_, ok := LoadResultCache(dir, "key-b")
	if ok {
		t.Error("expected false for key mismatch")
	}
}
