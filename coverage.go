package mutant

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// TestRef identifies a single test function within a package.
type TestRef struct {
	Name    string // test function name (e.g. "TestAdd")
	Package string // relative package path (e.g. "./pkg/math")
}

// CoverageMap maps source file lines to the tests that cover them.
// The outer key is the relative file path, the inner key is the line number.
type CoverageMap struct {
	lineToTests map[string]map[int][]TestRef
}

// CoverageResult holds the built coverage map along with timing information
// used for estimating mutation testing duration.
type CoverageResult struct {
	Map      *CoverageMap  // the line-to-test mapping
	Duration time.Duration // total wall-clock time to build the coverage map
	PerTest  time.Duration // average time per individual test run
}

// CoverageEntry represents a contiguous range of source lines covered by the
// same set of tests. Used by the `coverage` CLI command for display.
type CoverageEntry struct {
	File      string
	Tests     []string
	StartLine int
	EndLine   int
}

// TestsForLine returns all tests that cover the given line in the given file.
// Returns nil if no tests cover the line.
func (cm *CoverageMap) TestsForLine(file string, line int) []TestRef {
	lines, ok := cm.lineToTests[file]
	if !ok {
		return nil
	}

	return lines[line]
}

// Entries flattens the coverage map into a sorted list of CoverageEntry
// records, merging adjacent lines covered by the same tests into spans.
func (cm *CoverageMap) Entries() []CoverageEntry {
	entries := make([]CoverageEntry, 0, len(cm.lineToTests))

	for file, lines := range cm.lineToTests {
		for _, s := range coverageSpans(lines) {
			entries = append(entries, CoverageEntry{
				File:      file,
				StartLine: s.start,
				EndLine:   s.end,
				Tests:     s.tests,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].File != entries[j].File {
			return entries[i].File < entries[j].File
		}

		return entries[i].StartLine < entries[j].StartLine
	})

	return entries
}

type testSpan struct {
	key   string
	tests []string
	start int
	end   int
}

func coverageSpans(lines map[int][]TestRef) []testSpan {
	lineNums := make([]int, 0, len(lines))
	for l := range lines {
		lineNums = append(lineNums, l)
	}

	sort.Ints(lineNums)

	var spans []testSpan

	for _, l := range lineNums {
		names := testNames(lines[l])
		key := strings.Join(names, "|")

		if len(spans) > 0 {
			last := &spans[len(spans)-1]
			if l == last.end+1 && key == last.key {
				last.end = l
				continue
			}
		}

		spans = append(spans, testSpan{start: l, end: l, tests: names, key: key})
	}

	return spans
}

func testNames(refs []TestRef) []string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}

	return names
}

// DiscoverTests runs `go test -list .*` to enumerate all test and example
// functions in the given packages. It parses the output to pair each test
// name with its package path.
func DiscoverTests(ctx context.Context, dir string, packages []string) ([]TestRef, error) {
	args := append([]string{"test", "-list", ".*"}, packages...)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go test -list: %w\n%s", err, out)
	}

	var (
		tests   []TestRef
		pending []string
	)

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// `go test -list` outputs test names first, then a summary line
		// like "ok  pkg/path  0.001s". We accumulate test names in
		// pending, then assign them to the package from the summary line.
		if strings.HasPrefix(line, "Test") || strings.HasPrefix(line, "Example") {
			pending = append(pending, line)
			continue
		}

		if !strings.HasPrefix(line, "ok") && !strings.HasPrefix(line, "?") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		relPkg := importPathToRelPkg(fields[1], dir)
		for _, name := range pending {
			tests = append(tests, TestRef{Name: name, Package: relPkg})
		}

		pending = nil
	}

	return tests, nil
}

func importPathToRelPkg(importPath, dir string) string {
	modPath, _ := resolveModuleInfo(dir)
	if modPath == "" {
		return "./" + importPath
	}

	rel := strings.TrimPrefix(importPath, modPath)

	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "."
	}

	return "./" + rel
}

var (
	moduleInfoOnce sync.Once
	cachedModPath  string
	cachedModDir   string
)

