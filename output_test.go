package mutant

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPrintTable(t *testing.T) {
	results := []MutationResult{
		{
			Mutation: Mutation{RelFile: "calc.go", Line: 4, Mutator: "arithmetic", Description: "replaced + with -"},
			Status:   Killed,
			TestsRun: []string{"TestAdd"},
			Duration: 100 * time.Millisecond,
		},
		{
			Mutation: Mutation{RelFile: "calc.go", Line: 12, Mutator: "comparison", Description: "replaced > with >="},
			Status:   Survived,
			TestsRun: []string{"TestIsPositive"},
			Duration: 200 * time.Millisecond,
		},
		{
			Mutation: Mutation{RelFile: "util.go", Line: 7, Mutator: "integer_increment", Description: "incremented 0 to 1"},
			Status:   Uncovered,
			Duration: 1 * time.Millisecond,
		},
	}

	var buf bytes.Buffer
	PrintTable(&buf, results, false)
	output := buf.String()

	// Check header
	if !strings.Contains(output, "FILE") || !strings.Contains(output, "MUTATOR") {
		t.Error("output missing header columns")
	}

	// Check mutation rows
	if !strings.Contains(output, "calc.go") {
		t.Error("output missing calc.go")
	}

	if !strings.Contains(output, "KILLED") {
		t.Error("output missing KILLED status")
	}

	if !strings.Contains(output, "SURVIVED") {
		t.Error("output missing SURVIVED status")
	}

	if !strings.Contains(output, "UNCOVERED") {
		t.Error("output missing UNCOVERED status")
	}

	if !strings.Contains(output, "TestAdd") {
		t.Error("output missing test name")
	}

	if !strings.Contains(output, "(none)") {
		t.Error("output missing (none) for uncovered mutation")
	}

	// Check score line
	if !strings.Contains(output, "Score: 1/2 mutations killed (50.00%)") {
		t.Errorf("score line not found in output:\n%s", output)
	}

	if !strings.Contains(output, "Survived: 1") {
		t.Errorf("survived line not found in output:\n%s", output)
	}

	if !strings.Contains(output, "Uncovered: 1") {
		t.Errorf("uncovered line not found in output:\n%s", output)
	}

	if !strings.Contains(output, "SURVIVING MUTATIONS (1)") {
		t.Errorf("surviving mutations section not found in output:\n%s", output)
	}

	if !strings.Contains(output, "calc.go:12") {
		t.Errorf("surviving mutation detail not found in output:\n%s", output)
	}

	if !strings.Contains(output, "replaced > with >=") {
		t.Errorf("surviving mutation description not found in output:\n%s", output)
	}
}

func TestPrintTableVerbose(t *testing.T) {
	results := []MutationResult{
		{
			Mutation:   Mutation{RelFile: "calc.go", Line: 12, Mutator: "comparison", Description: "replaced > with >="},
			Status:     Survived,
			TestsRun:   []string{"TestIsPositive"},
			TestOutput: "PASS: TestIsPositive",
			Duration:   200 * time.Millisecond,
		},
	}

	var buf bytes.Buffer
	PrintTable(&buf, results, true)
	output := buf.String()

	if !strings.Contains(output, "PASS: TestIsPositive") {
		t.Errorf("verbose output missing test output:\n%s", output)
	}
}

func TestPrintTableAllKilled(t *testing.T) {
	results := []MutationResult{
		{
			Mutation: Mutation{RelFile: "a.go", Line: 1, Mutator: "X", Description: "x"},
			Status:   Killed,
			TestsRun: []string{"TestA"},
		},
	}

	var buf bytes.Buffer
	PrintTable(&buf, results, false)
	output := buf.String()

	if !strings.Contains(output, "100.00%") {
		t.Errorf("expected 100%% score:\n%s", output)
	}

	if strings.Contains(output, "Survived:") {
		t.Error("should not show survived line when 0 survived")
	}

	if strings.Contains(output, "SURVIVING MUTATIONS") {
		t.Error("should not show surviving mutations section when 0 survived")
	}
}

func TestPrintTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, nil, false)
	output := buf.String()

	if !strings.Contains(output, "Score: 0/0") {
		t.Errorf("empty results should show 0/0:\n%s", output)
	}
}

