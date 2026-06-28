package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/tui"
	"github.com/spf13/cobra"
)

func newTodoCommand() *cobra.Command {
	todoCmd := &cobra.Command{
		Use:   "todo",
		Short: "Manage VTODO entries",
	}
	todoCmd.AddCommand(newTodoListCommand())
	todoCmd.AddCommand(newTodoShowCommand())
	todoCmd.AddCommand(newTodoNewCommand())
	todoCmd.AddCommand(newTodoEditCommand())
	return todoCmd
}

func newTodoListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List todos",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _, ds, err := loadStore()
			if err != nil {
				return err
			}
			fmt.Println(tui.RenderTodos(calendar.FilterVisibleTodos(ds.Todos), tui.DefaultStyles()))
			return nil
		},
	}
}

func newTodoShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <uid>",
		Short: "Show details for one todo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, _, err := loadStore()
			if err != nil {
				return err
			}
			t, err := store.FindTodo(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("UID: %s\nSummary: %s\nStatus: %s\nPercent: %d\nSource: %s\nFile: %s\n", t.UID, t.Summary, t.Status, t.Percent, t.Source, t.FilePath)
			if t.Start != nil {
				fmt.Printf("Start: %s\n", t.Start.Format(time.RFC3339))
			}
			if t.Due != nil {
				fmt.Printf("Due: %s\n", t.Due.Format(time.RFC3339))
			}
			if t.Description != "" {
				fmt.Printf("Description:\n%s\n", t.Description)
			}
			return nil
		},
	}
}

func newTodoNewCommand() *cobra.Command {
	var sourceName string
	var calendarName string
	var summary string
	var description string
	var dueStr string
	var startStr string
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new VTODO",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, store, _, err := loadStore()
			if err != nil {
				return err
			}
			if sourceName == "" && len(cfg.Sources) > 0 {
				sourceName = cfg.Sources[0].Name
			}

			if !nonInteractive && strings.TrimSpace(summary) == "" {
				if err := runTodoCreateForm(&sourceName, &calendarName, &summary, &description, &startStr, &dueStr); err != nil {
					return err
				}
			}

			if strings.TrimSpace(summary) == "" {
				quick, err := runQuickSummaryPrompt()
				if err != nil {
					return err
				}
				summary = quick
			}

			start, err := parseOptionalDateTime(startStr)
			if err != nil {
				return fmt.Errorf("invalid --start: %w", err)
			}
			due, err := parseOptionalDateTime(dueStr)
			if err != nil {
				return fmt.Errorf("invalid --due: %w", err)
			}

			if err := store.CreateTodo(sourceName, calendarName, calendar.Todo{
				Summary:     summary,
				Description: description,
				Status:      "NEEDS-ACTION",
				Start:       start,
				Due:         due,
			}); err != nil {
				return err
			}
			fmt.Println("todo created")
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "source name")
	cmd.Flags().StringVar(&calendarName, "calendar", "", "calendar name inside source")
	cmd.Flags().StringVar(&summary, "summary", "", "todo summary")
	cmd.Flags().StringVar(&description, "description", "", "todo description")
	cmd.Flags().StringVar(&startStr, "start", "", "start datetime (RFC3339 or YYYY-MM-DD HH:MM)")
	cmd.Flags().StringVar(&dueStr, "due", "", "due datetime (RFC3339 or YYYY-MM-DD HH:MM)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "disable forms")
	return cmd
}

