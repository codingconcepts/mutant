package mutant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Status represents the outcome of running tests against a single mutation.
type Status int

const (
	// Killed means a test failed, proving it detected the mutation.
	Killed Status = iota
	// Survived means all tests passed despite the mutation - a gap in test coverage.
	Survived
	// Uncovered means no test covers the mutated line, so no tests were run.
	Uncovered
	// Errored means the mutation could not be tested (e.g. compilation failure).
	Errored
)

// String returns the human-readable label for a Status.
func (s Status) String() string {
	switch s {
	case Killed:
		return "KILLED"
	case Survived:
		return "SURVIVED"
	case Uncovered:
		return "UNCOVERED"
	case Errored:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON encodes the status as a lowercase JSON string (e.g. "killed").
func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToLower(s.String()))
}

// Mutation describes a single code mutation that can be applied and reverted.
// Apply modifies the in-memory AST; Revert restores it. Original holds the
// on-disk source so the file can be restored if the process is interrupted.
type Mutation struct {
	File        string         // absolute path to the source file
	RelFile     string         // path relative to the project root (used for display and cache keys)
	Mutator     string         // name of the mutator that produced this mutation (e.g. "arithmetic")
	Description string         // human-readable summary of the change (e.g. "replaced + with -")
	Apply       func()         // applies the mutation to the in-memory AST
	Revert      func()         // reverts the mutation, restoring the original AST
	FileSet     *token.FileSet // file set for position info; populated by CollectMutations
	ASTFile     *ast.File      // parsed AST of the source file; shared across mutations in the same file
	Original    []byte         // original file contents for crash recovery
	Line        int            // source line number of the mutation
}

// MutationResult captures the outcome of testing a single mutation.
type MutationResult struct {
	TestOutput string        // combined stdout/stderr from the test run
	TestsRun   []string      // names of tests that were executed against this mutation
	Mutation   Mutation      // the mutation that was tested
	Status     Status        // outcome: Killed, Survived, Uncovered, or Errored
	Duration   time.Duration // wall-clock time for this mutation's test run
}

// Mutator is the interface that all mutation strategies implement.
// Name returns a unique identifier (e.g. "arithmetic"), and Mutate walks an
// AST to produce zero or more Mutations for a given source file.
type Mutator interface {
	Name() string
	Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []Mutation
}

// Phase identifies which stage of the mutation testing pipeline is active.
// Used by MutationProgress to drive UI updates.
type Phase int

const (
	PhaseDiscoverTests    Phase = iota // listing test functions via go test -list
	PhaseBuildCoverage                 // building the per-test coverage map
	PhaseDiscoverFiles                 // listing source files via go list
	PhaseCollectMutations              // walking ASTs to gather mutations
	PhaseExecute                       // applying mutations and running tests
	PhaseDone                          // all mutations processed
)

// MutationProgress is emitted during a Run to report progress.
// The TUI and default logger both consume these to display status updates.
type MutationProgress struct {
	Result    *MutationResult // non-nil during PhaseExecute when a mutation finishes
	Mutation  *Mutation       // the mutation being reported on
	Message   string          // human-readable status message
	Phase     Phase           // current pipeline stage
	Completed int             // number of mutations processed so far
	Total     int             // total mutations to process
}

// Config controls a mutation testing run.
type Config struct {
	OnProgress   func(MutationProgress) // callback for progress updates; nil uses slog-based default
	Diff         *DiffSpec              // if non-nil, only mutate lines changed in the diff
	Dir          string                 // project root directory
	Packages     []string               // Go package patterns to test (e.g. "./...")
	Mutators     []Mutator              // mutation strategies to apply
	Timeout      time.Duration          // per-test timeout
	Workers      int                    // parallel worker count; 0 = NumCPU/2
	Verbose      bool                   // show test output for survived mutations
	FastCoverage bool                   // use package-level (coarse) coverage instead of per-test
	NoCache      bool                   // skip loading/saving coverage and result caches
}

// activeMutations tracks mutations currently applied to on-disk files, keyed
// by file path. Used by RestoreAllActive to recover original source on
// interrupt (SIGINT/SIGTERM).
var (
	activeMu        sync.Mutex
	activeMutations = make(map[string]*Mutation)
)

// RegisterMutation records a mutation as actively applied to disk, so
// RestoreAllActive can revert it on crash or interrupt.
func RegisterMutation(m *Mutation) {
	activeMu.Lock()
	activeMutations[m.File] = m
	activeMu.Unlock()
}

