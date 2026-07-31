package mutator

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"

	"github.com/codingconcepts/mutant"
)

func parseMutations(t *testing.T, m mutant.Mutator, src string) []mutant.Mutation {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing source: %v", err)
	}

	return m.Mutate(fset, file, "test.go", []byte(src))
}

func astString(fset *token.FileSet, file *ast.File) string {
	var buf strings.Builder
	printer.Fprint(&buf, fset, file)

	return buf.String()
}

func parseFile(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing source: %v", err)
	}

	return fset, file
}

func TestArithmetic(t *testing.T) {
	m := NewArithmetic()
	if m.Name() != "arithmetic" {
		t.Errorf("Name() = %q, want arithmetic", m.Name())
	}

	t.Run("add_to_sub", func(t *testing.T) {
		src := `package p; func f() int { return 1 + 2 }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if mutations[0].Description != "replaced + with -" {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("mul_to_quo", func(t *testing.T) {
		src := `package p; func f() int { return 3 * 4 }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if mutations[0].Description != "replaced * with /" {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("rem_to_mul", func(t *testing.T) {
		src := `package p; func f() int { return 5 % 3 }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if mutations[0].Description != "replaced % with *" {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("skips_string_concat", func(t *testing.T) {
		src := `package p; func f() string { return "a" + "b" }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for string concat, want 0", len(mutations))
		}
	})

	t.Run("skips_string_builtin", func(t *testing.T) {
		src := `package p; func f() string { return string([]byte{}) + "x" }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for string builtin, want 0", len(mutations))
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

func f() int { return 1 + 2 }
`
		fset, file := parseFile(t, src)

		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if original == mutated {
			t.Error("Apply() did not change the AST")
		}

		if !strings.Contains(mutated, "1 - 2") {
			t.Errorf("mutated AST should contain '1 - 2', got: %s", mutated)
		}

		mutations[0].Revert()

		reverted := astString(fset, file)
		if original != reverted {
			t.Errorf("Revert() did not restore AST.\nOriginal:\n%s\nReverted:\n%s", original, reverted)
		}
	})

	t.Run("no_match", func(t *testing.T) {
		src := `package p; func f() bool { return 1 == 2 }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for ==, want 0", len(mutations))
		}
	})
}

func TestArithmeticAssignment(t *testing.T) {
	m := NewArithmeticAssignment()
	if m.Name() != "arithmetic_assignment" {
		t.Errorf("Name() = %q", m.Name())
	}

	ops := []struct {
		op  string
		tok string
	}{
		{"+=", "+="},
		{"-=", "-="},
		{"*=", "*="},
		{"/=", "/="},
		{"%=", "%="},
		{"&=", "&="},
		{"|=", "|="},
		{"^=", "^="},
		{"<<=", "<<="},
		{">>=", ">>="},
		{"&^=", "&^="},
	}

	for _, op := range ops {
		t.Run(op.op, func(t *testing.T) {
			src := `package p; func f() { var x int; x ` + op.op + ` 1 }`

			mutations := parseMutations(t, m, src)
			if len(mutations) != 1 {
				t.Fatalf("got %d mutations for %s, want 1", len(mutations), op.op)
			}

			if !strings.Contains(mutations[0].Description, "replaced "+op.tok+" with =") {
				t.Errorf("description = %q", mutations[0].Description)
			}
		})
	}

	t.Run("skips_plain_assign", func(t *testing.T) {
		src := `package p; func f() { x := 1; _ = x }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for :=, want 0", len(mutations))
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

func f() {
	var x int
	x += 1
}
`
		fset, file := parseFile(t, src)

		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "x = 1") {
			t.Errorf("mutated should contain 'x = 1', got: %s", mutated)
		}

		mutations[0].Revert()

		reverted := astString(fset, file)
		if original != reverted {
			t.Errorf("Revert() failed")
		}
	})
}

func TestArithmeticAssignmentInvert(t *testing.T) {
	m := NewArithmeticAssignmentInvert()
	if m.Name() != "arithmetic_assignment_invert" {
		t.Errorf("Name() = %q", m.Name())
	}

	cases := []struct {
		op       string
		expected string
	}{
		{"+=", "-="},
		{"-=", "+="},
		{"*=", "/="},
		{"/=", "*="},
		{"%=", "*="},
	}

	for _, tc := range cases {
		t.Run(tc.op+"_to_"+tc.expected, func(t *testing.T) {
			src := `package p; func f() { var x int; x ` + tc.op + ` 1 }`

			mutations := parseMutations(t, m, src)
			if len(mutations) != 1 {
				t.Fatalf("got %d mutations for %s, want 1", len(mutations), tc.op)
			}

			if !strings.Contains(mutations[0].Description, tc.expected) {
				t.Errorf("description = %q, want to contain %q", mutations[0].Description, tc.expected)
			}
		})
	}
}

func TestBitwise(t *testing.T) {
	m := NewBitwise()
	if m.Name() != "bitwise" {
		t.Errorf("Name() = %q", m.Name())
	}

	cases := []struct {
		op       string
		expected string
	}{
		{"&", "|"},
		{"|", "&"},
		{"^", "&"},
		{"&^", "&"},
		{"<<", ">>"},
		{">>", "<<"},
	}

	for _, tc := range cases {
		t.Run(tc.op+"_to_"+tc.expected, func(t *testing.T) {
			src := `package p; func f() int { return 1 ` + tc.op + ` 2 }`

			mutations := parseMutations(t, m, src)
			if len(mutations) != 1 {
				t.Fatalf("got %d mutations for %s, want 1", len(mutations), tc.op)
			}

			if !strings.Contains(mutations[0].Description, tc.expected) {
				t.Errorf("description = %q, want to contain %q", mutations[0].Description, tc.expected)
			}
		})
	}

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

func f() int { return 1 & 2 }
`
		fset, file := parseFile(t, src)
		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "1 | 2") {
			t.Errorf("mutated should contain '1 | 2', got: %s", mutated)
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})
}

func TestComparison(t *testing.T) {
	m := NewComparison()
	if m.Name() != "comparison" {
		t.Errorf("Name() = %q", m.Name())
	}

	cases := []struct {
		op       string
		expected string
	}{
		{"<", "<="},
		{"<=", "<"},
		{">", ">="},
		{">=", ">"},
	}

	for _, tc := range cases {
		t.Run(tc.op+"_to_"+tc.expected, func(t *testing.T) {
			src := `package p; func f() bool { return 1 ` + tc.op + ` 2 }`

			mutations := parseMutations(t, m, src)
			if len(mutations) != 1 {
				t.Fatalf("got %d mutations for %s, want 1", len(mutations), tc.op)
			}

			if !strings.Contains(mutations[0].Description, tc.expected) {
				t.Errorf("description = %q, want to contain %q", mutations[0].Description, tc.expected)
			}
		})
	}

	t.Run("skips_equality", func(t *testing.T) {
		src := `package p; func f() bool { return 1 == 2 }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for ==, want 0", len(mutations))
		}
	})
}

func TestComparisonInvert(t *testing.T) {
	m := NewComparisonInvert()
	if m.Name() != "comparison_invert" {
		t.Errorf("Name() = %q", m.Name())
	}

	cases := []struct {
		op       string
		expected string
	}{
		{">", "<="},
		{"<=", ">"},
		{"<", ">="},
		{">=", "<"},
		{"==", "!="},
		{"!=", "=="},
	}

	for _, tc := range cases {
		t.Run(tc.op+"_to_"+tc.expected, func(t *testing.T) {
			src := `package p; func f() bool { return 1 ` + tc.op + ` 2 }`

			mutations := parseMutations(t, m, src)
			if len(mutations) != 1 {
				t.Fatalf("got %d mutations for %s, want 1", len(mutations), tc.op)
			}

			if !strings.Contains(mutations[0].Description, tc.expected) {
				t.Errorf("description = %q, want to contain %q", mutations[0].Description, tc.expected)
			}
		})
	}
}

func TestComparisonReplace(t *testing.T) {
	m := NewComparisonReplace()
	if m.Name() != "comparison_replace" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("land_produces_two_mutations", func(t *testing.T) {
		src := `package p; func f() bool { return 1 == 1 && 2 == 2 }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 2 {
			t.Fatalf("got %d mutations for &&, want 2", len(mutations))
		}

		hasLeft := false
		hasRight := false

		for _, mut := range mutations {
			if strings.Contains(mut.Description, "left") {
				hasLeft = true
			}

			if strings.Contains(mut.Description, "right") {
				hasRight = true
			}

			if !strings.Contains(mut.Description, "true") {
				t.Errorf("&& mutation should mention 'true': %q", mut.Description)
			}
		}

		if !hasLeft || !hasRight {
			t.Errorf("expected left and right mutations, got left=%v right=%v", hasLeft, hasRight)
		}
	})

	t.Run("lor_produces_two_mutations", func(t *testing.T) {
		src := `package p; func f() bool { return 1 == 1 || 2 == 2 }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 2 {
			t.Fatalf("got %d mutations for ||, want 2", len(mutations))
		}

		for _, mut := range mutations {
			if !strings.Contains(mut.Description, "false") {
				t.Errorf("|| mutation should mention 'false': %q", mut.Description)
			}
		}
	})

	t.Run("apply_revert_land", func(t *testing.T) {
		src := `package p

func f() bool { return 1 == 1 && 2 == 2 }
`
		fset, file := parseFile(t, src)

		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		if len(mutations) < 1 {
			t.Fatal("no mutations")
		}

		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if original == mutated {
			t.Error("Apply() did not change AST")
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})

	t.Run("skips_already_true", func(t *testing.T) {
		src := `package p; func f() bool { return true && true }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for 'true && true', want 0", len(mutations))
		}
	})
}

func TestFloatDecrement(t *testing.T) {
	m := NewFloatDecrement()
	if m.Name() != "float_decrement" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("basic", func(t *testing.T) {
		src := `package p; var x = 3.14`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if !strings.Contains(mutations[0].Description, "2.14") {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

var x = 3.14
`
		fset, file := parseFile(t, src)
		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "2.14") {
			t.Errorf("mutated should contain 2.14, got: %s", mutated)
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})

	t.Run("skips_int", func(t *testing.T) {
		src := `package p; var x = 42`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for int, want 0", len(mutations))
		}
	})
}

func TestFloatIncrement(t *testing.T) {
	m := NewFloatIncrement()
	if m.Name() != "float_increment" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("basic", func(t *testing.T) {
		src := `package p; var x = 3.14`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if !strings.Contains(mutations[0].Description, "4.14") {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

var x = 3.14
`
		fset, file := parseFile(t, src)
		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "4.14") {
			t.Errorf("mutated should contain 4.14, got: %s", mutated)
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})
}

func TestIntegerDecrement(t *testing.T) {
	m := NewIntegerDecrement()
	if m.Name() != "integer_decrement" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("basic", func(t *testing.T) {
		src := `package p; var x = 42`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if !strings.Contains(mutations[0].Description, "41") {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

var x = 42
`
		fset, file := parseFile(t, src)
		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "41") {
			t.Errorf("mutated should contain 41, got: %s", mutated)
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})

	t.Run("skips_float", func(t *testing.T) {
		src := `package p; var x = 3.14`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for float, want 0", len(mutations))
		}
	})
}

func TestIntegerIncrement(t *testing.T) {
	m := NewIntegerIncrement()
	if m.Name() != "integer_increment" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("basic", func(t *testing.T) {
		src := `package p; var x = 42`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if !strings.Contains(mutations[0].Description, "43") {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

var x = 42
`
		fset, file := parseFile(t, src)
		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		original := astString(fset, file)

		mutations[0].Apply()

		if astString(fset, file) == original {
			t.Error("Apply() did not change AST")
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})
}

func TestLoopBreak(t *testing.T) {
	m := NewLoopBreak()
	if m.Name() != "loop_break" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("break_to_continue", func(t *testing.T) {
		src := `package p; func f() { for { break } }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if mutations[0].Description != "replaced break with continue" {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("continue_to_break", func(t *testing.T) {
		src := `package p; func f() { for { continue } }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if mutations[0].Description != "replaced continue with break" {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

func f() {
	for {
		break
	}
}
`
		fset, file := parseFile(t, src)
		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "continue") {
			t.Errorf("mutated should contain 'continue', got: %s", mutated)
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})

	t.Run("skips_return", func(t *testing.T) {
		src := `package p; func f() { return }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for return, want 0", len(mutations))
		}
	})
}

func TestLoopCondition(t *testing.T) {
	m := NewLoopCondition()
	if m.Name() != "loop_condition" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("for_with_condition", func(t *testing.T) {
		src := `package p; func f() { i := 0; for i < 10 { i++ } }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if mutations[0].Description != "replaced loop condition with false" {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

func f() {
	i := 0
	for i < 10 {
		i++
	}
}
`
		fset, file := parseFile(t, src)
		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "0 != 0") {
			t.Errorf("mutated should contain '0 != 0', got: %s", mutated)
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})

	t.Run("skips_infinite_loop", func(t *testing.T) {
		src := `package p; func f() { for { break } }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for infinite loop, want 0", len(mutations))
		}
	})

	t.Run("skips_range", func(t *testing.T) {
		src := `package p; func f() { for range []int{} { } }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for range, want 0", len(mutations))
		}
	})
}

func TestRangeBreak(t *testing.T) {
	m := NewRangeBreak()
	if m.Name() != "range_break" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("basic", func(t *testing.T) {
		src := `package p; func f() { for _, v := range []int{1} { _ = v } }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if mutations[0].Description != "added early break to range loop" {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

func f() {
	for _, v := range []int{1, 2, 3} {
		_ = v
	}
}
`
		fset, file := parseFile(t, src)
		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "break") {
			t.Errorf("mutated should contain 'break', got: %s", mutated)
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})

	t.Run("skips_for_loop", func(t *testing.T) {
		src := `package p; func f() { for i := 0; i < 10; i++ { } }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for for loop, want 0", len(mutations))
		}
	})
}

func TestCancelNil(t *testing.T) {
	m := NewCancelNil()
	if m.Name() != "cancel_nil" {
		t.Errorf("Name() = %q", m.Name())
	}

	t.Run("replaces_cancel_arg_with_nil", func(t *testing.T) {
		src := `package p

import "context"

func f() {
	ctx, cancel := context.WithCancelCause(context.Background())
	_ = ctx
	cancel(someErr)
}
`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		if mutations[0].Description != "replaced cancel cause argument with nil" {
			t.Errorf("description = %q", mutations[0].Description)
		}
	})

	t.Run("skips_already_nil", func(t *testing.T) {
		src := `package p

import "context"

func f() {
	ctx, cancel := context.WithCancelCause(context.Background())
	_ = ctx
	cancel(nil)
}
`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations for cancel(nil), want 0", len(mutations))
		}
	})

	t.Run("apply_revert", func(t *testing.T) {
		src := `package p

import "context"

func f() {
	ctx, cancel := context.WithCancelCause(context.Background())
	_ = ctx
	cancel(someErr)
}
`
		fset, file := parseFile(t, src)

		mutations := m.Mutate(fset, file, "test.go", []byte(src))
		if len(mutations) != 1 {
			t.Fatalf("got %d mutations, want 1", len(mutations))
		}

		original := astString(fset, file)

		mutations[0].Apply()

		mutated := astString(fset, file)
		if !strings.Contains(mutated, "cancel(nil)") {
			t.Errorf("mutated should contain 'cancel(nil)', got: %s", mutated)
		}

		mutations[0].Revert()

		if astString(fset, file) != original {
			t.Error("Revert() failed")
		}
	})

	t.Run("no_cancel_cause", func(t *testing.T) {
		src := `package p; func f() { cancel(someErr) }`

		mutations := parseMutations(t, m, src)
		if len(mutations) != 0 {
			t.Errorf("got %d mutations without WithCancelCause, want 0", len(mutations))
		}
	})
}

func TestRegistryHasAll15Viruses(t *testing.T) {
	expected := []string{
		"arithmetic",
		"arithmetic_assignment",
		"arithmetic_assignment_invert",
		"bitwise",
		"comparison",
		"comparison_invert",
		"comparison_replace",
		"float_decrement",
		"float_increment",
		"integer_decrement",
		"integer_increment",
		"loop_break",
		"loop_condition",
		"range_break",
		"cancel_nil",
	}

	if len(Registry) != len(expected) {
		t.Fatalf("Registry has %d viruses, want %d", len(Registry), len(expected))
	}

	for i, m := range Registry {
		if m.Name() != expected[i] {
			t.Errorf("Registry[%d].Name() = %q, want %q", i, m.Name(), expected[i])
		}
	}
}

func TestByName(t *testing.T) {
	t.Run("empty_returns_all", func(t *testing.T) {
		result := ByName(nil)
		if len(result) != 15 {
			t.Errorf("ByName(nil) returned %d, want 15", len(result))
		}
	})

	t.Run("filters_by_name", func(t *testing.T) {
		result := ByName([]string{"arithmetic", "bitwise"})
		if len(result) != 2 {
			t.Fatalf("got %d, want 2", len(result))
		}

		if result[0].Name() != "arithmetic" || result[1].Name() != "bitwise" {
			t.Errorf("got %q and %q", result[0].Name(), result[1].Name())
		}
	})

	t.Run("case_insensitive", func(t *testing.T) {
		result := ByName([]string{"arithmetic"})
		if len(result) != 1 {
			t.Fatalf("got %d, want 1", len(result))
		}

		if result[0].Name() != "arithmetic" {
			t.Errorf("got %q", result[0].Name())
		}
	})

	t.Run("no_match", func(t *testing.T) {
		result := ByName([]string{"NonExistent"})
		if len(result) != 0 {
			t.Errorf("got %d, want 0", len(result))
		}
	})
}
