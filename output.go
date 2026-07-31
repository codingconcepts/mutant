package mutant

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

func PrintTable(w io.Writer, results []MutationResult, verbose bool) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "FILE\tLINE\tMUTATOR\tDESCRIPTION\tSTATUS\tTESTS RUN\n")

	var killed, survived, uncovered, errored int

	for _, r := range results {
		testNames := strings.Join(r.TestsRun, ", ")
		if len(r.TestsRun) == 0 {
			testNames = "(none)"
		}

		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n",
			r.Mutation.RelFile, r.Mutation.Line,
			r.Mutation.Mutator, r.Mutation.Description,
			r.Status, testNames)

		switch r.Status {
		case Killed:
			killed++
		case Survived:
			survived++

			if verbose && r.TestOutput != "" {
				fmt.Fprintf(tw, "\t\t\t--- test output ---\t\t\n")

				for line := range strings.SplitSeq(r.TestOutput, "\n") {
					fmt.Fprintf(tw, "\t\t\t%s\t\t\n", line)
				}
			}
		case Uncovered:
			uncovered++
		case Errored:
			errored++
		}
	}

	tw.Flush()

	total := len(results)
	testable := total - uncovered

	var score float64
	if testable > 0 {
		score = float64(killed) / float64(testable) * 100
	}

	fmt.Fprintf(w, "\nScore: %d/%d mutations killed (%.2f%%)\n", killed, testable, score)

	if survived > 0 {
		fmt.Fprintf(w, "Survived: %d mutations were not caught by tests\n", survived)
	}

	if uncovered > 0 {
		fmt.Fprintf(w, "Uncovered: %d mutations had no test coverage\n", uncovered)
	}

	if errored > 0 {
		fmt.Fprintf(w, "Errors: %d mutations encountered execution errors\n", errored)
	}

	printSurvivors(w, results, verbose)
}

func printSurvivors(w io.Writer, results []MutationResult, verbose bool) {
	var survivors []MutationResult
	for _, r := range results {
		if r.Status == Survived {
			survivors = append(survivors, r)
		}
	}

	if len(survivors) == 0 {
		return
	}

	fmt.Fprintf(w, "\n══════════════════════════════════════\n")
	fmt.Fprintf(w, " SURVIVING MUTATIONS (%d)\n", len(survivors))
	fmt.Fprintf(w, "══════════════════════════════════════\n\n")

	for i, r := range survivors {
		fmt.Fprintf(w, "  %d. %s:%d\n", i+1, r.Mutation.RelFile, r.Mutation.Line)
		fmt.Fprintf(w, "     Virus:  %s\n", r.Mutation.Mutator)
		fmt.Fprintf(w, "     Change: %s\n", r.Mutation.Description)
		if len(r.TestsRun) > 0 {
			fmt.Fprintf(w, "     Tests:  %s\n", strings.Join(r.TestsRun, ", "))
		}

		if verbose && r.TestOutput != "" {
			fmt.Fprintf(w, "     Output:\n")
			for line := range strings.SplitSeq(r.TestOutput, "\n") {
				fmt.Fprintf(w, "       %s\n", line)
			}
		}

		fmt.Fprintln(w)
	}
}

