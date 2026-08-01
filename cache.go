package mutant

import (
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	cacheDir       = ".mutant-cache"
	cacheFile      = "covermap.gob"
	resultCacheFile = "results.gob"
)

type CoverageCache struct {
	Key         string
	LineToTests map[string]map[int][]TestRef
	PerTest     time.Duration
	Duration    time.Duration
}

func ComputeCacheKey(dir string) (string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || base == cacheDir {
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
		return "", fmt.Errorf("walking directory: %w", err)
	}

	for _, name := range []string{"go.mod", "go.sum"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			files = append(files, p)
		}
	}

	sort.Strings(files)

	h := sha256.New()

	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		fmt.Fprintf(h, "file:%s\n", rel)

		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", f, err)
		}

		h.Write(data)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func LoadCoverageCache(dir string, key string) (*CoverageResult, bool) {
	path := filepath.Join(dir, cacheDir, cacheFile)

	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var cache CoverageCache
	if err := gob.NewDecoder(f).Decode(&cache); err != nil {
		return nil, false
	}

	if cache.Key != key {
		return nil, false
	}

	return &CoverageResult{
		Map:      &CoverageMap{lineToTests: cache.LineToTests},
		Duration: cache.Duration,
		PerTest:  cache.PerTest,
	}, true
}

func SaveCoverageCache(dir string, key string, result *CoverageResult) error {
	cacheDirectory := filepath.Join(dir, cacheDir)
	if err := os.MkdirAll(cacheDirectory, 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	cache := CoverageCache{
		Key:         key,
		LineToTests: result.Map.lineToTests,
		PerTest:     result.PerTest,
		Duration:    result.Duration,
	}

	path := filepath.Join(cacheDirectory, cacheFile)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating cache file: %w", err)
	}
	defer f.Close()

	if err := gob.NewEncoder(f).Encode(cache); err != nil {
		return fmt.Errorf("encoding cache: %w", err)
	}

	return nil
}

type CachedResult struct {
	Status     Status
	TestsRun   []string
	TestOutput string
	DurationMs int64
}

type ResultCache struct {
	Key     string
	Results map[string]CachedResult
}

func LoadResultCache(dir string, key string) (map[string]CachedResult, bool) {
	path := filepath.Join(dir, cacheDir, resultCacheFile)

	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var cache ResultCache
	if err := gob.NewDecoder(f).Decode(&cache); err != nil {
		return nil, false
	}

	if cache.Key != key {
		return nil, false
	}

	return cache.Results, true
}

func SaveResultCache(dir string, key string, results map[string]CachedResult) error {
	cacheDirectory := filepath.Join(dir, cacheDir)
	if err := os.MkdirAll(cacheDirectory, 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	cache := ResultCache{
		Key:     key,
		Results: results,
	}

	path := filepath.Join(cacheDirectory, resultCacheFile)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating result cache file: %w", err)
	}
	defer f.Close()

	if err := gob.NewEncoder(f).Encode(cache); err != nil {
		return fmt.Errorf("encoding result cache: %w", err)
	}

	return nil
}
