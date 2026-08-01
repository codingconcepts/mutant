package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/mattn/go-isatty"

	"github.com/codingconcepts/mutant"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [packages]",
	Short: "Execute mutation testing",
	Long:  "Run mutation testing against the specified packages. Builds a per-test coverage map, then applies each mutation and runs only the tests that cover the mutated line.",
	Args:  cobra.ArbitraryArgs,
	RunE:  runRun,
}

func init() {
	runCmd.Flags().StringP("viruses", "v", "", "comma-separated virus names to enable (default: all)")
	runCmd.Flags().StringP("mode", "m", "text", "output mode: text, json, or table")
	runCmd.Flags().StringP("output", "o", "", "write surviving mutations to file (format from extension: .json or text)")
	runCmd.Flags().DurationP("timeout", "t", 10*time.Second, "per-test timeout")
	runCmd.Flags().Bool("verbose", false, "show test output for survived mutations")
	runCmd.Flags().IntP("workers", "w", 0, "parallel workers (0 = NumCPU/2)")
	runCmd.Flags().Bool("fast-coverage", false, "use package-level coverage (faster build, runs more tests per mutation)")
	runCmd.Flags().Bool("no-cache", false, "force rebuild of coverage map, ignoring cache")
	runCmd.Flags().Bool("diff", false, "filter mutations to changed lines only (staged changes by default)")
	runCmd.Flags().Bool("unstaged", false, "diff unstaged changes instead of staged; implies --diff")
	runCmd.Flags().String("diff-ref", "", "git ref to diff against (e.g. HEAD~3, main); implies --diff")
	rootCmd.AddCommand(runCmd)
}

// runRun is the handler for `mutant run`. It parses flags, picks the output
// mode (text/json/table), and either runs in TUI mode or streams results
// to stdout.
func runRun(cmd *cobra.Command, args []string) error {
	f, err := parseCommonFlags(cmd, args)
	if err != nil {
		return err
	}

	mode, err := getFlag(cmd.Flags().GetString, "mode")
	if err != nil {
		return err
	}

	outputFile, err := getFlag(cmd.Flags().GetString, "output")
	if err != nil {
		return err
	}

	verbose, err := getFlag(cmd.Flags().GetBool, "verbose")
	if err != nil {
		return err
	}

	if verbose {
		slog.SetDefault(slog.New(log.NewWithOptions(os.Stderr, log.Options{
			ReportTimestamp: false,
			Level:           log.DebugLevel,
		})))
	}

	if mode != "text" && mode != "json" && mode != "table" {
		return fmt.Errorf("--mode must be 'text', 'json', or 'table', got %q", mode)
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg := mutant.Config{
		Dir:          dir,
		Packages:     f.packages,
		Mutators:     f.mutators,
		Timeout:      f.timeout,
		Verbose:      verbose,
		Workers:      f.workers,
		FastCoverage: f.fastCoverage,
		NoCache:      f.noCache,
		Diff:         f.diffSpec,
	}

	if mode == "table" {
		return runWithTUI(cfg, verbose, outputFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Warn("interrupted, cleaning up")
		cancel()
		mutant.RestoreAllActive()
		os.Exit(1)
	}()

	start := time.Now()

	results, err := mutant.Run(ctx, cfg)
	if err != nil {
		return err
	}

	if mode == "json" {
		if err := mutant.PrintJSON(os.Stdout, results, time.Since(start)); err != nil {
			return err
		}
	} else {
		if err := mutant.PrintTable(os.Stdout, results, verbose); err != nil {
			return err
		}
	}

	if outputFile != "" {
		if err := mutant.WriteSurvivors(outputFile, results); err != nil {
			return fmt.Errorf("writing survivors: %w", err)
		}
	}

	return nil
}

// runWithTUI launches a Bubble Tea TUI that displays a live progress table
// during mutation testing. Falls back to non-interactive mode when stdout
// is not a terminal.
func runWithTUI(cfg mutant.Config, verbose bool, outputFile string) error {
	m := newTUIModel(verbose)

	opts := []tea.ProgramOption{}
	if !isTerminal() {
		opts = []tea.ProgramOption{tea.WithInput(nil), tea.WithoutRenderer()}
	}

	p := tea.NewProgram(m, opts...)

	cfg.OnProgress = func(prog mutant.MutationProgress) {
		p.Send(progressMsg(prog))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
		mutant.RestoreAllActive()
		p.Send(tea.Quit())
	}()

	go func() {
		results, err := mutant.Run(ctx, cfg)
		p.Send(runDoneMsg{results: results, err: err})
	}()

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	fm, ok := finalModel.(*tuiModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}

	if fm.runErr != nil {
		return fm.runErr
	}

	if outputFile != "" {
		if err := mutant.WriteSurvivors(outputFile, fm.results); err != nil {
			return fmt.Errorf("writing survivors: %w", err)
		}
	}

	return nil
}

func isTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}
