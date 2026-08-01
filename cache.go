package mutant

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	cacheDir        = ".mutant-cache"
	cacheFile       = "covermap.gob"
	resultCacheFile = "results.gob"
)

// CoverageCache is the gob-serialized form of a CoverageResult, stored in
// .mutant-cache/covermap.gob. The Key is a SHA-256 hash of all Go source
// files and go.mod/go.sum; a key mismatch means the cache is stale.
type CoverageCache struct {
	LineToTests map[string]map[int][]TestRef
	Key         string
	PerTest     time.Duration
	Duration    time.Duration
}

func (c CoverageCache) cacheKey() string { return c.Key }

// ComputeCacheKey produces a SHA-256 hash of all .go files, go.mod, and
// go.sum in the project directory. Any source change invalidates the cache.
func ComputeCacheKey(dir string) (string, error) {
	files, err := collectHashableFiles(dir)
	if err != nil {
		return "", err
	}

	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		if err := hashFile(h, dir, f); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func collectHashableFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if isSkippedDir(filepath.Base(path)) {
				return filepath.SkipDir
			}

			return nil
		}

		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	for _, name := range []string{"go.mod", "go.sum"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			files = append(files, p)
		}
	}

	return files, nil
}

func isSkippedDir(base string) bool {
	return base == "vendor" || base == ".git" || base == cacheDir
}

func hashFile(h io.Writer, dir, f string) error {
	rel, err := filepath.Rel(dir, f)
	if err != nil {
		return fmt.Errorf("computing relative path for %s: %w", f, err)
	}

	if _, err = io.WriteString(h, "file:"+rel+"\n"); err != nil {
		return fmt.Errorf("hashing file path: %w", err)
	}

	data, err := os.ReadFile(f)
	if err != nil {
		return fmt.Errorf("reading %s: %w", f, err)
	}

	if _, err = h.Write(data); err != nil {
		return fmt.Errorf("hashing file content: %w", err)
	}

	return nil
}

// LoadCoverageCache loads and validates a cached coverage map from disk.
// Returns false if the cache file doesn't exist or the key doesn't match.
func LoadCoverageCache(dir string, key string) (*CoverageResult, bool) {
	cache, ok := loadCache[CoverageCache](filepath.Join(dir, cacheDir, cacheFile), key)
	if !ok {
		return nil, false
	}

	return &CoverageResult{
		Map:      &CoverageMap{lineToTests: cache.LineToTests},
		Duration: cache.Duration,
		PerTest:  cache.PerTest,
	}, true
}

// SaveCoverageCache persists a coverage map to .mutant-cache/covermap.gob.
func SaveCoverageCache(dir string, key string, result *CoverageResult) error {
	cache := CoverageCache{
		Key:         key,
		LineToTests: result.Map.lineToTests,
		PerTest:     result.PerTest,
		Duration:    result.Duration,
	}

	return saveCache(dir, cacheFile, cache)
}

// CachedResult stores the outcome of a single mutation test, keyed by
// "file:line:mutator:description". Stored in .mutant-cache/results.gob.
type CachedResult struct {
	TestOutput string
	TestsRun   []string
	Status     Status
	DurationMs int64
}

// ResultCache is the gob-serialized form of per-mutation test results.
type ResultCache struct {
	Results map[string]CachedResult
	Key     string
}

func (c ResultCache) cacheKey() string { return c.Key }

// LoadResultCache loads cached mutation test results from disk.
// Returns false if the cache file doesn't exist or the key doesn't match.
func LoadResultCache(dir string, key string) (map[string]CachedResult, bool) {
	cache, ok := loadCache[ResultCache](filepath.Join(dir, cacheDir, resultCacheFile), key)
	if !ok {
		return nil, false
	}

	return cache.Results, true
}

// SaveResultCache persists mutation test results to .mutant-cache/results.gob.
func SaveResultCache(dir string, key string, results map[string]CachedResult) error {
	cache := ResultCache{
		Key:     key,
		Results: results,
	}

	return saveCache(dir, resultCacheFile, cache)
}

type keyedCache interface {
	cacheKey() string
}

func loadCache[T keyedCache](path string, wantKey string) (T, bool) {
	var zero T

	f, err := os.Open(path)
	if err != nil {
		return zero, false
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Debug("closing cache file", "path", path, "error", err)
		}
	}()

	var cache T
	if err := gob.NewDecoder(f).Decode(&cache); err != nil {
		return zero, false
	}

	if cache.cacheKey() != wantKey {
		return zero, false
	}

	return cache, true
}

func saveCache[T any](dir, filename string, cache T) (retErr error) {
	cacheDirectory := filepath.Join(dir, cacheDir)
	if err := os.MkdirAll(cacheDirectory, 0o750); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	path := filepath.Join(cacheDirectory, filename)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating cache file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()

	if err := gob.NewEncoder(f).Encode(cache); err != nil {
		return fmt.Errorf("encoding cache: %w", err)
	}

	return nil
}
