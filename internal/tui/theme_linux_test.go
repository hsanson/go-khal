//go:build linux

package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThemeMonitorReloadsAndRecovers(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "current")
	themeDir := filepath.Join(stateDir, "theme")
	colorsPath := filepath.Join(themeDir, "colors.toml")
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(colorsPath, []byte(testOmarchyPalette("#123456")), 0o644); err != nil {
		t.Fatal(err)
	}

	monitor, initial := newThemeMonitorAt(stateDir)
	defer monitor.shutdown()
	if got := styleForeground(initial.styles.Accent); got != "#123456" {
		t.Fatalf("initial accent = %q, want #123456", got)
	}
	if err := monitor.start(); err != nil {
		t.Fatalf("start monitor: %v", err)
	}

	updated := themeAfterChange(t, monitor, func() {
		if err := os.WriteFile(colorsPath, []byte(testOmarchyPalette("#654321")), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if got := styleForeground(updated.styles.Accent); got != "#654321" {
		t.Fatalf("updated accent = %q, want #654321", got)
	}
	repaired := make(chan themeStyles, 1)
	go func() {
		msg, ok := monitor.wait()().(themeStylesMsg)
		if ok {
			repaired <- msg.theme
		}
	}()
	if err := os.WriteFile(colorsPath, []byte("accent = \"invalid\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repaired:
		t.Fatal("invalid palette replaced the last valid theme")
	case <-time.After(50 * time.Millisecond):
	}
	if err := os.WriteFile(colorsPath, []byte(testOmarchyPalette("#777777")), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case theme := <-repaired:
		if got := styleForeground(theme.styles.Accent); got != "#777777" {
			t.Fatalf("repaired accent = %q, want #777777", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for repaired palette")
	}

	fallback := themeAfterChange(t, monitor, func() {
		if err := os.Remove(colorsPath); err != nil {
			t.Fatal(err)
		}
	})
	if got := styleForeground(fallback.styles.Accent); got != "117" {
		t.Fatalf("fallback accent = %q, want 117", got)
	}

	recreated := themeAfterChange(t, monitor, func() {
		if err := os.WriteFile(colorsPath, []byte(testOmarchyPalette("#abcdef")), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if got := styleForeground(recreated.styles.Accent); got != "#abcdef" {
		t.Fatalf("recreated accent = %q, want #abcdef", got)
	}
	stopped := make(chan struct{})
	go func() {
		_ = monitor.wait()()
		close(stopped)
	}()
	monitor.close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("theme monitor did not stop")
	}
}

func themeAfterChange(t *testing.T, monitor *themeMonitor, change func()) themeStyles {
	t.Helper()
	result := make(chan themeStyles, 1)
	failure := make(chan string, 1)
	go func() {
		msg, ok := monitor.wait()().(themeStylesMsg)
		if !ok {
			failure <- "monitor stopped without a theme update"
			return
		}
		result <- msg.theme
	}()
	change()
	select {
	case theme := <-result:
		return theme
	case message := <-failure:
		t.Fatal(message)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for theme update")
	}
	return themeStyles{}
}