func TestPrintJSON(t *testing.T) {
	results := []MutationResult{
		{
			Mutation: Mutation{RelFile: "calc.go", Line: 4, Mutator: "arithmetic", Description: "replaced + with -"},
			Status:   Killed,
			TestsRun: []string{"TestAdd"},
			Duration: 142 * time.Millisecond,
		},
		{
			Mutation: Mutation{RelFile: "calc.go", Line: 12, Mutator: "comparison", Description: "replaced > with >="},
			Status:   Survived,
			TestsRun: []string{"TestIsPositive"},
			Duration: 300 * time.Millisecond,
		},
		{
			Mutation: Mutation{RelFile: "util.go", Line: 7, Mutator: "integer_increment", Description: "incremented 0 to 1"},
			Status:   Uncovered,
			Duration: 1 * time.Millisecond,
		},
	}

	var buf bytes.Buffer
	PrintJSON(&buf, results, 5*time.Second)

	type jsonMut struct {
		File        string   `json:"file"`
		Line        int      `json:"line"`
		Mutator     string   `json:"mutator"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		TestsRun    []string `json:"tests_run"`
		DurationMs  int64    `json:"duration_ms"`
	}

	var out struct {
		Mutations          []jsonMut `json:"mutations"`
		SurvivingMutations []jsonMut `json:"surviving_mutations"`
		Summary            struct {
			Total     int     `json:"total"`
			Killed    int     `json:"killed"`
			Survived  int     `json:"survived"`
			Uncovered int     `json:"uncovered"`
			Errors    int     `json:"errors"`
			Score     float64 `json:"score"`
			DurationS float64 `json:"duration_s"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	if len(out.Mutations) != 3 {
		t.Fatalf("got %d mutations, want 3", len(out.Mutations))
	}

	m := out.Mutations[0]
	if m.File != "calc.go" {
		t.Errorf("mutations[0].file = %q", m.File)
	}

	if m.Line != 4 {
		t.Errorf("mutations[0].line = %d", m.Line)
	}

	if m.Mutator != "arithmetic" {
		t.Errorf("mutations[0].mutator = %q", m.Mutator)
	}

	if m.Status != "killed" {
		t.Errorf("mutations[0].status = %q, want killed", m.Status)
	}

	if m.DurationMs != 142 {
		t.Errorf("mutations[0].duration_ms = %d, want 142", m.DurationMs)
	}

	s := out.Summary
	if s.Total != 3 {
		t.Errorf("summary.total = %d, want 3", s.Total)
	}

	if s.Killed != 1 {
		t.Errorf("summary.killed = %d, want 1", s.Killed)
	}

	if s.Survived != 1 {
		t.Errorf("summary.survived = %d, want 1", s.Survived)
	}

	if s.Uncovered != 1 {
		t.Errorf("summary.uncovered = %d, want 1", s.Uncovered)
	}

	if s.Score != 0.5 {
		t.Errorf("summary.score = %f, want 0.5", s.Score)
	}

	if s.DurationS != 5.0 {
		t.Errorf("summary.duration_s = %f, want 5.0", s.DurationS)
	}

	if len(out.SurvivingMutations) != 1 {
		t.Fatalf("surviving_mutations: got %d, want 1", len(out.SurvivingMutations))
	}

	sm := out.SurvivingMutations[0]
	if sm.File != "calc.go" || sm.Line != 12 || sm.Mutator != "comparison" {
		t.Errorf("surviving_mutations[0] = %+v", sm)
	}
}

func TestPrintJSONStatusValues(t *testing.T) {
	results := []MutationResult{
		{
			Mutation: Mutation{RelFile: "a.go", Line: 1, Mutator: "X", Description: "x"},
			Status:   Killed,
		},
		{
			Mutation: Mutation{RelFile: "a.go", Line: 2, Mutator: "X", Description: "x"},
			Status:   Survived,
		},
		{
			Mutation: Mutation{RelFile: "a.go", Line: 3, Mutator: "X", Description: "x"},
			Status:   Uncovered,
		},
		{
			Mutation: Mutation{RelFile: "a.go", Line: 4, Mutator: "X", Description: "x"},
			Status:   Errored,
		},
	}

	var buf bytes.Buffer
	PrintJSON(&buf, results, time.Second)

	raw := buf.String()
	if !strings.Contains(raw, `"killed"`) {
		t.Error("JSON missing 'killed' status")
	}

	if !strings.Contains(raw, `"survived"`) {
		t.Error("JSON missing 'survived' status")
	}

	if !strings.Contains(raw, `"uncovered"`) {
		t.Error("JSON missing 'uncovered' status")
	}

	if !strings.Contains(raw, `"error"`) {
		t.Error("JSON missing 'error' status")
	}
}

func TestPrintCoverageTable(t *testing.T) {
	entries := []CoverageEntry{
		{File: "calc.go", StartLine: 4, EndLine: 6, Tests: []string{"TestAdd", "TestSub"}},
		{File: "calc.go", StartLine: 10, EndLine: 10, Tests: []string{"TestMul"}},
	}

	var buf bytes.Buffer
	PrintCoverageTable(&buf, entries)
	output := buf.String()

	if !strings.Contains(output, "FILE") || !strings.Contains(output, "LINES") || !strings.Contains(output, "TESTS") {
		t.Error("missing header columns")
	}

	if !strings.Contains(output, "calc.go") {
		t.Error("missing file name")
	}

	if !strings.Contains(output, "4-6") {
		t.Error("missing line range")
	}

	if !strings.Contains(output, "10") {
		t.Error("missing single line")
	}

	if !strings.Contains(output, "TestAdd, TestSub") {
		t.Error("missing test names")
	}

	if !strings.Contains(output, "2 coverage entries") {
		t.Errorf("missing entry count:\n%s", output)
	}
}

func TestPrintCoverageJSON(t *testing.T) {
	entries := []CoverageEntry{
		{File: "calc.go", StartLine: 4, EndLine: 6, Tests: []string{"TestAdd"}},
	}

	var buf bytes.Buffer
	PrintCoverageJSON(&buf, entries)

	var out []struct {
		File      string   `json:"file"`
		StartLine int      `json:"start_line"`
		EndLine   int      `json:"end_line"`
		Tests     []string `json:"tests"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(out) != 1 {
		t.Fatalf("got %d entries, want 1", len(out))
	}

	if out[0].File != "calc.go" {
		t.Errorf("file = %q", out[0].File)
	}

	if out[0].StartLine != 4 || out[0].EndLine != 6 {
		t.Errorf("lines = %d-%d, want 4-6", out[0].StartLine, out[0].EndLine)
	}
}

func TestPrintPlanTable(t *testing.T) {
	plan := PlanOutput{
		MutationsByVirus:  map[string]int{"arithmetic": 5, "comparison": 3},
		TotalMutations:    8,
		TotalTests:        10,
		CoveredMutations:  6,
		AvgTestsPerMutant:  2.5,
		EstimatedDuration: 30 * time.Second,
		EstimatedSeconds:  30.0,
	}

	var buf bytes.Buffer
	PrintPlanTable(&buf, plan)
	output := buf.String()

	if !strings.Contains(output, "VIRUS") || !strings.Contains(output, "MUTATIONS") {
		t.Error("missing header")
	}

	if !strings.Contains(output, "arithmetic") || !strings.Contains(output, "5") {
		t.Error("missing Arithmetic row")
	}

	if !strings.Contains(output, "comparison") || !strings.Contains(output, "3") {
		t.Error("missing Comparison row")
	}

	if !strings.Contains(output, "Total mutations:    8") {
		t.Errorf("missing total:\n%s", output)
	}

	if !strings.Contains(output, "Covered mutations:  6") {
		t.Errorf("missing covered:\n%s", output)
	}

	if !strings.Contains(output, "Estimated duration:") {
		t.Errorf("missing ETA:\n%s", output)
	}
}

func TestPrintPlanJSON(t *testing.T) {
	plan := PlanOutput{
		MutationsByVirus:  map[string]int{"arithmetic": 5},
		TotalMutations:    5,
		TotalTests:        10,
		CoveredMutations:  4,
		AvgTestsPerMutant:  2.0,
		EstimatedDuration: 20 * time.Second,
		EstimatedSeconds:  20.0,
	}

	var buf bytes.Buffer
	PrintPlanJSON(&buf, plan)

	var out struct {
		MutationsByVirus map[string]int `json:"mutations_by_virus"`
		TotalMutations   int            `json:"total_mutations"`
		TotalTests       int            `json:"total_tests"`
		CoveredMutations int            `json:"covered_mutations"`
		AvgTestsPerMutant float64        `json:"avg_tests_per_mutant"`
		EstimatedSeconds float64        `json:"estimated_seconds"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if out.TotalMutations != 5 {
		t.Errorf("total_mutations = %d", out.TotalMutations)
	}

	if out.CoveredMutations != 4 {
		t.Errorf("covered_mutations = %d", out.CoveredMutations)
	}

	if out.EstimatedSeconds != 20.0 {
		t.Errorf("estimated_seconds = %f", out.EstimatedSeconds)
	}

	if out.MutationsByVirus["arithmetic"] != 5 {
		t.Errorf("mutations_by_virus[Arithmetic] = %d", out.MutationsByVirus["arithmetic"])
	}
}

func TestPrintVirusesTable(t *testing.T) {
	names := []string{"arithmetic", "bitwise", "comparison"}

	var buf bytes.Buffer
	PrintVirusesTable(&buf, names)
	output := buf.String()

	if !strings.Contains(output, "VIRUS") {
		t.Error("missing header")
	}

	for _, n := range names {
		if !strings.Contains(output, n) {
			t.Errorf("missing virus %q", n)
		}
	}

	if !strings.Contains(output, "3 viruses available") {
		t.Errorf("missing count:\n%s", output)
	}
}

func TestPrintVirusesJSON(t *testing.T) {
	names := []string{"arithmetic", "bitwise"}

	var buf bytes.Buffer
	PrintVirusesJSON(&buf, names)

	var out []string
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(out) != 2 {
		t.Fatalf("got %d, want 2", len(out))
	}

	if out[0] != "arithmetic" || out[1] != "bitwise" {
		t.Errorf("got %v", out)
	}
}
