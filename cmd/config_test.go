package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourcesFromVdirsyncerDiscoversConcreteFolders(t *testing.T) {
	root := t.TempDir()
	calRoot := filepath.Join(root, "calendars")
	personalCal := filepath.Join(calRoot, "personal")
	workCal := filepath.Join(calRoot, "work")
	emptyCal := filepath.Join(calRoot, "empty")
	addressBook := filepath.Join(root, "contacts", "personal")

	for _, dir := range []string{personalCal, workCal, emptyCal, addressBook} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(personalCal, "event.ics"), []byte("BEGIN:VCALENDAR\nEND:VCALENDAR\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workCal, "event.ics"), []byte("BEGIN:VCALENDAR\nEND:VCALENDAR\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addressBook, "person.vcf"), []byte("BEGIN:VCARD\nEND:VCARD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workCal, "displayname"), []byte("Work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workCal, "color"), []byte("#00aa00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyCal, "displayname"), []byte("Empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(root, "vdirsyncer.conf")
	configData := `[storage calendars]
type = "filesystem"
path = "` + calRoot + `"
fileext = ".ics"

[storage contacts]
type = "filesystem"
path = "` + filepath.Dir(addressBook) + `"
fileext = ".vcf"
`
	if err := os.WriteFile(configPath, []byte(configData), 0o644); err != nil {
		t.Fatal(err)
	}

	sources, err := sourcesFromVdirsyncer(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(sources), 4; got != want {
		t.Fatalf("source count = %d, want %d: %#v", got, want, sources)
	}

	byPath := map[string]string{}
	for _, src := range sources {
		byPath[src.Path] = src.Type
	}
	if byPath[personalCal] != "calendar" {
		t.Fatalf("personal calendar source missing: %#v", sources)
	}
	if byPath[workCal] != "calendar" {
		t.Fatalf("work calendar source missing: %#v", sources)
	}
	if byPath[emptyCal] != "calendar" {
		t.Fatalf("empty calendar source missing: %#v", sources)
	}
	if byPath[addressBook] != "addressbook" {
		t.Fatalf("addressbook source missing: %#v", sources)
	}
}

func TestAbsolutePathRejectsRelativePath(t *testing.T) {
	if _, err := absolutePath("relative/calendar"); err == nil {
		t.Fatal("expected relative path to fail")
	}
}
