package mutant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"log/slog"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
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

type Status int

const (
	Killed Status = iota
	Survived
	Uncovered
	Errored
)

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

func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToLower(s.String()))
}

type Mutation struct {
	File        string
	RelFile     string
	Mutator     string
	Description string
	Apply       func()
	Revert      func()
	FileSet     *token.FileSet
	ASTFile     *ast.File
	Original    []byte
	Line        int
}

type MutationResult struct {
	TestOutput string
	TestsRun   []string
	Mutation   Mutation
	Status     Status
	Duration   time.Duration
}

type Mutator interface {
	Name() string
	Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []Mutation
}

type Phase int

const (
	PhaseDiscoverTests Phase = iota
	PhaseBuildCoverage
	PhaseDiscoverFiles
	PhaseCollectMutations
	PhaseExecute
	PhaseDone
)

type MutationProgress struct {
	Result    *MutationResult
	Mutation  *Mutation
	Message   string
	Phase     Phase
	Completed int
	Total     int
}

type Config struct {
	OnProgress   func(MutationProgress)
	Dir          string
	Packages     []string
	Mutators     []Mutator
	Timeout      time.Duration
	Workers      int
	Verbose      bool
	FastCoverage bool
	NoCache      bool
	Diff         *DiffSpec
}

var (
	activeMu        sync.Mutex
	activeMutations = make(map[string]*Mutation)
)

func RegisterMutation(m *Mutation) {
	activeMu.Lock()
	activeMutations[m.File] = m
	activeMu.Unlock()
}

func UnregisterMutation(file string) {
	activeMu.Lock()
	delete(activeMutations, file)
	activeMu.Unlock()
}

func RestoreAllActive() {
	activeMu.Lock()
	defer activeMu.Unlock()

	for _, m := range activeMutations {
		if err := os.WriteFile(m.File, m.Original, 0o644); err != nil {
			slog.Error("failed to restore file", "file", m.File, "error", err)
		}
	}
}

