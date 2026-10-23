package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/hsanson/go-khal/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgPath string
	version = "devel"
)

var rootCmd = &cobra.Command{
	Use:   "go-khal",
	Short: "Terminal calendar and todo manager for vdirsyncer data",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath(), "path to config file")
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newTUICommand())
	rootCmd.AddCommand(newAgendaCommand())
	rootCmd.AddCommand(newListCommand())
	rootCmd.AddCommand(newPrintFormatsCommand())
	rootCmd.AddCommand(newImportCommand())
	rootCmd.AddCommand(newTodoCommand())
	rootCmd.AddCommand(newConfigCommand())
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "go-khal %s (%s)\n", version, runtime.Version())
		},
	}
}
