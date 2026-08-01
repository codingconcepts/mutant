package main

import (
	"context"
	"fmt"
	"log/slog"
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
	planCmd.Flags().IntP("workers", "w", 0, "parallel workers for estimation (0 = NumCPU/2)")
	planCmd.Flags().Bool("fast-coverage", false, "use package-level coverage (faster build, runs more tests per mutation)")
	planCmd.Flags().Bool("no-cache", false, "force rebuild of coverage map, ignoring cache")
	planCmd.Flags().Bool("diff", false, "filter mutations to changed lines only (staged changes by default)")
	planCmd.Flags().Bool("unstaged", false, "diff unstaged changes instead of staged; implies --diff")
	planCmd.Flags().String("diff-ref", "", "git ref to diff against (e.g. HEAD~3, main); implies --diff")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	packages := args
	if len(packages) == 0 {
		packages = []string{"./..."}
	}

	virusFlag, err := cmd.Flags().GetString("viruses")
	if err != nil {
		return fmt.Errorf("getting viruses flag: %w", err)
	}

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return fmt.Errorf("getting mode flag: %w", err)
	}

	timeout, err := cmd.Flags().GetDuration("timeout")
	if err != nil {
		return fmt.Errorf("getting timeout flag: %w", err)
	}

	workers, err := cmd.Flags().GetInt("workers")
	if err != nil {
		return fmt.Errorf("getting workers flag: %w", err)
	}

	fastCoverage, err := cmd.Flags().GetBool("fast-coverage")
	if err != nil {
		return fmt.Errorf("getting fast-coverage flag: %w", err)
	}

	noCache, err := cmd.Flags().GetBool("no-cache")
	if err != nil {
		return fmt.Errorf("getting no-cache flag: %w", err)
	}

	diffEnabled, err := cmd.Flags().GetBool("diff")
	if err != nil {
		return fmt.Errorf("getting diff flag: %w", err)
	}

	unstaged, err := cmd.Flags().GetBool("unstaged")
	if err != nil {
		return fmt.Errorf("getting unstaged flag: %w", err)
	}

	diffRef, err := cmd.Flags().GetString("diff-ref")
	if err != nil {
		return fmt.Errorf("getting diff-ref flag: %w", err)
	}

	if diffRef != "" || unstaged {
		diffEnabled = true
	}

	var diffSpec *mutant.DiffSpec

	if diffEnabled {
		spec := mutant.DiffSpec{Unstaged: unstaged}
		if diffRef != "" {
			spec.Ref = diffRef
		}

		diffSpec = &spec
	}

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

	slog.Info("discovering tests")

	tests, err := mutant.DiscoverTests(ctx, dir, packages)
	if err != nil {
		return fmt.Errorf("discovering tests: %w", err)
	}

	slog.Info("found tests", "count", len(tests))

	if len(tests) == 0 {
		return fmt.Errorf("no tests found in %v", packages)
	}

	slog.Info("building coverage map")

	var coverResult *mutant.CoverageResult

	if !noCache {
		cacheKey, keyErr := mutant.ComputeCacheKey(dir)
		if keyErr == nil {
			if cached, ok := mutant.LoadCoverageCache(dir, cacheKey); ok {
				coverResult = cached

				slog.Info("⚡ coverage map loaded from cache (use --no-cache to rebuild)")
			}
		}
	}

	if coverResult == nil {
		if fastCoverage {
			coverResult, err = mutant.BuildCoarseCoverageMap(ctx, dir, tests, timeout)
		} else {
			coverResult, err = mutant.BuildCoverageMap(ctx, dir, tests, timeout)
		}

		if err != nil {
			return fmt.Errorf("building coverage map: %w", err)
		}

		if !noCache {
			if cacheKey, keyErr := mutant.ComputeCacheKey(dir); keyErr == nil {
				if saveErr := mutant.SaveCoverageCache(dir, cacheKey, coverResult); saveErr != nil {
					slog.Warn("failed to save coverage cache", "error", saveErr)
				}
			}
		}
	}

	slog.Info("coverage map complete", "duration", coverResult.Duration.Round(time.Millisecond))

	slog.Info("discovering source files")

	files, err := mutant.DiscoverSourceFiles(ctx, dir, packages)
	if err != nil {
		return fmt.Errorf("discovering source files: %w", err)
	}

	slog.Info("collecting mutations")

	mutations, err := mutant.CollectMutations(files, dir, mutators)
	if err != nil {
		return fmt.Errorf("collecting mutations: %w", err)
	}

	if diffSpec != nil {
		changedLines, diffErr := mutant.ParseGitDiff(ctx, dir, *diffSpec)
		if diffErr != nil {
			return fmt.Errorf("parsing git diff: %w", diffErr)
		}

		before := len(mutations)
		mutations = mutant.FilterMutationsByDiff(mutations, changedLines)
		slog.Info("diff filter applied", "before", before, "after", len(mutations))
	}

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
	// Parallel go test invocations contend on CPU/IO; scale by observed overhead
	mutationEstimate *= 2
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
		return mutant.PrintPlanJSON(os.Stdout, plan)
	}

	return mutant.PrintPlanTable(os.Stdout, plan)
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
