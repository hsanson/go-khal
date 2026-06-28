package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/tui"
	"github.com/spf13/cobra"
)

func newTUICommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch full-screen terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, ds, err := loadStore()
			if err != nil {
				return err
			}
			model := tui.NewModel(cfg, ds)
			if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
				return fmt.Errorf("run tui: %w", err)
			}
			return nil
		},
	}
	return cmd
}
