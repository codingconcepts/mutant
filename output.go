package mutant

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type checkedWriter struct {
	w   io.Writer
	err error
}

func (cw *checkedWriter) printf(format string, a ...any) {
	if cw.err != nil {
		return
	}

	_, cw.err = fmt.Fprintf(cw.w, format, a...)
}

func (cw *checkedWriter) println() {
	if cw.err != nil {
		return
	}

	_, cw.err = fmt.Fprintln(cw.w)
}

func PrintTable(w io.Writer, results []MutationResult, verbose bool) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	cw := &checkedWriter{w: tw}
	cw.printf("FILE\tLINE\tMUTATOR\tDESCRIPTION\tSTATUS\tTESTS RUN\n")

	var killed, survived, uncovered, errored int

	for i := range results {
		r := &results[i]

		testNames := strings.Join(r.TestsRun, ", ")
		if len(r.TestsRun) == 0 {
			testNames = "(none)"
		}

		cw.printf("%s\t%d\t%s\t%s\t%s\t%s\n",
			r.Mutation.RelFile, r.Mutation.Line,
			r.Mutation.Mutator, r.Mutation.Description,
			r.Status, testNames)

		switch r.Status {
		case Killed:
			killed++
		case Survived:
			survived++

			if verbose && r.TestOutput != "" {
				cw.printf("\t\t\t--- test output ---\t\t\n")

				for line := range strings.SplitSeq(r.TestOutput, "\n") {
					cw.printf("\t\t\t%s\t\t\n", line)
				}
			}
		case Uncovered:
			uncovered++
		case Errored:
			errored++
		}
	}

	if cw.err != nil {
		return cw.err
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	total := len(results)
	testable := total - uncovered

	var score float64
	if testable > 0 {
		score = float64(killed) / float64(testable) * 100
	}

	cw.w = w
	cw.printf("\nScore: %d/%d mutations killed (%.2f%%)\n", killed, testable, score)

	if survived > 0 {
		cw.printf("Survived: %d mutations were not caught by tests\n", survived)
	}

	if uncovered > 0 {
		cw.printf("Uncovered: %d mutations had no test coverage\n", uncovered)
	}

	if errored > 0 {
		cw.printf("Errors: %d mutations encountered execution errors\n", errored)
	}

	if cw.err != nil {
		return cw.err
	}

	printSurvivors(cw, results, verbose)

	return cw.err
}

func printSurvivors(cw *checkedWriter, results []MutationResult, verbose bool) {
	var survivors []MutationResult

	for i := range results {
		if results[i].Status == Survived {
			survivors = append(survivors, results[i])
		}
	}

	if len(survivors) == 0 {
		return
	}

	cw.printf("\n══════════════════════════════════════\n")
	cw.printf(" SURVIVING MUTATIONS (%d)\n", len(survivors))
	cw.printf("══════════════════════════════════════\n\n")

	for i := range survivors {
		r := &survivors[i]
		cw.printf("  %d. %s:%d\n", i+1, r.Mutation.RelFile, r.Mutation.Line)
		cw.printf("     Virus:  %s\n", r.Mutation.Mutator)
		cw.printf("     Change: %s\n", r.Mutation.Description)

		if len(r.TestsRun) > 0 {
			cw.printf("     Tests:  %s\n", strings.Join(r.TestsRun, ", "))
		}

		if verbose && r.TestOutput != "" {
			cw.printf("     Output:\n")

			for line := range strings.SplitSeq(r.TestOutput, "\n") {
				cw.printf("       %s\n", line)
			}
		}

		cw.println()
	}
}