func Run(ctx context.Context, cfg Config) ([]MutationResult, error) {
	progress := cfg.OnProgress
	if progress == nil {
		progress = func(p MutationProgress) {
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
	}

	packages := cfg.Packages

	var changedLines map[string][]lineRange

	if cfg.Diff != nil {
		var diffErr error

		changedLines, diffErr = ParseGitDiff(ctx, cfg.Dir, *cfg.Diff)
		if diffErr != nil {
			return nil, fmt.Errorf("parsing git diff: %w", diffErr)
		}

		if len(changedLines) == 0 {
			progress(MutationProgress{Phase: PhaseDone, Message: "No changed lines found", Completed: 0, Total: 0})
			return nil, nil
		}

		if hasWildcard(packages) {
			scopedPkgs := ChangedPackages(changedLines)
			if len(scopedPkgs) > 0 {
				packages = scopedPkgs
				progress(MutationProgress{Phase: PhaseDiscoverTests, Message: fmt.Sprintf("Diff scoped to %d package(s)", len(packages))})
			}
		}
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

	var coverResult *CoverageResult
	var cacheKey string
	var cachedResults map[string]CachedResult

	if !cfg.NoCache {
		if key, keyErr := ComputeCacheKey(cfg.Dir); keyErr == nil {
			cacheKey = key

			if cached, ok := LoadCoverageCache(cfg.Dir, cacheKey); ok {
				coverResult = cached

				progress(MutationProgress{Phase: PhaseBuildCoverage, Message: "⚡ Coverage map loaded from cache (use --no-cache to rebuild)"})
			}

			if results, ok := LoadResultCache(cfg.Dir, cacheKey); ok {
				cachedResults = results
			}
		}
	}

	if coverResult == nil {
		if cfg.FastCoverage {
			coverResult, err = BuildCoarseCoverageMap(ctx, cfg.Dir, tests, cfg.Timeout)
		} else {
			coverResult, err = BuildCoverageMap(ctx, cfg.Dir, tests, cfg.Timeout)
		}

		if err != nil {
			return nil, fmt.Errorf("building coverage map: %w", err)
		}

		if !cfg.NoCache && cacheKey != "" {
			if saveErr := SaveCoverageCache(cfg.Dir, cacheKey, coverResult); saveErr != nil {
				slog.Warn("failed to save coverage cache", "error", saveErr)
			}
		}
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
		progress(MutationProgress{Phase: PhaseCollectMutations, Message: fmt.Sprintf("Diff filter: %d → %d mutations", before, len(mutations))})

		if len(mutations) == 0 {
			progress(MutationProgress{Phase: PhaseDone, Message: "No mutations in changed lines", Completed: 0, Total: 0})
			return nil, nil
		}
	}

	progress(MutationProgress{Phase: PhaseCollectMutations, Message: fmt.Sprintf("Found %d mutations\n", len(mutations))})

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

			mutatedFile, err := os.CreateTemp("", "mutant-*.go")
			if err != nil {
				for _, idx := range indices {
					results[idx] = MutationResult{
						Mutation:   mutations[idx],
						Status:     Errored,
						TestOutput: fmt.Sprintf("creating temp file: %v", err),
					}

					completed.Add(1)
				}

				return
			}

			mutatedPath := mutatedFile.Name()
			mutatedFile.Close()

			defer os.Remove(mutatedPath)

			overlayPath, err := writeOverlay(mutations[indices[0]].File, mutatedPath)
			if err != nil {
				for _, idx := range indices {
					results[idx] = MutationResult{
						Mutation:   mutations[idx],
						Status:     Errored,
						TestOutput: fmt.Sprintf("creating overlay file: %v", err),
					}

					completed.Add(1)
				}

				return
			}

			defer os.Remove(overlayPath)

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
		}()
	}

	wg.Wait()

	if !cfg.NoCache && cacheKey != "" {
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

	progress(MutationProgress{Phase: PhaseDone, Message: "Done", Completed: total, Total: total})

	return results, nil
}

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
		cmd := exec.CommandContext(testCtx, "go", "test",
			"-overlay="+overlayPath,
			"-count=1",
			"-run", pattern,
			pkg,
		)
		cmd.Dir = cfg.Dir
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}

		output, err := cmd.CombinedOutput()

		cancel()
		allOutput.Write(output)

		testsRun = append(testsRun, testNames...)

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

func writeMutatedToFile(path string, m Mutation) error {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, m.FileSet, m.ASTFile); err != nil {
		return fmt.Errorf("printing mutated AST: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("formatting mutated source: %w", err)
	}

	return os.WriteFile(path, formatted, 0o644)
}

func writeMutatedToTemp(m Mutation) (string, error) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, m.FileSet, m.ASTFile); err != nil {
		return "", fmt.Errorf("printing mutated AST: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("formatting mutated source: %w", err)
	}

	f, err := os.CreateTemp("", "mutant-*.go")
	if err != nil {
		return "", err
	}

	if _, err := f.Write(formatted); err != nil {
		if e := f.Close(); e != nil {
			err = fmt.Errorf("%w (close: %w)", err, e)
		}

		if e := os.Remove(f.Name()); e != nil {
			err = fmt.Errorf("%w (remove: %w)", err, e)
		}

		return "", err
	}

	if err := f.Close(); err != nil {
		if e := os.Remove(f.Name()); e != nil {
			err = fmt.Errorf("%w (remove: %w)", err, e)
		}

		return "", err
	}

	return f.Name(), nil
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

	f, err := os.CreateTemp("", "mutant-overlay-*.json")
	if err != nil {
		return "", err
	}

	if _, err := f.Write(data); err != nil {
		if e := f.Close(); e != nil {
			err = fmt.Errorf("%w (close: %w)", err, e)
		}

		if e := os.Remove(f.Name()); e != nil {
			err = fmt.Errorf("%w (remove: %w)", err, e)
		}

		return "", err
	}

	if err := f.Close(); err != nil {
		if e := os.Remove(f.Name()); e != nil {
			err = fmt.Errorf("%w (remove: %w)", err, e)
		}

		return "", err
	}

	return f.Name(), nil
}