func newTodoEditCommand() *cobra.Command {
	var summary string
	var description string
	var status string
	var startStr string
	var dueStr string
	var percent int
	var form bool

	cmd := &cobra.Command{
		Use:   "edit <uid>",
		Short: "Edit an existing VTODO",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := args[0]
			_, store, _, err := loadStore()
			if err != nil {
				return err
			}

			todo, err := store.FindTodo(uid)
			if err != nil {
				return err
			}

			if form {
				if err := runTodoEditForm(&summary, &description, &status, &startStr, &dueStr, &percent, todo); err != nil {
					return err
				}
			}

			var update calendar.TodoUpdate
			if summary != "" {
				update.Summary = &summary
			}
			if description != "" {
				update.Description = &description
			}
			if status != "" {
				update.Status = &status
			}
			if cmd.Flags().Changed("percent") {
				update.Percent = &percent
			}
			if startStr != "" {
				start, err := parseOptionalDateTime(startStr)
				if err != nil {
					return fmt.Errorf("invalid --start: %w", err)
				}
				update.Start = &start
			}
			if dueStr != "" {
				due, err := parseOptionalDateTime(dueStr)
				if err != nil {
					return fmt.Errorf("invalid --due: %w", err)
				}
				update.Due = &due
			}

			if err := store.UpdateTodo(uid, update); err != nil {
				return err
			}
			fmt.Println("todo updated")
			return nil
		},
	}

	cmd.Flags().StringVar(&summary, "summary", "", "new summary")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringVar(&status, "status", "", "new status (NEEDS-ACTION/IN-PROCESS/COMPLETED)")
	cmd.Flags().StringVar(&startStr, "start", "", "new start datetime")
	cmd.Flags().StringVar(&dueStr, "due", "", "new due datetime")
	cmd.Flags().IntVar(&percent, "percent", 0, "new percent complete")
	cmd.Flags().BoolVar(&form, "form", false, "open interactive edit form")
	return cmd
}

func runTodoCreateForm(sourceName, calendarName, summary, description, startStr, dueStr *string) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Source").Value(sourceName),
			huh.NewInput().Title("Calendar").Value(calendarName),
			huh.NewInput().Title("Summary").Value(summary).Validate(huh.ValidateNotEmpty()),
			huh.NewText().Title("Description").Value(description),
			huh.NewInput().Title("Start (RFC3339 or YYYY-MM-DD HH:MM)").Value(startStr),
			huh.NewInput().Title("Due (RFC3339 or YYYY-MM-DD HH:MM)").Value(dueStr),
		),
	)
	return form.Run()
}

func runTodoEditForm(summary, description, status, startStr, dueStr *string, percent *int, existing calendar.Todo) error {
	if *summary == "" {
		*summary = existing.Summary
	}
	if *description == "" {
		*description = existing.Description
	}
	if *status == "" {
		*status = existing.Status
	}
	if existing.Start != nil && *startStr == "" {
		*startStr = existing.Start.Format(time.RFC3339)
	}
	if existing.Due != nil && *dueStr == "" {
		*dueStr = existing.Due.Format(time.RFC3339)
	}
	if *percent == 0 {
		*percent = existing.Percent
	}

	percentStr := strconv.Itoa(*percent)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Summary").Value(summary),
			huh.NewText().Title("Description").Value(description),
			huh.NewSelect[string]().Title("Status").
				Options(
					huh.NewOption("NEEDS-ACTION", "NEEDS-ACTION"),
					huh.NewOption("IN-PROCESS", "IN-PROCESS"),
					huh.NewOption("COMPLETED", "COMPLETED"),
				).Value(status),
			huh.NewInput().Title("Start").Value(startStr),
			huh.NewInput().Title("Due").Value(dueStr),
			huh.NewInput().Title("Percent complete").Value(&percentStr),
		),
	)
	if err := form.Run(); err != nil {
		return err
	}
	if parsed, err := strconv.Atoi(percentStr); err == nil {
		*percent = parsed
	}
	return nil
}

type summaryModel struct {
	input textinput.Model
	done  bool
	err   error

	summary string
}

func runQuickSummaryPrompt() (string, error) {
	t := textinput.New()
	t.Placeholder = "Todo summary"
	t.Focus()
	t.CharLimit = 200
	t.Width = 40

	m := summaryModel{input: t}
	p, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	model := p.(summaryModel)
	if model.err != nil {
		return "", model.err
	}
	if strings.TrimSpace(model.summary) == "" {
		return "", fmt.Errorf("summary cannot be empty")
	}
	return model.summary, nil
}

func (m summaryModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m summaryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.err = fmt.Errorf("cancelled")
			return m, tea.Quit
		case tea.KeyEnter:
			m.summary = m.input.Value()
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m summaryModel) View() string {
	return "New todo summary:\n" + m.input.View() + "\n(enter to confirm, esc to cancel)"
}
