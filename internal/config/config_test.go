package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectRegistryPathIsOptIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PROJECTDOCK_PROJECTS_FILE", "")
	if got := ProjectRegistryPath(); got != "" {
		t.Fatalf("empty opt-in must disable external registry, got %q", got)
	}

	t.Setenv("PROJECTDOCK_PROJECTS_FILE", filepath.Join(home, "projects.json"))
	want, err := filepath.Abs(filepath.Join(home, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := ProjectRegistryPath(); got != want {
		t.Fatalf("explicit registry path mismatch: got %q want %q", got, want)
	}

	if err := os.Unsetenv("PROJECTDOCK_PROJECTS_FILE"); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRegistryPath(); got != "" {
		t.Fatalf("external registry must be disabled by default, got %q", got)
	}
}
