package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"projectdock/internal/model"
)

func TestStoreRoundTripAndPermissions(t *testing.T) {
	st := New(t.TempDir())
	_, err := st.Update(context.Background(), func(registry *model.Registry) error {
		registry.Projects = append(registry.Projects, model.Project{ID: "demo-app", Name: "Demo"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := st.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Projects) != 1 || registry.Projects[0].ID != "demo-app" {
		t.Fatalf("unexpected registry: %#v", registry)
	}
	info, err := os.Stat(filepath.Join(st.Dir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestStoreMigratesVersionOneRegistry(t *testing.T) {
	dir := t.TempDir()
	payload := `{"version":1,"projects":[{"id":"demo-app","name":"Demo","path":"/tmp/demo","source":"codex","startCommand":"","ports":[],"createdAt":"2026-07-20T00:00:00Z","updatedAt":"2026-07-20T00:00:00Z"}],"reservations":[],"audit":[]}`
	if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := New(dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version != model.RegistryVersion || registry.Projects[0].SyncMode != "manual" {
		t.Fatalf("version one registry was not migrated: %#v", registry)
	}
}

func TestStoreMigratesVersionTwoAndDropsRedundantTemporaryReservation(t *testing.T) {
	dir := t.TempDir()
	payload := `{"version":2,"projects":[{"id":"demo-app","name":"Demo","path":"/tmp/demo","source":"codex","syncMode":"manual","startCommand":"","ports":[5173],"createdAt":"2026-07-20T00:00:00Z","updatedAt":"2026-07-20T00:00:00Z"}],"reservations":[{"port":5173,"projectId":"demo-app","owner":"projectdock","createdAt":"2026-07-20T00:00:00Z","expiresAt":"2099-07-20T01:00:00Z"}],"audit":[],"ignoredPaths":[]}`
	if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := New(dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version != model.RegistryVersion || len(registry.Reservations) != 0 || len(registry.Projects[0].Ports) != 1 {
		t.Fatalf("version two registry was not migrated safely: %#v", registry)
	}
}

func TestStoreMigratesVersionThreeLifecycleFields(t *testing.T) {
	dir := t.TempDir()
	payload := `{"version":3,"projects":[{"id":"demo-app","name":"Demo","path":"/tmp/demo","source":"codex","syncMode":"manual","startCommand":"npm run dev","ports":[5173],"createdAt":"2026-07-20T00:00:00Z","updatedAt":"2026-07-20T00:00:00Z"}],"reservations":[],"audit":[],"ignoredPaths":[]}`
	if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := New(dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	project := registry.Projects[0]
	if registry.Version != model.RegistryVersion || project.StartCommand != "npm run dev" {
		t.Fatalf("version three registry was not migrated safely: %#v", registry)
	}
}

func TestStoreMigratesVersionFourAndDeduplicatesProjectPaths(t *testing.T) {
	dir := t.TempDir()
	payload := `{"version":4,"projects":[{"id":"manual-copy","name":"Manual","path":"/tmp/demo","source":"codex","syncMode":"manual","discoveredBy":"codex","startCommand":"","ports":[4310,4311],"createdAt":"2026-07-19T00:00:00Z","updatedAt":"2026-07-19T00:00:00Z"},{"id":"auto-copy","name":"Auto","path":"/tmp/demo","source":"tri-agent","syncMode":"auto","discoveredBy":"tri-agent-registry","startCommand":"docker compose up","stopCommand":"docker compose stop","launchSource":"compose","launchPorts":[4310,4311],"ports":[],"createdAt":"2026-07-20T00:00:00Z","updatedAt":"2026-07-20T00:00:00Z"}],"reservations":[{"port":4312,"projectId":"manual-copy","owner":"projectdock","createdAt":"2026-07-20T00:00:00Z","expiresAt":"2099-07-20T01:00:00Z"}],"audit":[],"ignoredPaths":[]}`
	if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := New(dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version != model.RegistryVersion || len(registry.Projects) != 1 {
		t.Fatalf("version four registry was not deduplicated: %#v", registry)
	}
	project := registry.Projects[0]
	if project.ID != "auto-copy" || project.StartCommand == "" || len(project.Ports) != 2 {
		t.Fatalf("expected lifecycle-capable project with merged allocations: %#v", project)
	}
	if len(registry.Reservations) != 1 || registry.Reservations[0].ProjectID != "auto-copy" {
		t.Fatalf("expected reservations to follow the surviving project: %#v", registry.Reservations)
	}
}
