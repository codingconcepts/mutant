package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mutant",
	Short: "Mutation testing with targeted test selection",
	Long:  "A mutation testing tool for Go that uses per-test coverage profiling to run only the tests affected by each mutation.",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
