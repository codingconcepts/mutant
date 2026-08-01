package mutant

import (
	"bufio"
	"context"
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

type DiffSpec struct {
	Ref      string
	Unstaged bool
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

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
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
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

		if strings.HasPrefix(line, "@@") && currentFile != "" {
			matches := hunkHeader.FindStringSubmatch(line)
			if matches == nil {
				continue
			}

			startLine, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}

			count := 1
			if matches[2] != "" {
				count, err = strconv.Atoi(matches[2])
				if err != nil {
					continue
				}
			}

			if count == 0 {
				continue
			}

			result[currentFile] = append(result[currentFile], lineRange{
				Start: startLine,
				End:   startLine + count - 1,
			})
		}
	}

	return result, scanner.Err()
}

func FilterMutationsByDiff(mutations []Mutation, changedLines map[string][]lineRange) []Mutation {
	if len(changedLines) == 0 {
		return nil
	}

	var filtered []Mutation

	for _, m := range mutations {
		ranges, ok := changedLines[m.RelFile]
		if !ok {
			continue
		}

		for _, r := range ranges {
			if m.Line >= r.Start && m.Line <= r.End {
				filtered = append(filtered, m)
				break
			}
		}
	}

	return filtered
}

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