func resolveModuleInfo(dir string) (string, string) {
	moduleInfoOnce.Do(func() {
		cmd := exec.Command("go", "list", "-m", "-json")
		cmd.Dir = dir

		out, err := cmd.Output()
		if err != nil {
			return
		}

		var info struct {
			Path string
			Dir  string
		}
		if err := json.Unmarshal(out, &info); err != nil {
			return
		}

		cachedModPath = info.Path
		cachedModDir = info.Dir
	})

	return cachedModPath, cachedModDir
}

// parallelMap runs fn over items with bounded concurrency (runtime.NumCPU
// workers) and returns the results in arbitrary order.
func parallelMap[T, R any](items []T, fn func(idx int, item T) R) []R {
	if len(items) == 0 {
		return nil
	}

	sem := make(chan struct{}, runtime.NumCPU())
	out := make(chan R, len(items))

	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)

		go func(idx int, item T) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			out <- fn(idx, item)
		}(i, item)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	results := make([]R, 0, len(items))
	for r := range out {
		results = append(results, r)
	}

	return results
}

// BuildCoverageMap creates a fine-grained coverage map by running each test
// individually with `-coverprofile`. This produces precise per-test coverage
// so mutations are tested only by tests that actually cover the mutated line,
// but is slower than BuildCoarseCoverageMap.
func BuildCoverageMap(ctx context.Context, dir string, tests []TestRef, timeout time.Duration) (_ *CoverageResult, retErr error) {
	start := time.Now()

	tmpDir, err := os.MkdirTemp("", "mutant-cover-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil && retErr == nil {
			retErr = fmt.Errorf("removing temp dir: %w", rmErr)
		}
	}()

	binaries, err := buildTestBinaries(ctx, dir, tests, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("building test binaries: %w", err)
	}

	modPath, modDir := resolveModuleInfo(dir)

	results := parallelMap(tests, func(idx int, t TestRef) testResult {
		return runCoveredTest(ctx, dir, tmpDir, binaries, timeout, idx, len(tests), t)
	})

	cm := &CoverageMap{
		lineToTests: make(map[string]map[int][]TestRef),
	}

	var totalTestTime time.Duration

	successfulTests := 0

	for _, r := range results {
		if r.err != nil {
			slog.Warn("test coverage failed", "error", r.err)
			continue
		}

		totalTestTime += r.duration
		successfulTests++

		recordTestBlocks(cm, r.blocks, modPath, modDir, dir, r.test)
	}

	elapsed := time.Since(start)

	var perTest time.Duration
	if successfulTests > 0 {
		perTest = totalTestTime / time.Duration(successfulTests)
	}

	return &CoverageResult{
		Map:      cm,
		Duration: elapsed,
		PerTest:  perTest,
	}, nil
}

type testResult struct {
	test     TestRef
	err      error
	blocks   []coverBlock
	duration time.Duration
}

func runCoveredTest(ctx context.Context, dir, tmpDir string, binaries map[string]string, timeout time.Duration, idx, total int, t TestRef) testResult {
	testStart := time.Now()

	binPath := binaries[t.Package]
	profilePath := filepath.Join(tmpDir, fmt.Sprintf("cover_%d.out", idx))

	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(
		testCtx, binPath,
		"-test.run", "^"+t.Name+"$",
		"-test.coverprofile="+profilePath,
		"-test.count=1",
	)
	cmd.Dir = resolvePkgDir(dir, t.Package)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return testResult{test: t, err: fmt.Errorf("running %s (%s): %w\n%s", t.Name, t.Package, err, output)}
	}

	blocks, err := parseCoverProfile(profilePath)
	if err != nil {
		return testResult{test: t, err: fmt.Errorf("parsing coverage for %s: %w", t.Name, err)}
	}

	d := time.Since(testStart)
	slog.Info("", "progress", fmt.Sprintf("%d/%d", idx+1, total), "test", t.Name, "package", t.Package, "duration", d.Round(time.Millisecond))

	return testResult{test: t, blocks: blocks, duration: d}
}

func recordTestBlocks(cm *CoverageMap, blocks []coverBlock, modPath, modDir, dir string, test TestRef) {
	for _, block := range blocks {
		if block.count == 0 {
			continue
		}

		relFile := importPathToFilePath(block.file, modPath, modDir, dir)
		lines := ensureLineMap(cm, relFile)

		for line := block.startLine; line <= block.endLine; line++ {
			if !containsTestRef(lines[line], test) {
				lines[line] = append(lines[line], test)
			}
		}
	}
}