func WriteSurvivors(path string, results []MutationResult) (retErr error) {
	var survivors []MutationResult

	for i := range results {
		if results[i].Status == Survived {
			survivors = append(survivors, results[i])
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	if strings.HasSuffix(path, ".json") {
		return writeSurvivorsJSON(f, survivors)
	}

	return writeSurvivorsText(f, survivors)
}

func writeSurvivorsJSON(w io.Writer, survivors []MutationResult) error {
	out := make([]jsonMutation, len(survivors))
	for i := range survivors {
		r := &survivors[i]
		out[i] = jsonMutation{
			File:        r.Mutation.RelFile,
			Line:        r.Mutation.Line,
			Mutator:     r.Mutation.Mutator,
			Description: r.Mutation.Description,
			Status:      r.Status,
			TestsRun:    r.TestsRun,
			DurationMs:  r.Duration.Milliseconds(),
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out)
}

func writeSurvivorsText(w io.Writer, survivors []MutationResult) error {
	cw := &checkedWriter{w: w}

	for i := range survivors {
		r := &survivors[i]
		cw.printf("%d. %s:%d\n", i+1, r.Mutation.RelFile, r.Mutation.Line)
		cw.printf("   Virus:  %s\n", r.Mutation.Mutator)
		cw.printf("   Change: %s\n", r.Mutation.Description)

		if len(r.TestsRun) > 0 {
			cw.printf("   Tests:  %s\n", strings.Join(r.TestsRun, ", "))
		}

		cw.println()
	}

	return cw.err
}

type jsonOutput struct {
	Mutations          []jsonMutation `json:"mutations"`
	SurvivingMutations []jsonMutation `json:"surviving_mutations"`
	Summary            jsonSummary    `json:"summary"`
}

type jsonMutation struct {
	File        string   `json:"file"`
	Mutator     string   `json:"mutator"`
	Description string   `json:"description"`
	TestsRun    []string `json:"tests_run"`
	Status      Status   `json:"status"`
	Line        int      `json:"line"`
	DurationMs  int64    `json:"duration_ms"`
}

type jsonSummary struct {
	Total     int     `json:"total"`
	Killed    int     `json:"killed"`
	Survived  int     `json:"survived"`
	Uncovered int     `json:"uncovered"`
	Errors    int     `json:"errors"`
	Score     float64 `json:"score"`
	DurationS float64 `json:"duration_s"`
}

func PrintJSON(w io.Writer, results []MutationResult, totalDuration time.Duration) error {
	out := jsonOutput{}

	for i := range results {
		r := &results[i]
		jm := jsonMutation{
			File:        r.Mutation.RelFile,
			Line:        r.Mutation.Line,
			Mutator:     r.Mutation.Mutator,
			Description: r.Mutation.Description,
			Status:      r.Status,
			TestsRun:    r.TestsRun,
			DurationMs:  r.Duration.Milliseconds(),
		}
		out.Mutations = append(out.Mutations, jm)

		switch r.Status {
		case Killed:
			out.Summary.Killed++
		case Survived:
			out.Summary.Survived++
			out.SurvivingMutations = append(out.SurvivingMutations, jm)
		case Uncovered:
			out.Summary.Uncovered++
		case Errored:
			out.Summary.Errors++
		}
	}

	out.Summary.Total = len(results)

	testable := out.Summary.Total - out.Summary.Uncovered
	if testable > 0 {
		out.Summary.Score = float64(out.Summary.Killed) / float64(testable)
	}

	out.Summary.DurationS = totalDuration.Seconds()

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out)
}

// Coverage output

type jsonCoverageEntry struct {
	File      string   `json:"file"`
	Tests     []string `json:"tests"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
}

func PrintCoverageTable(w io.Writer, entries []CoverageEntry) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	cw := &checkedWriter{w: tw}
	cw.printf("FILE\tLINES\tTESTS\n")

	for _, e := range entries {
		lines := fmt.Sprintf("%d-%d", e.StartLine, e.EndLine)
		if e.StartLine == e.EndLine {
			lines = strconv.Itoa(e.StartLine)
		}

		cw.printf("%s\t%s\t%s\n", e.File, lines, strings.Join(e.Tests, ", "))
	}

	if cw.err != nil {
		return cw.err
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	cw.w = w
	cw.printf("\n%d coverage entries\n", len(entries))

	return cw.err
}

func PrintCoverageJSON(w io.Writer, entries []CoverageEntry) error {
	out := make([]jsonCoverageEntry, len(entries))
	for i, e := range entries {
		out[i] = jsonCoverageEntry(e)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out)
}

// Plan output

type PlanOutput struct {
	MutationsByVirus  map[string]int `json:"mutations_by_virus"`
	TotalMutations    int            `json:"total_mutations"`
	TotalTests        int            `json:"total_tests"`
	CoveredMutations  int            `json:"covered_mutations"`
	AvgTestsPerMutant float64        `json:"avg_tests_per_mutant"`
	EstimatedDuration time.Duration  `json:"-"`
	EstimatedSeconds  float64        `json:"estimated_seconds"`
}

func PrintPlanTable(w io.Writer, p PlanOutput) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	cw := &checkedWriter{w: tw}
	cw.printf("VIRUS\tMUTATIONS\n")

	viruses := make([]string, 0, len(p.MutationsByVirus))
	for v := range p.MutationsByVirus {
		viruses = append(viruses, v)
	}

	sort.Strings(viruses)

	for _, v := range viruses {
		cw.printf("%s\t%d\n", v, p.MutationsByVirus[v])
	}

	if cw.err != nil {
		return cw.err
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	cw.w = w
	cw.printf("\nTotal mutations:    %d\n", p.TotalMutations)
	cw.printf("Covered mutations:  %d\n", p.CoveredMutations)
	cw.printf("Total tests:        %d\n", p.TotalTests)
	cw.printf("Avg tests/mutation: %.1f\n", p.AvgTestsPerMutant)
	cw.printf("Estimated duration: %v\n", p.EstimatedDuration.Round(time.Second))

	return cw.err
}

func PrintPlanJSON(w io.Writer, p PlanOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(p)
}

// Viruses output

func PrintVirusesTable(w io.Writer, names []string) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	cw := &checkedWriter{w: tw}
	cw.printf("VIRUS\n")

	for _, n := range names {
		cw.printf("%s\n", n)
	}

	if cw.err != nil {
		return cw.err
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	cw.w = w
	cw.printf("\n%d viruses available\n", len(names))

	return cw.err
}

func PrintVirusesJSON(w io.Writer, names []string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(names)
}
