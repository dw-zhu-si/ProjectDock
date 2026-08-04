package projects

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"projectdock/internal/ports"
	"projectdock/internal/store"
)

func TestScanFindsManageableProjectsAndSkipsDependencies(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "apps", "demo")
	ignoredPath := filepath.Join(root, "node_modules", "dependency")
	for _, path := range []string{appPath, ignoredPath} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	st := store.New(t.TempDir())
	service := NewService(st, ports.NewService(st, noListeners{}))
	report, err := service.Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedAppPath, _ := filepath.EvalSymlinks(appPath)
	if len(report.Candidates) != 1 || report.Candidates[0].Path != resolvedAppPath || !report.Candidates[0].Manageable {
		t.Fatalf("unexpected scan report: %#v", report)
	}
}

func TestScanContinuesIntoUnmanageableMonorepoRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "apps", "web")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st := store.New(t.TempDir())
	service := NewService(st, ports.NewService(st, noListeners{}))
	report, err := service.Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 2 || report.Candidates[0].Name != "web" || !report.Candidates[0].Manageable {
		t.Fatalf("scan did not discover manageable monorepo child: %#v", report)
	}
}
