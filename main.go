// hank is a small terminal tool to track tasks and projects.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hoffmannma/hank/internal/config"
	"github.com/hoffmannma/hank/internal/store"
	"github.com/hoffmannma/hank/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hank:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dbPath, err := cfg.ResolveDBPath()
	if err != nil {
		return err
	}
	s, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer s.Close()

	_, err = tea.NewProgram(ui.New(s, cfg.PageSize()), tea.WithAltScreen()).Run()
	return err
}