func containsTestRef(refs []TestRef, t TestRef) bool {
	for _, e := range refs {
		if e.Name == t.Name && e.Package == t.Package {
			return true
		}
	}

	return false
}

func ensureLineMap(cm *CoverageMap, file string) map[int][]TestRef {
	lines, ok := cm.lineToTests[file]
	if !ok {
		lines = make(map[int][]TestRef)
		cm.lineToTests[file] = lines
	}

	return lines
}

// buildTestBinaries compiles a coverage-instrumented test binary for each
// unique package in parallel. Returns a map from package path to binary path.
// Binaries are written to tmpDir and reused across individual test runs.
func buildTestBinaries(ctx context.Context, dir string, tests []TestRef, tmpDir string) (map[string]string, error) {
	pkgList := uniquePackages(tests)

	slog.Info("building test binaries", "packages", len(pkgList))

	results := parallelMap(pkgList, func(idx int, pkg string) buildResult {
		return buildTestBinary(ctx, dir, tmpDir, idx, len(pkgList), pkg)
	})

	binaries := make(map[string]string)

	var firstErr error

	for _, r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}

			continue
		}

		binaries[r.pkg] = r.binPath
	}

	if firstErr != nil {
		return nil, firstErr
	}

	return binaries, nil
}

func uniquePackages(tests []TestRef) []string {
	pkgSet := make(map[string]struct{})

	var pkgList []string

	for _, t := range tests {
		if _, ok := pkgSet[t.Package]; !ok {
			pkgSet[t.Package] = struct{}{}
			pkgList = append(pkgList, t.Package)
		}
	}

	return pkgList
}

type buildResult struct {
	err     error
	pkg     string
	binPath string
}

func buildTestBinary(ctx context.Context, dir, tmpDir string, idx, total int, pkg string) buildResult {
	safeName := strings.ReplaceAll(strings.TrimPrefix(pkg, "./"), "/", "_")
	if safeName == "" || safeName == "." {
		safeName = "root"
	}

	binPath := filepath.Join(tmpDir, safeName+".test")

	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-cover", "-covermode=set", "-o", binPath, pkg)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return buildResult{pkg: pkg, err: fmt.Errorf("building test binary for %s: %w\n%s", pkg, err, out)}
	}

	slog.Info("built test binary", "progress", fmt.Sprintf("%d/%d", idx+1, total), "package", pkg)

	return buildResult{pkg: pkg, binPath: binPath}
}

func resolvePkgDir(baseDir, pkg string) string {
	rel := strings.TrimPrefix(pkg, "./")
	if rel == "" || rel == "." {
		return baseDir
	}

	return filepath.Join(baseDir, rel)
}

// importPathToFilePath converts a coverage profile's import-qualified file
// path (e.g. "github.com/user/repo/pkg/file.go") to a path relative to
// baseDir (e.g. "pkg/file.go") using the module path from go.mod.
func importPathToFilePath(importQualified, modPath, modDir, baseDir string) string {
	if modPath == "" {
		return importQualified
	}

	rel := strings.TrimPrefix(importQualified, modPath+"/")
	if rel == importQualified {
		return importQualified
	}

	absPath := filepath.Join(modDir, rel)

	relPath, err := filepath.Rel(baseDir, absPath)
	if err != nil {
		return rel
	}

	return relPath
}

