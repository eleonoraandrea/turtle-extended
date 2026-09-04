package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/atps/atps/internal/tui"
)

func cmdTUI() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Lancia TUI interattiva — backtest visuale con grafici ASCII e report HTML",
		Long:  `TUI ATPS: seleziona simbolo/variante/interval, lancia backtest, vedi KPI + equity sparkline + trades, genera report HTML MT5-style self-contained.`,
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			// if no tty, fallback to textual
			m := tui.New(cfg, cfgPath)
			p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
			if _, err := p.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "tui error %v\n", err)
				os.Exit(1)
			}
		},
	}
}

// inject tui command into root — call from init
func init() {
	// This file is in package main, but root is defined in main.go.
	// We need to add command at runtime. We'll patch main.go to include cmdTUI.
}
