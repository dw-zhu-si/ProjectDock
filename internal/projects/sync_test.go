package projects

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"projectdock/internal/model"
	"projectdock/internal/ports"
	"projectdock/internal/store"
)

func TestSyncPathIsIdempotentAndDeleteCreatesIgnore(t *testing.T) {
	st := store.New(t.TempDir())
	service := NewService(st, ports.NewService(st, noListeners{}))
	projectPath := t.TempDir()

	first, err := service.SyncPath(context.Background(), SyncInput{
		Path: projectPath, Name: "同步项目", Source: "codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.SyncPath(context.Background(), SyncInput{
		Path: projectPath, Name: "同步项目", Source: "trae",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Project.ID != second.Project.ID || second.Action != "updated" {
		t.Fatalf("expected stable idempotent update: first=%#v second=%#v", first, second)
	}
	if err := service.Delete(context.Background(), first.Project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SyncPath(context.Background(), SyncInput{Path: projectPath, Source: "codex"}); !errors.Is(err, ErrProjectIgnored) {
		t.Fatalf("expected ignored sync after delete, got %v", err)
	}
	revived, err := service.SyncPath(context.Background(), SyncInput{
		Path: projectPath, Source: "manual", Revive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if revived.Project.SyncMode != "manual" {
		t.Fatalf("expected manual revive, got %#v", revived.Project)
	}
}

func TestSyncRegistrySkipsMissingPaths(t *testing.T) {
	st := store.New(t.TempDir())
	service := NewService(st, ports.NewService(st, noListeners{}))
	existing := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	payload := map[string]any{
		"version": 1,
		"vault":   t.TempDir(),
		"projects": []map[string]string{
			{"name": "可访问", "path": existing, "card": "card.md"},
			{"name": "缺失", "path": filepath.Join(t.TempDir(), "missing"), "card": "missing.md"},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := service.SyncRegistry(context.Background(), registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Added != 1 || report.Skipped != 1 {
		t.Fatalf("unexpected sync report: %#v", report)
	}
}

func TestSyncPathIgnoresUnrelatedOfflineProject(t *testing.T) {
	st := store.New(t.TempDir())
	offlinePath := filepath.Join(t.TempDir(), "offline-volume", "project")
	_, err := st.Update(context.Background(), func(registry *model.Registry) error {
		registry.Projects = append(registry.Projects, model.Project{
			ID: "offline-project", Name: "Offline", Path: offlinePath,
			Source: "manual", SyncMode: "manual", StartCommand: "npm start",
			LaunchSource: "package",
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	service := NewService(st, ports.NewService(st, noListeners{}))
	result, err := service.SyncPath(context.Background(), SyncInput{
		Path: t.TempDir(), Name: "在线项目", Source: "codex",
	})
	if err != nil {
		t.Fatalf("offline project should not break an unrelated sync: %v", err)
	}
	if result.Project.ID == "" {
		t.Fatal("online project was not registered")
	}
	offline, err := service.Get(context.Background(), "offline-project")
	if err != nil {
		t.Fatal(err)
	}
	if offline.StartCommand != "npm start" || offline.LaunchSource != "package" {
		t.Fatalf("offline launch profile should be preserved: %#v", offline)
	}
}

func TestStartRejectsProjectWithoutCommand(t *testing.T) {
	st := store.New(t.TempDir())
	service := NewService(st, ports.NewService(st, noListeners{}))
	synced, err := service.SyncPath(context.Background(), SyncInput{
		Path: t.TempDir(), Source: "codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(context.Background(), synced.Project.ID); err == nil {
		t.Fatal("expected project without start command to be rejected")
	}
}