// UnregisterMutation removes a file from the active mutation tracker after
// the mutation has been reverted.
func UnregisterMutation(file string) {
	activeMu.Lock()
	delete(activeMutations, file)
	activeMu.Unlock()
}

// RestoreAllActive writes original source bytes back to disk for every file
// with an active mutation. Called from signal handlers to prevent leaving
// mutated source on disk after an interrupt.
func RestoreAllActive() {
	activeMu.Lock()
	defer activeMu.Unlock()

	for _, m := range activeMutations {
		if err := os.WriteFile(m.File, m.Original, 0o644); err != nil {
			slog.Error("failed to restore file", "file", m.File, "error", err)
		}
	}
}

// Run is the main entry point for mutation testing. It orchestrates the
// full pipeline: discover tests, build coverage map, discover source files,
// collect mutations, optionally filter by diff, execute mutations against
// their covering tests, and return all results. Returns nil results with no
// error when a diff filter produces zero mutations.
func Run(ctx context.Context, cfg Config) ([]MutationResult, error) {
	progress := cfg.OnProgress
	if progress == nil {
		progress = defaultProgress
	}

	packages, changedLines, err := resolveDiffScope(ctx, cfg, progress)
	if err != nil {
		return nil, err
	}

	if cfg.Diff != nil && changedLines == nil {
		return nil, nil
	}

	progress(MutationProgress{Phase: PhaseDiscoverTests, Message: "Discovering tests..."})

	tests, err := DiscoverTests(ctx, cfg.Dir, packages)
	if err != nil {
		return nil, fmt.Errorf("discovering tests: %w", err)
	}

	progress(MutationProgress{Phase: PhaseDiscoverTests, Message: fmt.Sprintf("Found %d tests", len(tests))})

	if len(tests) == 0 {
		return nil, fmt.Errorf("no tests found in %v", packages)
	}

	progress(MutationProgress{Phase: PhaseBuildCoverage, Message: "Building coverage map..."})

	coverResult, cacheKey, cachedResults, err := loadCaches(ctx, cfg, tests, progress)
	if err != nil {
		return nil, err
	}

	coverMap := coverResult.Map
	progress(MutationProgress{Phase: PhaseBuildCoverage, Message: fmt.Sprintf("Coverage map complete (%v)", coverResult.Duration.Round(time.Millisecond))})

	progress(MutationProgress{Phase: PhaseDiscoverFiles, Message: "Discovering source files..."})

	files, err := DiscoverSourceFiles(ctx, cfg.Dir, packages)
	if err != nil {
		return nil, fmt.Errorf("discovering source files: %w", err)
	}

	progress(MutationProgress{Phase: PhaseDiscoverFiles, Message: fmt.Sprintf("Found %d source files", len(files))})

	progress(MutationProgress{Phase: PhaseCollectMutations, Message: "Collecting mutations..."})

	mutations, err := CollectMutations(files, cfg.Dir, cfg.Mutators)
	if err != nil {
		return nil, fmt.Errorf("collecting mutations: %w", err)
	}

	if changedLines != nil {
		before := len(mutations)
		mutations = FilterMutationsByDiff(mutations, changedLines)
		progress(MutationProgress{Phase: PhaseCollectMutations, Message: fmt.Sprintf("Diff filter: %d -> %d mutations", before, len(mutations))})

		if len(mutations) == 0 {
			progress(MutationProgress{Phase: PhaseDone, Message: "No mutations in changed lines", Completed: 0, Total: 0})
			return nil, nil
		}
	}

	progress(MutationProgress{Phase: PhaseCollectMutations, Message: fmt.Sprintf("Found %d mutations\n", len(mutations))})

	results := executeMutations(ctx, cfg, mutations, coverMap, cachedResults, progress)

	saveResultsToCache(cfg, cacheKey, results)

	progress(MutationProgress{Phase: PhaseDone, Message: "Done", Completed: len(results), Total: len(results)})

	return results, nil
}

func defaultProgress(p MutationProgress) {
	if p.Phase == PhaseExecute && p.Result != nil {
		testNames := strings.Join(p.Result.TestsRun, ", ")
		if len(p.Result.TestsRun) == 0 {
			testNames = "(none)"
		}

		attrs := []any{
			"progress", fmt.Sprintf("%d/%d", p.Completed, p.Total),
			"file", p.Mutation.RelFile, "line", p.Mutation.Line,
			"mutator", p.Mutation.Mutator, "description", p.Mutation.Description,
			"status", p.Result.Status, "tests", testNames,
			"duration", p.Result.Duration.Round(time.Millisecond),
		}
		if p.Result.Status == Killed {
			slog.Debug("mutation result", attrs...)
		} else {
			slog.Info("mutation result", attrs...)
		}
	} else if p.Message != "" {
		slog.Info(p.Message)
	}
}

