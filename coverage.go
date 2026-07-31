package mutant

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

type TestRef struct {
	Name    string
	Package string
}

type CoverageMap struct {
	lineToTests map[string]map[int][]TestRef
}

type CoverageResult struct {
	Map      *CoverageMap
	Duration time.Duration
	PerTest  time.Duration
}

type CoverageEntry struct {
	File      string
	Tests     []string
	StartLine int
	EndLine   int
}

func (cm *CoverageMap) TestsForLine(file string, line int) []TestRef {
	lines, ok := cm.lineToTests[file]
	if !ok {
		return nil
	}

	return lines[line]
}

func (cm *CoverageMap) Entries() []CoverageEntry {
	entries := make([]CoverageEntry, 0, len(cm.lineToTests))

	for file, lines := range cm.lineToTests {
		type span struct {
			tests []string
			start int
			end   int
		}

		testKey := func(refs []TestRef) string {
			names := make([]string, len(refs))
			for i, r := range refs {
				names[i] = r.Name
			}

			return strings.Join(names, "|")
		}

		lineNums := make([]int, 0, len(lines))
		for l := range lines {
			lineNums = append(lineNums, l)
		}

		sort.Ints(lineNums)

		var spans []span

		for _, l := range lineNums {
			refs := lines[l]
			key := testKey(refs)

			names := make([]string, len(refs))
			for i, r := range refs {
				names[i] = r.Name
			}

			if len(spans) > 0 {
				last := &spans[len(spans)-1]
				if l == last.end+1 && testKey(refs) == key {
					last.end = l
					continue
				}
			}

			_ = key

			spans = append(spans, span{start: l, end: l, tests: names})
		}

		for _, s := range spans {
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

		if strings.HasPrefix(line, "ok") || strings.HasPrefix(line, "?") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				pkg := fields[1]

				relPkg := importPathToRelPkg(pkg, dir)
				for _, name := range pending {
					tests = append(tests, TestRef{Name: name, Package: relPkg})
				}

				pending = nil
			}

			continue
		}

		if strings.HasPrefix(line, "Test") || strings.HasPrefix(line, "Example") {
			pending = append(pending, line)
		}
	}

	return tests, nil
}

func importPathToRelPkg(importPath, dir string) string {
	modPath, modDir := resolveModuleInfo(dir)
	if modPath == "" {
		return "./" + importPath
	}

	rel := strings.TrimPrefix(importPath, modPath)

	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return modDir
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

	type testResult struct {
		test     TestRef
		err      error
		blocks   []coverBlock
		duration time.Duration
	}

	results := make(chan testResult, len(tests))
	sem := make(chan struct{}, runtime.NumCPU())

	var wg sync.WaitGroup

	for i, t := range tests {
		wg.Add(1)
		go func(idx int, t TestRef) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

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
				results <- testResult{test: t, err: fmt.Errorf("running %s (%s): %w\n%s", t.Name, t.Package, err, output)}
				return
			}

			blocks, err := parseCoverProfile(profilePath)
			if err != nil {
				results <- testResult{test: t, err: fmt.Errorf("parsing coverage for %s: %w", t.Name, err)}
				return
			}

			d := time.Since(testStart)
			fmt.Fprintf(os.Stderr, "  [%d/%d] %s (%s) ... done (%v)\n", idx+1, len(tests), t.Name, t.Package, d.Round(time.Millisecond))

			results <- testResult{test: t, blocks: blocks, duration: d}
		}(i, t)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	cm := &CoverageMap{
		lineToTests: make(map[string]map[int][]TestRef),
	}

	var totalTestTime time.Duration

	successfulTests := 0

	for r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "  warning: %v\n", r.err)
			continue
		}

		totalTestTime += r.duration
		successfulTests++

		for _, block := range r.blocks {
			if block.count == 0 {
				continue
			}

			relFile := importPathToFilePath(block.file, modPath, modDir, dir)

			lines, ok := cm.lineToTests[relFile]
			if !ok {
				lines = make(map[int][]TestRef)
				cm.lineToTests[relFile] = lines
			}

			for line := block.startLine; line <= block.endLine; line++ {
				existing := lines[line]
				found := false

				for _, e := range existing {
					if e.Name == r.test.Name && e.Package == r.test.Package {
						found = true
						break
					}
				}

				if !found {
					lines[line] = append(lines[line], r.test)
				}
			}
		}
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

func buildTestBinaries(ctx context.Context, dir string, tests []TestRef, tmpDir string) (map[string]string, error) {
	pkgs := make(map[string]struct{})
	for _, t := range tests {
		pkgs[t.Package] = struct{}{}
	}

	fmt.Fprintf(os.Stderr, "  Building test binaries for %d package(s)...\n", len(pkgs))

	binaries := make(map[string]string)

	for pkg := range pkgs {
		safeName := strings.ReplaceAll(strings.TrimPrefix(pkg, "./"), "/", "_")
		if safeName == "" || safeName == "." {
			safeName = "root"
		}

		binPath := filepath.Join(tmpDir, safeName+".test")

		cmd := exec.CommandContext(ctx, "go", "test", "-c", "-cover", "-covermode=set", "-o", binPath, pkg)
		cmd.Dir = dir

		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("building test binary for %s: %w\n%s", pkg, err, out)
		}

		binaries[pkg] = binPath
	}

	return binaries, nil
}

func resolvePkgDir(baseDir, pkg string) string {
	rel := strings.TrimPrefix(pkg, "./")
	if rel == "" || rel == "." {
		return baseDir
	}

	return filepath.Join(baseDir, rel)
}

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

type coverBlock struct {
	file      string
	startLine int
	startCol  int
	endLine   int
	endCol    int
	numStmt   int
	count     int
}

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
