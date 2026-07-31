package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/codingconcepts/mutant"
	"github.com/codingconcepts/mutant/mutator"
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
	runCmd.Flags().IntP("workers", "w", 1, "parallel workers (0 to use all CPUs)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	packages := args
	if len(packages) == 0 {
		packages = []string{"./..."}
	}

	virusFlag, err := cmd.Flags().GetString("viruses")
	if err != nil {
		return fmt.Errorf("getting viruses flag: %w", err)
	}

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return fmt.Errorf("getting mode flag: %w", err)
	}

	outputFile, err := cmd.Flags().GetString("output")
	if err != nil {
		return fmt.Errorf("getting output flag: %w", err)
	}

	timeout, err := cmd.Flags().GetDuration("timeout")
	if err != nil {
		return fmt.Errorf("getting timeout flag: %w", err)
	}

	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return fmt.Errorf("getting verbose flag: %w", err)
	}

	workers, err := cmd.Flags().GetInt("workers")
	if err != nil {
		return fmt.Errorf("getting workers flag: %w", err)
	}

	if mode != "text" && mode != "json" && mode != "table" {
		return fmt.Errorf("--mode must be 'text', 'json', or 'table', got %q", mode)
	}

	var virusNames []string
	if virusFlag != "" {
		virusNames = strings.Split(virusFlag, ",")
	}

	mutators := mutator.ByName(virusNames)
	if len(mutators) == 0 {
		return fmt.Errorf("no matching viruses found")
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg := mutant.Config{
		Dir:      dir,
		Packages: packages,
		Mutators: mutators,
		Timeout:  timeout,
		Verbose:  verbose,
		Workers:  workers,
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
		fmt.Fprintf(os.Stderr, "\nInterrupted, cleaning up...\n")
		cancel()
		mutant.RestoreAllActive()
		os.Exit(1)
	}()

	start := time.Now()

	results, err := mutant.Run(ctx, cfg)
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr)

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

	fm, ok := finalModel.(tuiModel)
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