func resolveDiffScope(ctx context.Context, cfg Config, progress func(MutationProgress)) ([]string, map[string][]lineRange, error) {
	packages := cfg.Packages

	if cfg.Diff == nil {
		return packages, nil, nil
	}

	changedLines, err := ParseGitDiff(ctx, cfg.Dir, *cfg.Diff)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing git diff: %w", err)
	}

	if len(changedLines) == 0 {
		progress(MutationProgress{Phase: PhaseDone, Message: "No changed lines found", Completed: 0, Total: 0})
		return packages, nil, nil
	}

	if hasWildcard(packages) {
		scopedPkgs := ChangedPackages(changedLines)
		if len(scopedPkgs) > 0 {
			packages = scopedPkgs
			progress(MutationProgress{Phase: PhaseDiscoverTests, Message: fmt.Sprintf("Diff scoped to %d package(s)", len(packages))})
		}
	}

	return packages, changedLines, nil
}

func loadCaches(ctx context.Context, cfg Config, tests []TestRef, progress func(MutationProgress)) (*CoverageResult, string, map[string]CachedResult, error) {
	var (
		coverResult   *CoverageResult
		cacheKey      string
		cachedResults map[string]CachedResult
	)

	if !cfg.NoCache {
		if key, keyErr := ComputeCacheKey(cfg.Dir); keyErr == nil {
			cacheKey = key

			if cached, ok := LoadCoverageCache(cfg.Dir, cacheKey); ok {
				coverResult = cached

				progress(MutationProgress{Phase: PhaseBuildCoverage, Message: "Coverage map loaded from cache (use --no-cache to rebuild)"})
			}

			if results, ok := LoadResultCache(cfg.Dir, cacheKey); ok {
				cachedResults = results
			}
		}
	}

	if coverResult == nil {
		var err error
		if cfg.FastCoverage {
			coverResult, err = BuildCoarseCoverageMap(ctx, cfg.Dir, tests, cfg.Timeout)
		} else {
			coverResult, err = BuildCoverageMap(ctx, cfg.Dir, tests, cfg.Timeout)
		}

		if err != nil {
			return nil, "", nil, fmt.Errorf("building coverage map: %w", err)
		}

		if !cfg.NoCache && cacheKey != "" {
			if saveErr := SaveCoverageCache(cfg.Dir, cacheKey, coverResult); saveErr != nil {
				slog.Warn("failed to save coverage cache", "error", saveErr)
			}
		}
	}

	return coverResult, cacheKey, cachedResults, nil
}

// executeMutations runs all mutations against their covering tests, grouped
// by file. Mutations in the same file are serialized (they share one AST),
// while different files run in parallel up to the worker limit.
func executeMutations(ctx context.Context, cfg Config, mutations []Mutation, coverMap *CoverageMap, cachedResults map[string]CachedResult, progress func(MutationProgress)) []MutationResult {
	workers := cfg.Workers
	if workers <= 0 {
		workers = max(1, runtime.NumCPU()/2)
	}

	fileGroups := make(map[string][]int)
	for i := range mutations {
		fileGroups[mutations[i].File] = append(fileGroups[mutations[i].File], i)
	}

	results := make([]MutationResult, len(mutations))

	var completed atomic.Int64

	total := len(mutations)

	sem := make(chan struct{}, workers)

	var wg sync.WaitGroup

	for _, indices := range fileGroups {
		wg.Add(1)

		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			executeFileGroup(ctx, indices, mutations, results, coverMap, cachedResults, cfg, &completed, total, progress)
		}()
	}

	wg.Wait()

	return results
}

