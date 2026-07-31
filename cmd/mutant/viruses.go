package main

import (
	"fmt"
	"os"

	"github.com/codingconcepts/mutant"
	"github.com/codingconcepts/mutant/mutator"
	"github.com/spf13/cobra"
)

var virusesCmd = &cobra.Command{
	Use:   "viruses",
	Short: "List available viruses",
	Long:  "Display all available mutation viruses that can be used with the --viruses flag.",
	Args:  cobra.NoArgs,
	RunE:  runViruses,
}

func init() {
	virusesCmd.Flags().StringP("mode", "m", "text", "output mode: text or json")
	rootCmd.AddCommand(virusesCmd)
}

func runViruses(cmd *cobra.Command, args []string) error {
	mode, _ := cmd.Flags().GetString("mode")

	if mode != "text" && mode != "json" {
		return fmt.Errorf("--mode must be 'text' or 'json', got %q", mode)
	}

	names := make([]string, len(mutator.Registry))
	for i, m := range mutator.Registry {
		names[i] = m.Name()
	}

	if mode == "json" {
		mutant.PrintVirusesJSON(os.Stdout, names)
	} else {
		mutant.PrintVirusesTable(os.Stdout, names)
	}

	return nil
}