func WriteSurvivors(path string, results []MutationResult) error {
	var survivors []MutationResult
	for _, r := range results {
		if r.Status == Survived {
			survivors = append(survivors, r)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if strings.HasSuffix(path, ".json") {
		return writeSurvivorsJSON(f, survivors)
	}

	return writeSurvivorsText(f, survivors)
}

func writeSurvivorsJSON(w io.Writer, survivors []MutationResult) error {
	out := make([]jsonMutation, len(survivors))
	for i, r := range survivors {
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
	for i, r := range survivors {
		fmt.Fprintf(w, "%d. %s:%d\n", i+1, r.Mutation.RelFile, r.Mutation.Line)
		fmt.Fprintf(w, "   Virus:  %s\n", r.Mutation.Mutator)
		fmt.Fprintf(w, "   Change: %s\n", r.Mutation.Description)
		if len(r.TestsRun) > 0 {
			fmt.Fprintf(w, "   Tests:  %s\n", strings.Join(r.TestsRun, ", "))
		}
		fmt.Fprintln(w)
	}

	return nil
}

type jsonOutput struct {
	Mutations          []jsonMutation `json:"mutations"`
	SurvivingMutations []jsonMutation `json:"surviving_mutations"`
	Summary            jsonSummary    `json:"summary"`
}

type jsonMutation struct {
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Mutator     string   `json:"mutator"`
	Description string   `json:"description"`
	Status      Status   `json:"status"`
	TestsRun    []string `json:"tests_run"`
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

func PrintJSON(w io.Writer, results []MutationResult, totalDuration time.Duration) {
	out := jsonOutput{}

	for _, r := range results {
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
	enc.Encode(out)
}

// Coverage output

type jsonCoverageEntry struct {
	File      string   `json:"file"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Tests     []string `json:"tests"`
}

func PrintCoverageTable(w io.Writer, entries []CoverageEntry) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "FILE\tLINES\tTESTS\n")

	for _, e := range entries {
		lines := fmt.Sprintf("%d-%d", e.StartLine, e.EndLine)
		if e.StartLine == e.EndLine {
			lines = fmt.Sprintf("%d", e.StartLine)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\n", e.File, lines, strings.Join(e.Tests, ", "))
	}

	tw.Flush()
	fmt.Fprintf(w, "\n%d coverage entries\n", len(entries))
}

func PrintCoverageJSON(w io.Writer, entries []CoverageEntry) {
	out := make([]jsonCoverageEntry, len(entries))
	for i, e := range entries {
		out[i] = jsonCoverageEntry{
			File:      e.File,
			StartLine: e.StartLine,
			EndLine:   e.EndLine,
			Tests:     e.Tests,
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// Plan output

type PlanOutput struct {
	MutationsByVirus   map[string]int `json:"mutations_by_virus"`
	TotalMutations     int            `json:"total_mutations"`
	TotalTests         int            `json:"total_tests"`
	CoveredMutations   int            `json:"covered_mutations"`
	AvgTestsPerMutant  float64        `json:"avg_tests_per_mutant"`
	EstimatedDuration  time.Duration  `json:"-"`
	EstimatedSeconds   float64        `json:"estimated_seconds"`
}

func PrintPlanTable(w io.Writer, p PlanOutput) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "VIRUS\tMUTATIONS\n")

	viruses := make([]string, 0, len(p.MutationsByVirus))
	for v := range p.MutationsByVirus {
		viruses = append(viruses, v)
	}

	sort.Strings(viruses)

	for _, v := range viruses {
		fmt.Fprintf(tw, "%s\t%d\n", v, p.MutationsByVirus[v])
	}

	tw.Flush()

	fmt.Fprintf(w, "\nTotal mutations:    %d\n", p.TotalMutations)
	fmt.Fprintf(w, "Covered mutations:  %d\n", p.CoveredMutations)
	fmt.Fprintf(w, "Total tests:        %d\n", p.TotalTests)
	fmt.Fprintf(w, "Avg tests/mutation: %.1f\n", p.AvgTestsPerMutant)
	fmt.Fprintf(w, "Estimated duration: %v\n", p.EstimatedDuration.Round(time.Second))
}

func PrintPlanJSON(w io.Writer, p PlanOutput) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(p)
}

// Viruses output

func PrintVirusesTable(w io.Writer, names []string) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "VIRUS\n")

	for _, n := range names {
		fmt.Fprintf(tw, "%s\n", n)
	}

	tw.Flush()
	fmt.Fprintf(w, "\n%d viruses available\n", len(names))
}

func PrintVirusesJSON(w io.Writer, names []string) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(names)
}