// executeFileGroup processes all mutations for a single source file. It
// creates one temp file and one overlay file that are reused across all
// mutations in the group, avoiding repeated file creation overhead.
func executeFileGroup(ctx context.Context, indices []int, mutations []Mutation, results []MutationResult, coverMap *CoverageMap, cachedResults map[string]CachedResult, cfg Config, completed *atomic.Int64, total int, progress func(MutationProgress)) {
	mutatedFile, err := os.CreateTemp("", "mutant-*.go")
	if err != nil {
		failAllMutations(indices, mutations, results, completed, fmt.Sprintf("creating temp file: %v", err))
		return
	}

	mutatedPath := mutatedFile.Name()

	if cerr := mutatedFile.Close(); cerr != nil {
		slog.Debug("closing temp file", "error", cerr)
	}

	defer func() {
		if rerr := os.Remove(mutatedPath); rerr != nil {
			slog.Debug("removing temp file", "error", rerr)
		}
	}()

	overlayPath, err := writeOverlay(mutations[indices[0]].File, mutatedPath)
	if err != nil {
		failAllMutations(indices, mutations, results, completed, fmt.Sprintf("creating overlay file: %v", err))
		return
	}

	defer func() {
		if rerr := os.Remove(overlayPath); rerr != nil {
			slog.Debug("removing overlay file", "error", rerr)
		}
	}()

	for _, idx := range indices {
		if ctx.Err() != nil {
			break
		}

		m := mutations[idx]

		var result MutationResult
		if cr, ok := cachedResults[mutationCacheKey(m)]; ok {
			result = MutationResult{
				Mutation:   m,
				Status:     cr.Status,
				TestsRun:   cr.TestsRun,
				TestOutput: cr.TestOutput,
				Duration:   time.Duration(cr.DurationMs) * time.Millisecond,
			}
		} else {
			result = executeMutationWithFiles(ctx, m, coverMap, cfg, mutatedPath, overlayPath)
		}

		results[idx] = result

		n := int(completed.Add(1))
		progress(MutationProgress{
			Phase:     PhaseExecute,
			Result:    &result,
			Mutation:  &m,
			Completed: n,
			Total:     total,
		})
	}
}

func failAllMutations(indices []int, mutations []Mutation, results []MutationResult, completed *atomic.Int64, msg string) {
	for _, idx := range indices {
		results[idx] = MutationResult{
			Mutation:   mutations[idx],
			Status:     Errored,
			TestOutput: msg,
		}

		completed.Add(1)
	}
}

func saveResultsToCache(cfg Config, cacheKey string, results []MutationResult) {
	if cfg.NoCache || cacheKey == "" {
		return
	}

	toCache := make(map[string]CachedResult, len(results))
	for i := range results {
		r := &results[i]
		toCache[mutationCacheKey(r.Mutation)] = CachedResult{
			Status:     r.Status,
			TestsRun:   r.TestsRun,
			TestOutput: r.TestOutput,
			DurationMs: r.Duration.Milliseconds(),
		}
	}

	if saveErr := SaveResultCache(cfg.Dir, cacheKey, toCache); saveErr != nil {
		slog.Warn("failed to save result cache", "error", saveErr)
	}
}

// DiscoverSourceFiles returns the absolute paths of all non-test Go source
// files in the given packages. Uses `go list -json` to resolve file lists.
func DiscoverSourceFiles(ctx context.Context, dir string, packages []string) ([]string, error) {
	args := append([]string{"list", "-json"}, packages...)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	var files []string

	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var pkg struct {
			Dir     string
			GoFiles []string
		}
		if err := dec.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("decoding go list output: %w", err)
		}

		for _, f := range pkg.GoFiles {
			files = append(files, filepath.Join(pkg.Dir, f))
		}
	}

	return files, nil
}

// CollectMutations parses each source file and runs all mutators against its
// AST, returning every possible mutation. Each mutation's RelFile, FileSet,
// ASTFile, and Original fields are populated here.
func CollectMutations(files []string, baseDir string, mutators []Mutator) ([]Mutation, error) {
	var all []Mutation

	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		fset := token.NewFileSet()

		file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		for _, m := range mutators {
			mutations := m.Mutate(fset, file, path, content)
			for i := range mutations {
				rel, err := filepath.Rel(baseDir, path)
				if err != nil {
					return nil, fmt.Errorf("computing relative path for %s: %w", path, err)
				}

				mutations[i].RelFile = rel
				mutations[i].FileSet = fset
				mutations[i].ASTFile = file
				mutations[i].Original = content
			}

			all = append(all, mutations...)
		}
	}

	return all, nil
}

