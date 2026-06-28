package cmd

import (
	"fmt"
	"os"

	"github.com/hsanson/go-khal/internal/config"
	"github.com/spf13/cobra"
)

var cfgPath string

var rootCmd = &cobra.Command{
	Use:   "go-khal",
	Short: "Terminal calendar and todo manager for vdirsyncer data",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath(), "path to config file")
	rootCmd.AddCommand(newTUICommand())
	rootCmd.AddCommand(newAgendaCommand())
	rootCmd.AddCommand(newDayCommand())
	rootCmd.AddCommand(newWeekCommand())
	rootCmd.AddCommand(newMonthCommand())
	rootCmd.AddCommand(newYearCommand())
	rootCmd.AddCommand(newTodoCommand())
	rootCmd.AddCommand(newConfigCommand())
}