// BuildCoarseCoverageMap creates a package-level coverage map by running all
// tests in a package together with a single `-coverprofile`. Faster to build
// than BuildCoverageMap, but less precise: all tests in a package are treated
// as covering every covered line, so mutations may run more tests than necessary.
func BuildCoarseCoverageMap(ctx context.Context, dir string, tests []TestRef, timeout time.Duration) (_ *CoverageResult, retErr error) {
	start := time.Now()

	tmpDir, err := os.MkdirTemp("", "mutant-cover-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil && retErr == nil {
			retErr = fmt.Errorf("removing temp dir: %w", rmErr)
		}
	}()

	pkgTests := make(map[string][]TestRef)
	for _, t := range tests {
		pkgTests[t.Package] = append(pkgTests[t.Package], t)
	}

	binaries, err := buildTestBinaries(ctx, dir, tests, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("building test binaries: %w", err)
	}

	modPath, modDir := resolveModuleInfo(dir)

	pkgs := make([]string, 0, len(pkgTests))
	for pkg := range pkgTests {
		pkgs = append(pkgs, pkg)
	}

	results := parallelMap(pkgs, func(idx int, pkg string) pkgResult {
		return runCoveredPackage(ctx, dir, tmpDir, binaries, timeout, idx, len(pkgs), pkg, pkgTests[pkg])
	})

	cm := &CoverageMap{
		lineToTests: make(map[string]map[int][]TestRef),
	}

	var totalTime time.Duration

	successfulPkgs := 0

	for _, r := range results {
		if r.err != nil {
			slog.Warn("package coverage failed", "error", r.err)
			continue
		}

		totalTime += r.duration
		successfulPkgs++

		recordPackageBlocks(cm, r.blocks, modPath, modDir, dir, r.tests)
	}

	elapsed := time.Since(start)

	var perTest time.Duration
	if len(tests) > 0 && successfulPkgs > 0 {
		perTest = totalTime / time.Duration(len(tests))
	}

	return &CoverageResult{
		Map:      cm,
		Duration: elapsed,
		PerTest:  perTest,
	}, nil
}

type pkgResult struct {
	pkg      string
	err      error
	tests    []TestRef
	blocks   []coverBlock
	duration time.Duration
}

func runCoveredPackage(ctx context.Context, dir, tmpDir string, binaries map[string]string, timeout time.Duration, idx, total int, pkg string, testsInPkg []TestRef) pkgResult {
	testStart := time.Now()

	binPath := binaries[pkg]
	profilePath := filepath.Join(tmpDir, fmt.Sprintf("cover_pkg_%d.out", idx))

	pkgTimeout := timeout * time.Duration(len(testsInPkg))

	testCtx, cancel := context.WithTimeout(ctx, pkgTimeout)
	defer cancel()

	cmd := exec.CommandContext(
		testCtx, binPath,
		"-test.coverprofile="+profilePath,
		"-test.count=1",
	)
	cmd.Dir = resolvePkgDir(dir, pkg)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return pkgResult{pkg: pkg, err: fmt.Errorf("running tests in %s: %w\n%s", pkg, err, output)}
	}

	blocks, err := parseCoverProfile(profilePath)
	if err != nil {
		return pkgResult{pkg: pkg, err: fmt.Errorf("parsing coverage for %s: %w", pkg, err)}
	}

	d := time.Since(testStart)
	slog.Info("package coverage complete", "progress", fmt.Sprintf("%d/%d", idx+1, total), "package", pkg, "tests", len(testsInPkg), "duration", d.Round(time.Millisecond))

	return pkgResult{pkg: pkg, tests: testsInPkg, blocks: blocks, duration: d}
}

func recordPackageBlocks(cm *CoverageMap, blocks []coverBlock, modPath, modDir, dir string, tests []TestRef) {
	for _, block := range blocks {
		if block.count == 0 {
			continue
		}

		relFile := importPathToFilePath(block.file, modPath, modDir, dir)
		lines := ensureLineMap(cm, relFile)

		for line := block.startLine; line <= block.endLine; line++ {
			lines[line] = tests
		}
	}
}

type coverBlock struct {
	file      string
	startLine int
	startCol  int
	endLine   int
	endCol    int
	numStmt   int
	count     int
}

// parseCoverProfile reads a Go cover profile file and extracts coverage
// blocks. Each block describes a source range and how many times it was hit.
func parseCoverProfile(path string) (_ []coverBlock, retErr error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	var blocks []coverBlock

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") || line == "" {
			continue
		}

		// Format: file:startLine.startCol,endLine.endCol numStmt count
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx < 0 {
			continue
		}

		file := line[:colonIdx]
		rest := line[colonIdx+1:]

		var b coverBlock

		b.file = file

		_, err := fmt.Sscanf(rest, "%d.%d,%d.%d %d %d",
			&b.startLine, &b.startCol, &b.endLine, &b.endCol, &b.numStmt, &b.count)
		if err != nil {
			continue
		}

		blocks = append(blocks, b)
	}

	return blocks, scanner.Err()
}
