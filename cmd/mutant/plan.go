package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/codingconcepts/mutant"
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
	planCmd.Flags().IntP("workers", "w", 0, "parallel workers for estimation (0 = NumCPU/2)")
	planCmd.Flags().Bool("fast-coverage", false, "use package-level coverage (faster build, runs more tests per mutation)")
	planCmd.Flags().Bool("no-cache", false, "force rebuild of coverage map, ignoring cache")
	planCmd.Flags().Bool("diff", false, "filter mutations to changed lines only (staged changes by default)")
	planCmd.Flags().Bool("unstaged", false, "diff unstaged changes instead of staged; implies --diff")
	planCmd.Flags().String("diff-ref", "", "git ref to diff against (e.g. HEAD~3, main); implies --diff")
	rootCmd.AddCommand(planCmd)
}

// runPlan is the handler for `mutant plan`. It performs a dry run: discovers
// tests, builds coverage, collects mutations, and estimates duration without
// actually running any mutations.
func runPlan(cmd *cobra.Command, args []string) error {
	f, err := parseCommonFlags(cmd, args)
	if err != nil {
		return err
	}

	mode, err := getFlag(cmd.Flags().GetString, "mode")
	if err != nil {
		return err
	}

	if mode != "text" && mode != "json" {
		return fmt.Errorf("--mode must be 'text' or 'json', got %q", mode)
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	ctx := context.Background()

	slog.Info("discovering tests")

	tests, err := mutant.DiscoverTests(ctx, dir, f.packages)
	if err != nil {
		return fmt.Errorf("discovering tests: %w", err)
	}

	slog.Info("found tests", "count", len(tests))

	if len(tests) == 0 {
		return fmt.Errorf("no tests found in %v", f.packages)
	}

	slog.Info("building coverage map")

	coverResult, err := loadOrBuildCoverage(ctx, dir, tests, f)
	if err != nil {
		return err
	}

	slog.Info("coverage map complete", "duration", coverResult.Duration.Round(time.Millisecond))

	slog.Info("discovering source files")

	files, err := mutant.DiscoverSourceFiles(ctx, dir, f.packages)
	if err != nil {
		return fmt.Errorf("discovering source files: %w", err)
	}

	slog.Info("collecting mutations")

	mutations, err := mutant.CollectMutations(files, dir, f.mutators)
	if err != nil {
		return fmt.Errorf("collecting mutations: %w", err)
	}

	if f.diffSpec != nil {
		changedLines, diffErr := mutant.ParseGitDiff(ctx, dir, *f.diffSpec)
		if diffErr != nil {
			return fmt.Errorf("parsing git diff: %w", diffErr)
		}

		before := len(mutations)
		mutations = mutant.FilterMutationsByDiff(mutations, changedLines)
		slog.Info("diff filter applied", "before", before, "after", len(mutations))
	}

	plan := buildPlanOutput(mutations, tests, coverResult, f.workers)

	if mode == "json" {
		return mutant.PrintPlanJSON(os.Stdout, plan)
	}

	return mutant.PrintPlanTable(os.Stdout, plan)
}

func loadOrBuildCoverage(ctx context.Context, dir string, tests []mutant.TestRef, f commonFlags) (*mutant.CoverageResult, error) {
	var (
		cacheKey string
		keyErr   error
	)

	if !f.noCache {
		cacheKey, keyErr = mutant.ComputeCacheKey(dir)
		if keyErr == nil {
			if cached, ok := mutant.LoadCoverageCache(dir, cacheKey); ok {
				slog.Info("⚡ coverage map loaded from cache (use --no-cache to rebuild)")
				return cached, nil
			}
		}
	}

	buildCoverage := mutant.BuildCoverageMap
	if f.fastCoverage {
		buildCoverage = mutant.BuildCoarseCoverageMap
	}

	coverResult, err := buildCoverage(ctx, dir, tests, f.timeout)
	if err != nil {
		return nil, fmt.Errorf("building coverage map: %w", err)
	}

	if !f.noCache && keyErr == nil {
		if saveErr := mutant.SaveCoverageCache(dir, cacheKey, coverResult); saveErr != nil {
			slog.Warn("failed to save coverage cache", "error", saveErr)
		}
	}

	return coverResult, nil
}

// buildPlanOutput computes plan statistics from the collected mutations and
// coverage map: mutation counts per virus, coverage ratio, and estimated
// duration based on per-test timing and worker parallelism.
func buildPlanOutput(mutations []mutant.Mutation, tests []mutant.TestRef, coverResult *mutant.CoverageResult, workers int) mutant.PlanOutput {
	byVirus := make(map[string]int)
	covered := 0
	totalTestRuns := 0
	fileCosts := make(map[string]time.Duration)

	for i := range mutations {
		m := &mutations[i]
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
		workers = max(1, runtime.NumCPU()/2)
	}

	mutationEstimate := scheduleEstimate(fileCosts, workers)
	// 2x multiplier accounts for overhead: AST manipulation, file I/O,
	// process startup, and overlay setup that aren't captured in raw test time.
	mutationEstimate *= 2
	estimated := coverResult.Duration + mutationEstimate

	return mutant.PlanOutput{
		MutationsByVirus:  byVirus,
		TotalMutations:    len(mutations),
		TotalTests:        len(tests),
		CoveredMutations:  covered,
		AvgTestsPerMutant: avgTests,
		EstimatedDuration: estimated,
		EstimatedSeconds:  estimated.Seconds(),
	}
}

// scheduleEstimate simulates a greedy scheduling of file costs across worker
// buckets (largest-first) and returns the max bucket — the estimated
// wall-clock time for the mutation phase.
func scheduleEstimate(fileCosts map[string]time.Duration, workers int) time.Duration {
	costs := make([]time.Duration, 0, len(fileCosts))
	for _, c := range fileCosts {
		costs = append(costs, c)
	}

	slices.SortFunc(costs, func(a, b time.Duration) int {
		return cmp.Compare(b, a)
	})

	buckets := make([]time.Duration, workers)

	for _, c := range costs {
		minIdx := 0
		for i := 1; i < len(buckets); i++ {
			if buckets[i] < buckets[minIdx] {
				minIdx = i
			}
		}

		buckets[minIdx] += c
	}

	return slices.Max(buckets)
}
