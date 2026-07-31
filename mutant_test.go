package mutant

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectMutations(t *testing.T) {
	src := `package testpkg

func Add(a, b int) int {
	return a + b
}

func IsPositive(n int) bool {
	return n > 0
}
`
	tmp := t.TempDir()

	path := filepath.Join(tmp, "code.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	arith := &testMutator{
		name: "arithmetic",
		mutateFunc: func(fset *token.FileSet, file *ast.File, filePath string, original []byte) []Mutation {
			var out []Mutation

			ast.Inspect(file, func(n ast.Node) bool {
				expr, ok := n.(*ast.BinaryExpr)
				if !ok {
					return true
				}

				if expr.Op == token.ADD {
					pos := fset.Position(expr.OpPos)
					originalOp := expr.Op

					out = append(out, Mutation{
						File:        filePath,
						Line:        pos.Line,
						Mutator:     "arithmetic",
						Description: "replaced + with -",
						Apply:       func() { expr.Op = token.SUB },
						Revert:      func() { expr.Op = originalOp },
					})
				}

				return true
			})

			return out
		},
	}

	comp := &testMutator{
		name: "comparison",
		mutateFunc: func(fset *token.FileSet, file *ast.File, filePath string, original []byte) []Mutation {
			var out []Mutation

			ast.Inspect(file, func(n ast.Node) bool {
				expr, ok := n.(*ast.BinaryExpr)
				if !ok {
					return true
				}

				if expr.Op == token.GTR {
					pos := fset.Position(expr.OpPos)
					originalOp := expr.Op

					out = append(out, Mutation{
						File:        filePath,
						Line:        pos.Line,
						Mutator:     "comparison",
						Description: "replaced > with >=",
						Apply:       func() { expr.Op = token.GEQ },
						Revert:      func() { expr.Op = originalOp },
					})
				}

				return true
			})

			return out
		},
	}

	mutations, err := CollectMutations([]string{path}, tmp, []Mutator{arith, comp})
	if err != nil {
		t.Fatal(err)
	}

	if len(mutations) != 2 {
		t.Fatalf("got %d mutations, want 2", len(mutations))
	}

	m1 := mutations[0]
	if m1.Mutator != "arithmetic" {
		t.Errorf("mutations[0].Mutator = %q", m1.Mutator)
	}

	if m1.Line != 4 {
		t.Errorf("mutations[0].Line = %d, want 4", m1.Line)
	}

	if m1.File != path {
		t.Errorf("mutations[0].File = %q, want %q", m1.File, path)
	}

	if m1.RelFile != "code.go" {
		t.Errorf("mutations[0].RelFile = %q, want code.go", m1.RelFile)
	}

	if m1.Original == nil {
		t.Error("mutations[0].Original is nil")
	}

	if m1.FileSet == nil {
		t.Error("mutations[0].FileSet is nil")
	}

	if m1.ASTFile == nil {
		t.Error("mutations[0].ASTFile is nil")
	}

	m2 := mutations[1]
	if m2.Mutator != "comparison" {
		t.Errorf("mutations[1].Mutator = %q", m2.Mutator)
	}

	if m2.Line != 8 {
		t.Errorf("mutations[1].Line = %d, want 8", m2.Line)
	}
}

func TestWriteMutatedToTemp(t *testing.T) {
	src := `package testpkg

func Add(a, b int) int {
	return a + b
}
`
	tmp := t.TempDir()
	path := filepath.Join(tmp, "code.go")

	originalBytes := []byte(src)
	if err := os.WriteFile(path, originalBytes, 0644); err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, originalBytes, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	var targetExpr *ast.BinaryExpr

	ast.Inspect(file, func(n ast.Node) bool {
		if expr, ok := n.(*ast.BinaryExpr); ok && expr.Op == token.ADD {
			targetExpr = expr
			return false
		}

		return true
	})

	if targetExpr == nil {
		t.Fatal("could not find + expression")
	}

	m := Mutation{
		File:     path,
		RelFile:  "code.go",
		Line:     4,
		Mutator:  "arithmetic",
		Apply:    func() { targetExpr.Op = token.SUB },
		Revert:   func() { targetExpr.Op = token.ADD },
		FileSet:  fset,
		ASTFile:  file,
		Original: originalBytes,
	}

	m.Apply()
	defer m.Revert()

	tmpPath, err := writeMutatedToTemp(m)
	if err != nil {
		t.Fatalf("writeMutatedToTemp: %v", err)
	}
	defer os.Remove(tmpPath)

	mutatedBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(mutatedBytes, originalBytes) {
		t.Error("mutated file should differ from original")
	}

	if !containsSubstring(string(mutatedBytes), "-") {
		t.Error("mutated file should contain '-' operator")
	}

	// Original file must be untouched
	diskBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(diskBytes, originalBytes) {
		t.Error("original file on disk should not be modified")
	}
}

func TestWriteMutatedToTempFormatsSource(t *testing.T) {
	src := `package testpkg

func Add(a, b int) int {
	return a + b
}
`
	tmp := t.TempDir()

	path := filepath.Join(tmp, "code.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, []byte(src), parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	m := Mutation{
		File:     path,
		FileSet:  fset,
		ASTFile:  file,
		Original: []byte(src),
	}

	tmpPath, err := writeMutatedToTemp(m)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpPath)

	written, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}

	formatted, err := format.Source(written)
	if err != nil {
		t.Fatalf("written file is not valid Go: %v", err)
	}

	if !bytes.Equal(written, formatted) {
		t.Error("written file is not gofmt-formatted")
	}
}

func TestWriteOverlay(t *testing.T) {
	tmpPath, err := writeOverlay("/original/path.go", "/replacement/path.go")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpPath)

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"Replace":{"/original/path.go":"/replacement/path.go"}}`
	if string(data) != want {
		t.Errorf("overlay content = %s, want %s", data, want)
	}
}

func TestStatusString(t *testing.T) {
	cases := []struct {
		status Status
		want   string
	}{
		{Killed, "KILLED"},
		{Survived, "SURVIVED"},
		{Uncovered, "UNCOVERED"},
		{Errored, "ERROR"},
		{Status(99), "UNKNOWN"},
	}
	for _, tc := range cases {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestStatusMarshalJSON(t *testing.T) {
	cases := []struct {
		status Status
		want   string
	}{
		{Killed, `"killed"`},
		{Survived, `"survived"`},
		{Uncovered, `"uncovered"`},
		{Errored, `"error"`},
	}
	for _, tc := range cases {
		got, err := tc.status.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != tc.want {
			t.Errorf("Status(%d).MarshalJSON() = %s, want %s", tc.status, got, tc.want)
		}
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}

type testMutator struct {
	name       string
	mutateFunc func(fset *token.FileSet, file *ast.File, filePath string, original []byte) []Mutation
}

func (m *testMutator) Name() string { return m.name }
func (m *testMutator) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []Mutation {
	return m.mutateFunc(fset, file, filePath, original)
}

// renderAST renders an AST file back to source for comparison.
func renderAST(t *testing.T, fset *token.FileSet, file *ast.File) string {
	t.Helper()

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		t.Fatalf("rendering AST: %v", err)
	}

	return buf.String()
}
