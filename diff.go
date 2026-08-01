package mutant

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type lineRange struct {
	Start int
	End   int
}

// DiffSpec controls which git diff to parse. When Ref is set, diffs against
// that ref (e.g. "main", "HEAD~3"). When Unstaged is true, diffs working tree
// changes. When both are empty, diffs staged (--cached) changes.
type DiffSpec struct {
	Ref      string // git ref to diff against (e.g. "main", "HEAD~3")
	Unstaged bool   // diff unstaged changes instead of staged
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// ParseGitDiff runs `git diff` with the given spec and returns a map of
// changed file paths to the line ranges that were added or modified.
func ParseGitDiff(ctx context.Context, dir string, spec DiffSpec) (map[string][]lineRange, error) {
	args := []string{"diff", "--unified=0", "--no-color"}

	switch {
	case spec.Ref != "":
		args = append(args, spec.Ref)
	case spec.Unstaged:
		// default git diff behavior
	default:
		args = append(args, "--cached")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("git diff: %w\n%s", err, exitErr.Stderr)
		}

		if len(out) == 0 {
			return make(map[string][]lineRange), nil
		}
	}

	return parseDiffOutput(string(out))
}

func parseDiffOutput(output string) (map[string][]lineRange, error) {
	result := make(map[string][]lineRange)

	var currentFile string

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if after, ok := strings.CutPrefix(line, "+++ b/"); ok {
			currentFile = after
			continue
		}

		if currentFile == "" || !strings.HasPrefix(line, "@@") {
			continue
		}

		r, ok := parseHunkRange(line)
		if !ok {
			continue
		}

		result[currentFile] = append(result[currentFile], r)
	}

	return result, scanner.Err()
}

func parseHunkRange(line string) (lineRange, bool) {
	matches := hunkHeader.FindStringSubmatch(line)
	if matches == nil {
		return lineRange{}, false
	}

	startLine, err := strconv.Atoi(matches[1])
	if err != nil {
		return lineRange{}, false
	}

	count := 1
	if matches[2] != "" {
		count, err = strconv.Atoi(matches[2])
		if err != nil {
			return lineRange{}, false
		}
	}

	if count == 0 {
		return lineRange{}, false
	}

	return lineRange{Start: startLine, End: startLine + count - 1}, true
}

// FilterMutationsByDiff keeps only mutations whose line falls within a
// changed range from the diff. Used by --diff mode to limit mutation
// testing to recently changed code.
func FilterMutationsByDiff(mutations []Mutation, changedLines map[string][]lineRange) []Mutation {
	if len(changedLines) == 0 {
		return nil
	}

	var filtered []Mutation

	for i := range mutations {
		ranges, ok := changedLines[mutations[i].RelFile]
		if !ok {
			continue
		}

		for _, r := range ranges {
			if mutations[i].Line >= r.Start && mutations[i].Line <= r.End {
				filtered = append(filtered, mutations[i])
				break
			}
		}
	}

	return filtered
}

// ChangedPackages derives Go package paths from the changed file paths in a
// diff. Used to scope test discovery to only packages with changes when the
// user specified `./...` as the package pattern.
func ChangedPackages(changedLines map[string][]lineRange) []string {
	pkgSet := make(map[string]struct{})

	for file := range changedLines {
		if !strings.HasSuffix(file, ".go") {
			continue
		}

		dir := "./"
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			dir = "./" + file[:idx]
		}

		pkgSet[dir] = struct{}{}
	}

	pkgs := make([]string, 0, len(pkgSet))
	for pkg := range pkgSet {
		pkgs = append(pkgs, pkg)
	}

	return pkgs
}
