package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/codingconcepts/mutant"
	"github.com/codingconcepts/mutant/mutator"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [packages]",
	Short: "Show mutation count and estimated duration",
	Long:  "Dry run: discover tests, build coverage map, collect mutations, and estimate how long mutation testing will take without actually running mutations.",
	Args:  cobra.ArbitraryArgs,
	RunE:  runPlan,
}

func init() {
	planCmd.Flags().StringP("viruses", "v", "", "comma-separated virus names to enable (default: all)")
	planCmd.Flags().StringP("mode", "m", "text", "output mode: text or json")
	planCmd.Flags().DurationP("timeout", "t", 10*time.Second, "per-test timeout")
	planCmd.Flags().IntP("workers", "w", 1, "parallel workers for estimation (0 to use all cores)")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	packages := args
	if len(packages) == 0 {
		packages = []string{"./..."}
	}

	virusFlag, _ := cmd.Flags().GetString("viruses")
	mode, _ := cmd.Flags().GetString("mode")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	workers, _ := cmd.Flags().GetInt("workers")

	if mode != "text" && mode != "json" {
		return fmt.Errorf("--mode must be 'text' or 'json', got %q", mode)
	}

	var virusNames []string
	if virusFlag != "" {
		virusNames = strings.Split(virusFlag, ",")
	}

	mutators := mutator.ByName(virusNames)
	if len(mutators) == 0 {
		return fmt.Errorf("no matching viruses found")
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Discovering tests...\n")

	tests, err := mutant.DiscoverTests(ctx, dir, packages)
	if err != nil {
		return fmt.Errorf("discovering tests: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Found %d tests\n", len(tests))

	if len(tests) == 0 {
		return fmt.Errorf("no tests found in %v", packages)
	}

	fmt.Fprintf(os.Stderr, "Building coverage map...\n")

	coverResult, err := mutant.BuildCoverageMap(ctx, dir, tests, timeout)
	if err != nil {
		return fmt.Errorf("building coverage map: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Coverage map complete (%v)\n", coverResult.Duration.Round(time.Millisecond))

	fmt.Fprintf(os.Stderr, "Discovering source files...\n")

	files, err := mutant.DiscoverSourceFiles(ctx, dir, packages)
	if err != nil {
		return fmt.Errorf("discovering source files: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Collecting mutations...\n")

	mutations, err := mutant.CollectMutations(files, dir, mutators)
	if err != nil {
		return fmt.Errorf("collecting mutations: %w", err)
	}

	fmt.Fprintln(os.Stderr)

	byVirus := make(map[string]int)
	covered := 0
	totalTestRuns := 0
	fileCosts := make(map[string]time.Duration)

	for _, m := range mutations {
		byVirus[m.Mutator]++

		refs := coverResult.Map.TestsForLine(m.RelFile, m.Line)
		if len(refs) > 0 {
			covered++
			totalTestRuns += len(refs)
			cost := coverResult.PerTest * time.Duration(len(refs))
			fileCosts[m.RelFile] += cost
		}
	}

	var avgTests float64
	if covered > 0 {
		avgTests = float64(totalTestRuns) / float64(covered)
	}

	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	mutationEstimate := scheduleEstimate(fileCosts, workers)
	// Parallel go test invocations contend on CPU/IO; scale by observed overhead
	mutationEstimate = mutationEstimate * 2
	estimated := coverResult.Duration + mutationEstimate

	plan := mutant.PlanOutput{
		MutationsByVirus:  byVirus,
		TotalMutations:    len(mutations),
		TotalTests:        len(tests),
		CoveredMutations:  covered,
		AvgTestsPerMutant: avgTests,
		EstimatedDuration: estimated,
		EstimatedSeconds:  estimated.Seconds(),
	}

	if mode == "json" {
		mutant.PrintPlanJSON(os.Stdout, plan)
	} else {
		mutant.PrintPlanTable(os.Stdout, plan)
	}

	return nil
}

func scheduleEstimate(fileCosts map[string]time.Duration, workers int) time.Duration {
	costs := make([]time.Duration, 0, len(fileCosts))
	for _, c := range fileCosts {
		costs = append(costs, c)
	}

	sort.Slice(costs, func(i, j int) bool {
		return costs[i] > costs[j]
	})

	buckets := make([]time.Duration, workers)
	for _, c := range costs {
		min := 0
		for i := 1; i < len(buckets); i++ {
			if buckets[i] < buckets[min] {
				min = i
			}
		}
		buckets[min] += c
	}

	var max time.Duration
	for _, b := range buckets {
		if b > max {
			max = b
		}
	}

	return max
}
