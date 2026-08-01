# mutant
A fast, coverage-aware mutation testing tool for Go. Mutant builds a per-test coverage map, then applies mutations only where tests exist - running just the tests that cover each mutated line.

### Install

```sh
go install github.com/codingconcepts/mutant/cmd/mutant@latest
```

### Quick start

```sh
# Run mutation testing against your whole project.
mutant run ./...

# Run against a specific package (or directory).
mutant run ./pkg/auth/... # Package

mutal run ./example       # Directory

```

### Usage

#### Run mutation tests

```sh
# Default text output.
mutant run ./...

# Interactive TUI with live progress.
mutant run --mode table ./...

# JSON output for CI pipelines.
mutant run --mode json ./...

# Save surviving mutations to a file.
mutant run --output mutant.json ./...

# Show test output for survivors (useful for debugging weak tests).
mutant run --verbose ./...
```

#### Run specific viruses (mutators)

```sh
# Only arithmetic mutations.
mutant run --viruses arithmetic ./...

# Multiple viruses.
mutant run --viruses arithmetic,comparison,bitwise ./...

# List all available viruses.
mutant viruses
```

#### Diff mode - only test what changed

```sh
# Mutate only staged changes.
mutant run --diff ./...

# Mutate changes since a specific ref.
mutant run --diff-ref main ./...
mutant run --diff-ref HEAD~3 ./...
```

#### Performance tuning

```sh
# Control parallelism (default: NumCPU/2).
mutant run --workers 8 ./...

# Faster coverage builds (package-level instead of per-test).
mutant run --fast-coverage ./...

# Force rebuild of the coverage cache.
mutant run --no-cache ./...

# Set per-test timeout (default: 10s).
mutant run --timeout 30s ./...
```

#### Plan - estimate before running

```sh
# See mutation count, test count, and estimated duration.
mutant plan ./...

# JSON output.
mutant plan --mode json ./...

# Plan with diff filter.
mutant plan --diff-ref main ./...
```

#### Inspect the coverage map

```sh
# Show which tests cover which source lines.
mutant coverage ./...

# JSON output.
mutant coverage --mode json ./...
```

### CI example

```sh
mutant run --mode json --output mutant.json ./...

# Fail the build if any mutations survived.
if jq -e 'length > 0' mutant.json > /dev/null 2>&1; then
  echo "Mutation testing failed: $(jq length mutant.json) survivors"
  exit 1
fi
```

### Common workflows

**After writing a new feature** - run mutant to check test strength:
```sh
mutant run --diff-ref main ./...
```

**Investigating a surviving mutation** - see which test ran and what it output:
```sh
mutant run --verbose --viruses comparison ./pkg/calc/...
```

**Pre-merge gate** - fast check on changed code only:
```sh
mutant run --diff --fast-coverage --mode json ./...
```

### Todos