// executeMutationWithFiles applies a single mutation, writes the mutated AST
// to a temp file, runs the covering tests via `go test -overlay`, and returns
// the result. The overlay mechanism avoids modifying the original source on disk.
func executeMutationWithFiles(ctx context.Context, m Mutation, coverMap *CoverageMap, cfg Config, mutatedPath, overlayPath string) MutationResult {
	start := time.Now()

	tests := coverMap.TestsForLine(m.RelFile, m.Line)
	if len(tests) == 0 {
		return MutationResult{
			Mutation: m,
			Status:   Uncovered,
			Duration: time.Since(start),
		}
	}

	m.Apply()
	defer m.Revert()

	if err := writeMutatedToFile(mutatedPath, m); err != nil {
		return MutationResult{
			Mutation:   m,
			Status:     Errored,
			Duration:   time.Since(start),
			TestOutput: err.Error(),
		}
	}

	return runTests(ctx, m, tests, cfg, overlayPath, start)
}

// runTests executes tests grouped by package against a mutated source file.
// It uses `-overlay` to redirect the Go toolchain to the mutated temp file
// and `-run` to execute only tests that cover the mutated line.
func runTests(ctx context.Context, m Mutation, tests []TestRef, cfg Config, overlayPath string, start time.Time) MutationResult {
	testsByPkg := groupTestsByPackage(tests)
	killed := false

	var (
		allOutput strings.Builder
		testsRun  []string
	)

	for pkg, testNames := range testsByPkg {
		pattern := "^(" + strings.Join(testNames, "|") + ")$"
		testCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		cmd := exec.CommandContext(
			testCtx, "go", "test",
			"-overlay="+overlayPath,
			"-count=1",
			"-run", pattern,
			pkg,
		)
		cmd.Dir = cfg.Dir
		// Setpgid puts the test in its own process group so SIGKILL can
		// terminate the entire group (test binary + any child processes).
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}

		output, err := cmd.CombinedOutput()

		cancel()
		allOutput.Write(output)

		testsRun = append(testsRun, testNames...)

		// A non-nil error means a test failed (exit code != 0), which
		// means the mutation was detected - mark as killed.
		if err != nil {
			killed = true
			break
		}
	}

	status := Survived
	if killed {
		status = Killed
	}

	return MutationResult{
		Mutation:   m,
		Status:     status,
		TestsRun:   testsRun,
		Duration:   time.Since(start),
		TestOutput: allOutput.String(),
	}
}

func groupTestsByPackage(tests []TestRef) map[string][]string {
	m := make(map[string][]string)
	for _, t := range tests {
		m[t.Package] = append(m[t.Package], t.Name)
	}

	return m
}

func formatMutatedSource(m Mutation) ([]byte, error) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, m.FileSet, m.ASTFile); err != nil {
		return nil, fmt.Errorf("printing mutated AST: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting mutated source: %w", err)
	}

	return formatted, nil
}

func writeMutatedToFile(path string, m Mutation) error {
	formatted, err := formatMutatedSource(m)
	if err != nil {
		return err
	}

	return os.WriteFile(path, formatted, 0o644)
}

func writeMutatedToTemp(m Mutation) (string, error) {
	formatted, err := formatMutatedSource(m)
	if err != nil {
		return "", err
	}

	return writeTempFile("mutant-*.go", formatted)
}

// writeTempFile creates a temp file matching pattern, writes data to it, and
// cleans up (closing/removing) the partial file if anything fails.
func writeTempFile(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}

	if _, err := f.Write(data); err != nil {
		return "", abortTempFile(f, err, false)
	}

	if err := f.Close(); err != nil {
		return "", abortTempFile(f, err, true)
	}

	return f.Name(), nil
}

func abortTempFile(f *os.File, cause error, closed bool) error {
	err := cause

	if !closed {
		if e := f.Close(); e != nil {
			err = fmt.Errorf("%w (close: %w)", err, e)
		}
	}

	if e := os.Remove(f.Name()); e != nil {
		err = fmt.Errorf("%w (remove: %w)", err, e)
	}

	return err
}

func mutationCacheKey(m Mutation) string {
	return m.RelFile + ":" + strconv.Itoa(m.Line) + ":" + m.Mutator + ":" + m.Description
}

func hasWildcard(packages []string) bool {
	for _, p := range packages {
		if strings.HasSuffix(p, "...") {
			return true
		}
	}

	return false
}

// writeOverlay creates a JSON overlay file that tells `go test -overlay` to
// read replacementPath whenever it would read originalPath. This lets us test
// mutated code without modifying the original source file on disk.
func writeOverlay(originalPath, replacementPath string) (string, error) {
	overlay := struct {
		Replace map[string]string `json:"Replace"`
	}{
		Replace: map[string]string{originalPath: replacementPath},
	}

	data, err := json.Marshal(overlay)
	if err != nil {
		return "", err
	}

	return writeTempFile("mutant-overlay-*.json", data)
}
