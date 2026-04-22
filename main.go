package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runTUI(screenChooser, "")
		return
	}
	input := strings.Join(args, " ")
	if isPathOrURL(input) {
		runTUI(screenSave, input)
	} else {
		runTUI(screenSearch, input)
	}
}

func runTUI(initial screenID, arg string) {
	m := newModel(initial, arg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

