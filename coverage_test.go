package mutant

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCoverProfile(t *testing.T) {
	content := `mode: set
github.com/example/pkg/calc.go:12.24,14.2 1 1
github.com/example/pkg/calc.go:16.32,20.2 3 0
github.com/example/pkg/calc.go:22.14,25.2 2 1
`
	tmp := t.TempDir()

	path := filepath.Join(tmp, "cover.out")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	blocks, err := parseCoverProfile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}

	b := blocks[0]
	if b.file != "github.com/example/pkg/calc.go" {
		t.Errorf("block[0].file = %q", b.file)
	}

	if b.startLine != 12 || b.endLine != 14 {
		t.Errorf("block[0] lines = %d-%d, want 12-14", b.startLine, b.endLine)
	}

	if b.startCol != 24 || b.endCol != 2 {
		t.Errorf("block[0] cols = %d-%d, want 24-2", b.startCol, b.endCol)
	}

	if b.numStmt != 1 {
		t.Errorf("block[0].numStmt = %d, want 1", b.numStmt)
	}

	if b.count != 1 {
		t.Errorf("block[0].count = %d, want 1", b.count)
	}

	if blocks[1].count != 0 {
		t.Errorf("block[1].count = %d, want 0", blocks[1].count)
	}

	if blocks[2].startLine != 22 || blocks[2].endLine != 25 {
		t.Errorf("block[2] lines = %d-%d, want 22-25", blocks[2].startLine, blocks[2].endLine)
	}
}

func TestParseCoverProfileEmpty(t *testing.T) {
	content := `mode: set
`
	tmp := t.TempDir()

	path := filepath.Join(tmp, "cover.out")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	blocks, err := parseCoverProfile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(blocks) != 0 {
		t.Errorf("got %d blocks, want 0", len(blocks))
	}
}

func TestCoverageMapTestsForLine(t *testing.T) {
	cm := &CoverageMap{
		lineToTests: map[string]map[int][]TestRef{
			"calc.go": {
				10: {
					{Name: "TestAdd", Package: "./pkg"},
					{Name: "TestSub", Package: "./pkg"},
				},
				15: {
					{Name: "TestMul", Package: "./pkg"},
				},
			},
			"util.go": {
				5: {
					{Name: "TestHelper", Package: "./internal"},
				},
			},
		},
	}

	t.Run("covered_line_returns_tests", func(t *testing.T) {
		tests := cm.TestsForLine("calc.go", 10)
		if len(tests) != 2 {
			t.Fatalf("got %d tests, want 2", len(tests))
		}

		if tests[0].Name != "TestAdd" {
			t.Errorf("tests[0].Name = %q", tests[0].Name)
		}

		if tests[1].Name != "TestSub" {
			t.Errorf("tests[1].Name = %q", tests[1].Name)
		}
	})

	t.Run("single_test_line", func(t *testing.T) {
		tests := cm.TestsForLine("calc.go", 15)
		if len(tests) != 1 {
			t.Fatalf("got %d tests, want 1", len(tests))
		}

		if tests[0].Name != "TestMul" {
			t.Errorf("tests[0].Name = %q", tests[0].Name)
		}
	})

	t.Run("uncovered_line_returns_nil", func(t *testing.T) {
		tests := cm.TestsForLine("calc.go", 99)
		if tests != nil {
			t.Errorf("got %v, want nil", tests)
		}
	})

	t.Run("unknown_file_returns_nil", func(t *testing.T) {
		tests := cm.TestsForLine("unknown.go", 10)
		if tests != nil {
			t.Errorf("got %v, want nil", tests)
		}
	})

	t.Run("different_file", func(t *testing.T) {
		tests := cm.TestsForLine("util.go", 5)
		if len(tests) != 1 {
			t.Fatalf("got %d tests, want 1", len(tests))
		}

		if tests[0].Package != "./internal" {
			t.Errorf("package = %q, want ./internal", tests[0].Package)
		}
	})
}

func TestImportPathToFilePath(t *testing.T) {
	cases := []struct {
		name            string
		importQualified string
		modPath         string
		modDir          string
		baseDir         string
		want            string
	}{
		{
			name:            "strips_module_prefix",
			importQualified: "github.com/example/mymod/pkg/calc/calc.go",
			modPath:         "github.com/example/mymod",
			modDir:          "/home/user/mymod",
			baseDir:         "/home/user/mymod",
			want:            "pkg/calc/calc.go",
		},
		{
			name:            "no_module_path",
			importQualified: "some/path/file.go",
			modPath:         "",
			modDir:          "",
			baseDir:         "/home/user",
			want:            "some/path/file.go",
		},
		{
			name:            "no_prefix_match",
			importQualified: "other/module/file.go",
			modPath:         "github.com/example/mymod",
			modDir:          "/home/user/mymod",
			baseDir:         "/home/user/mymod",
			want:            "other/module/file.go",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := importPathToFilePath(tc.importQualified, tc.modPath, tc.modDir, tc.baseDir)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGroupTestsByPackage(t *testing.T) {
	tests := []TestRef{
		{Name: "TestA", Package: "./pkg1"},
		{Name: "TestB", Package: "./pkg1"},
		{Name: "TestC", Package: "./pkg2"},
	}

	result := groupTestsByPackage(tests)
	if len(result) != 2 {
		t.Fatalf("got %d packages, want 2", len(result))
	}

	pkg1 := result["./pkg1"]
	if len(pkg1) != 2 {
		t.Errorf("pkg1 has %d tests, want 2", len(pkg1))
	}

	pkg2 := result["./pkg2"]
	if len(pkg2) != 1 {
		t.Errorf("pkg2 has %d tests, want 1", len(pkg2))
	}

	if pkg2[0] != "TestC" {
		t.Errorf("pkg2[0] = %q, want TestC", pkg2[0])
	}
}

func TestCoverageMapEntries(t *testing.T) {
	cm := &CoverageMap{
		lineToTests: map[string]map[int][]TestRef{
			"calc.go": {
				4:  {{Name: "TestAdd", Package: "./pkg"}},
				5:  {{Name: "TestAdd", Package: "./pkg"}},
				6:  {{Name: "TestAdd", Package: "./pkg"}},
				10: {{Name: "TestMul", Package: "./pkg"}},
			},
			"util.go": {
				1: {{Name: "TestHelper", Package: "./internal"}},
			},
		},
	}

	entries := cm.Entries()

	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3 (calc.go:4-6, calc.go:10, util.go:1)", len(entries))
	}

	// Sorted by file then line
	if entries[0].File != "calc.go" || entries[0].StartLine != 4 || entries[0].EndLine != 6 {
		t.Errorf("entries[0] = %+v, want calc.go:4-6", entries[0])
	}

	if len(entries[0].Tests) != 1 || entries[0].Tests[0] != "TestAdd" {
		t.Errorf("entries[0].Tests = %v", entries[0].Tests)
	}

	if entries[1].File != "calc.go" || entries[1].StartLine != 10 {
		t.Errorf("entries[1] = %+v, want calc.go:10", entries[1])
	}

	if entries[2].File != "util.go" || entries[2].StartLine != 1 {
		t.Errorf("entries[2] = %+v, want util.go:1", entries[2])
	}
}

func TestCoverageMapEntriesEmpty(t *testing.T) {
	cm := &CoverageMap{
		lineToTests: make(map[string]map[int][]TestRef),
	}

	entries := cm.Entries()
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}
