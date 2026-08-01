package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mutant",
	Short: "Mutation testing with targeted test selection",
	Long:  "A mutation testing tool for Go that uses per-test coverage profiling to run only the tests affected by each mutation.",
}

func main() {
	slog.SetDefault(slog.New(log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
