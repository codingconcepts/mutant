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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	OnProgress func(MutationProgress)
	Dir        string
	Packages   []string
	Mutators   []Mutator
	Timeout    time.Duration
	Workers    int
	Verbose    bool
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
			fmt.Fprintf(os.Stderr, "FATAL: failed to restore %s: %v\n", m.File, err)
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

				fmt.Fprintf(os.Stderr, "[%d/%d] %s:%d %s %q ... %s (%s, %v)\n",
					p.Completed, p.Total,
					p.Mutation.RelFile, p.Mutation.Line, p.Mutation.Mutator, p.Mutation.Description,
					p.Result.Status, testNames, p.Result.Duration.Round(time.Millisecond))
			} else if p.Message != "" {
				fmt.Fprintf(os.Stderr, "%s\n", p.Message)
			}
		}
	}

	progress(MutationProgress{Phase: PhaseDiscoverTests, Message: "Discovering tests..."})

	tests, err := DiscoverTests(ctx, cfg.Dir, cfg.Packages)
	if err != nil {
		return nil, fmt.Errorf("discovering tests: %w", err)
	}

	progress(MutationProgress{Phase: PhaseDiscoverTests, Message: fmt.Sprintf("Found %d tests", len(tests))})

	if len(tests) == 0 {
		return nil, fmt.Errorf("no tests found in %v", cfg.Packages)
	}

	progress(MutationProgress{Phase: PhaseBuildCoverage, Message: "Building coverage map..."})

	coverResult, err := BuildCoverageMap(ctx, cfg.Dir, tests, cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("building coverage map: %w", err)
	}

	coverMap := coverResult.Map
	progress(MutationProgress{Phase: PhaseBuildCoverage, Message: fmt.Sprintf("Coverage map complete (%v)", coverResult.Duration.Round(time.Millisecond))})

	progress(MutationProgress{Phase: PhaseDiscoverFiles, Message: "Discovering source files..."})

	files, err := DiscoverSourceFiles(ctx, cfg.Dir, cfg.Packages)
	if err != nil {
		return nil, fmt.Errorf("discovering source files: %w", err)
	}

	progress(MutationProgress{Phase: PhaseDiscoverFiles, Message: fmt.Sprintf("Found %d source files", len(files))})

	progress(MutationProgress{Phase: PhaseCollectMutations, Message: "Collecting mutations..."})

	mutations, err := CollectMutations(files, cfg.Dir, cfg.Mutators)
	if err != nil {
		return nil, fmt.Errorf("collecting mutations: %w", err)
	}

	progress(MutationProgress{Phase: PhaseCollectMutations, Message: fmt.Sprintf("Found %d mutations\n", len(mutations))})

	workers := cfg.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
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

			for _, idx := range indices {
				if ctx.Err() != nil {
					break
				}

				m := mutations[idx]
				result := executeMutation(ctx, m, coverMap, cfg)
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

func executeMutation(ctx context.Context, m Mutation, coverMap *CoverageMap, cfg Config) MutationResult {
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

	mutatedPath, err := writeMutatedToTemp(m)
	if err != nil {
		return MutationResult{
			Mutation:   m,
			Status:     Errored,
			Duration:   time.Since(start),
			TestOutput: err.Error(),
		}
	}
	defer func() {
		if rmErr := os.Remove(mutatedPath); rmErr != nil && !os.IsNotExist(rmErr) {
			fmt.Fprintf(os.Stderr, "warning: removing temp file: %v\n", rmErr)
		}
	}()

	overlayPath, err := writeOverlay(m.File, mutatedPath)
	if err != nil {
		return MutationResult{
			Mutation:   m,
			Status:     Errored,
			Duration:   time.Since(start),
			TestOutput: err.Error(),
		}
	}
	defer func() {
		if err := os.Remove(overlayPath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: removing temp file: %v\n", err)
		}
	}()

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
