package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codingconcepts/mutant"
)

type virusRow struct {
	total     int
	killed    int
	survived  int
	uncovered int
	errored   int
}

type tuiModel struct {
	startTime  time.Time
	runErr     error
	virusStats map[string]*virusRow
	phaseMsg   string
	virusOrder []string
	results    []mutant.MutationResult
	phase      mutant.Phase
	completed  int
	total      int
	done       bool
	verbose    bool
}

type (
	progressMsg mutant.MutationProgress
	runDoneMsg struct {
		err     error
		results []mutant.MutationResult
	}
)

func newTUIModel(verbose bool) tuiModel {
	return tuiModel{
		virusStats: make(map[string]*virusRow),
		startTime:  time.Now(),
		verbose:    verbose,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			mutant.RestoreAllActive()
			return m, tea.Quit
		}

	case progressMsg:
		p := mutant.MutationProgress(msg)

		m.phase = p.Phase
		if p.Message != "" {
			m.phaseMsg = p.Message
		}

		if p.Phase == mutant.PhaseCollectMutations && p.Total > 0 {
			m.total = p.Total
		}

		if p.Phase == mutant.PhaseExecute && p.Result != nil {
			m.completed = p.Completed
			m.total = p.Total
			m.results = append(m.results, *p.Result)

			virus := p.Mutation.Mutator

			row, ok := m.virusStats[virus]
			if !ok {
				row = &virusRow{}
				m.virusStats[virus] = row
				m.virusOrder = append(m.virusOrder, virus)
				sort.Strings(m.virusOrder)
			}

			row.total++

			switch p.Result.Status {
			case mutant.Killed:
				row.killed++
			case mutant.Survived:
				row.survived++
			case mutant.Uncovered:
				row.uncovered++
			case mutant.Errored:
				row.errored++
			}
		}

	case runDoneMsg:
		m.done = true
		m.results = msg.results
		m.runErr = msg.err

		return m, tea.Quit
	}

	return m, nil
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	killedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	survivedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	uncovStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	progressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	phaseStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

func (m tuiModel) View() string {
	var b strings.Builder

	phase := m.phaseMsg
	if phase == "" {
		phase = "Initializing..."
	}

	b.WriteString(titleStyle.Render("Mutation Testing"))
	b.WriteString(" — ")
	b.WriteString(phaseStyle.Render(phase))
	b.WriteString("\n\n")

	if len(m.virusOrder) > 0 {
		maxName := 5
		for _, v := range m.virusOrder {
			if len(v) > maxName {
				maxName = len(v)
			}
		}

		hdr := fmt.Sprintf("%-*s  %6s  %6s  %8s  %7s  %7s",
			maxName, "VIRUS", "TOTAL", "KILLED", "SURVIVED", "UNCOVERED", "ERRORED")
		b.WriteString(headerStyle.Render(hdr))
		b.WriteString("\n")

		for _, virus := range m.virusOrder {
			row := m.virusStats[virus]
			name := fmt.Sprintf("%-*s", maxName, virus)
			total := fmt.Sprintf("%6d", row.total)
			killed := fmt.Sprintf("%6d", row.killed)
			survived := fmt.Sprintf("%8d", row.survived)
			uncov := fmt.Sprintf("%7d", row.uncovered)
			errored := fmt.Sprintf("%7d", row.errored)

			fmt.Fprintf(&b, "%s  %s  %s  %s  %s  %s\n",
				name,
				total,
				killedStyle.Render(killed),
				survivedStyle.Render(survived),
				uncovStyle.Render(uncov),
				errored)
		}

		b.WriteString("\n")
	}

	if m.total > 0 {
		pct := float64(m.completed) / float64(m.total) * 100
		elapsed := time.Since(m.startTime).Round(time.Second)
		bar := renderProgressBar(m.completed, m.total, 40)
		b.WriteString(progressStyle.Render(fmt.Sprintf("%s %d/%d (%.1f%%)  Elapsed: %v",
			bar, m.completed, m.total, pct, elapsed)))
		b.WriteString("\n")
	}

	if m.done {
		b.WriteString("\n")

		var killed, survived, uncovered int

		for i := range m.results {
			switch m.results[i].Status {
			case mutant.Killed:
				killed++
			case mutant.Survived:
				survived++
			case mutant.Uncovered:
				uncovered++
			}
		}

		testable := len(m.results) - uncovered

		var score float64
		if testable > 0 {
			score = float64(killed) / float64(testable) * 100
		}

		fmt.Fprintf(&b, "Score: %d/%d mutations killed (%.2f%%)\n", killed, testable, score)

		if survived > 0 {
			b.WriteString(survivedStyle.Render(fmt.Sprintf("Survived: %d mutations were not caught by tests\n", survived)))
		}

		if uncovered > 0 {
			b.WriteString(uncovStyle.Render(fmt.Sprintf("Uncovered: %d mutations had no test coverage\n", uncovered)))
		}

		if survived > 0 {
			b.WriteString("\n")
			b.WriteString(survivedStyle.Render("══════════════════════════════════════"))
			b.WriteString("\n")
			b.WriteString(survivedStyle.Render(fmt.Sprintf(" SURVIVING MUTATIONS (%d)", survived)))
			b.WriteString("\n")
			b.WriteString(survivedStyle.Render("══════════════════════════════════════"))
			b.WriteString("\n\n")

			n := 0

			for i := range m.results {
				r := &m.results[i]
				if r.Status != mutant.Survived {
					continue
				}

				n++
				fmt.Fprintf(&b, "  %d. %s:%d\n", n, r.Mutation.RelFile, r.Mutation.Line)
				fmt.Fprintf(&b, "     Virus:  %s\n", r.Mutation.Mutator)
				fmt.Fprintf(&b, "     Change: %s\n", r.Mutation.Description)

				if len(r.TestsRun) > 0 {
					fmt.Fprintf(&b, "     Tests:  %s\n", strings.Join(r.TestsRun, ", "))
				}

				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

func renderProgressBar(completed, total, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}

	filled := min(completed*width/total, width)

	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
