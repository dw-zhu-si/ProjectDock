package projects

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"projectdock/internal/model"
	"projectdock/internal/ports"
	"projectdock/internal/store"
)

type noListeners struct{}

func (noListeners) List(context.Context) ([]model.PortListener, error) {
	return []model.PortListener{}, nil
}

type fixedListeners []model.PortListener

func (listeners fixedListeners) List(context.Context) ([]model.PortListener, error) {
	return listeners, nil
}

type oneShotListeners struct {
	mu       sync.Mutex
	listener model.PortListener
	used     bool
}

func (listeners *oneShotListeners) List(context.Context) ([]model.PortListener, error) {
	listeners.mu.Lock()
	defer listeners.mu.Unlock()
	if listeners.used {
		return []model.PortListener{}, nil
	}
	listeners.used = true
	return []model.PortListener{listeners.listener}, nil
}

func TestManagedProcessLifecycle(t *testing.T) {
	st := store.New(t.TempDir())
	portService := ports.NewService(st, noListeners{})
	service := NewService(st, portService)
	project, err := service.Upsert(context.Background(), model.Project{
		ID: "managed-demo", Name: "Managed demo", Path: t.TempDir(), Source: "codex",
		StartCommand: "echo ready; sleep 30", Ports: []int{},
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.Start(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.State != "running" || started.PID < 1 {
		t.Fatalf("unexpected start status: %#v", started)
	}
	logDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(logDeadline) {
		lines, readErr := service.ReadLog(project.ID, 20)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(lines) > 0 && lines[0] == "ready" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	stopped, err := service.Stop(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != "stopped" {
		t.Fatalf("expected stopped state, got %#v", stopped)
	}
	lines, err := service.ReadLog(project.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 || lines[0] != "ready" {
		t.Fatalf("expected managed log output, got %#v", lines)
	}
}

func TestListMarksAllocatedListeningProjectAsExternal(t *testing.T) {
	ctx := context.Background()
	st := store.New(t.TempDir())
	projectPath := t.TempDir()
	_, err := st.Update(ctx, func(registry *model.Registry) error {
		registry.Projects = append(registry.Projects, model.NormalizeProject(model.Project{
			ID: "external-demo", Name: "External demo", Path: projectPath, Source: "codex",
			StartCommand: "make run", StopCommand: "make stop", Ports: []int{4310},
		}, time.Now()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, ports.NewService(st, fixedListeners{
		{Port: 4310, PID: 86427, Process: "com.docker.backend", Address: "127.0.0.1:4310"},
	}))
	views, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("expected one project view, got %#v", views)
	}
	view := views[0]
	if view.Run.State != "external" || view.Run.PID != 86427 {
		t.Fatalf("expected external listener state, got %#v", view.Run)
	}
	if !view.ConfiguredToStart || view.ReadyToStart || !view.CanStop {
		t.Fatalf("unexpected lifecycle flags: %#v", view)
	}
}

func TestVisibleInControlPanelHidesRegistrationOnlyArchivedAndUnavailableProjects(t *testing.T) {
	views := []ProjectView{
		{Project: model.Project{ID: "ready", LaunchSource: "manual"}, PathAvailable: true, ConfiguredToStart: true},
		{Project: model.Project{ID: "registered"}, PathAvailable: true},
		{Project: model.Project{ID: "archived", LaunchSource: "archived"}, PathAvailable: true, ConfiguredToStart: true},
		{Project: model.Project{ID: "unavailable", LaunchSource: "manual"}, ConfiguredToStart: true},
		{Project: model.Project{ID: "stoppable"}, PathAvailable: true, CanStop: true},
	}
	visible := VisibleInControlPanel(views)
	if len(visible) != 2 || visible[0].ID != "ready" || visible[1].ID != "stoppable" {
		t.Fatalf("unexpected visible projects: %#v", visible)
	}
}

func TestListMarksAmbiguousDetectedListenerAsConflict(t *testing.T) {
	ctx := context.Background()
	st := store.New(t.TempDir())
	firstPath := t.TempDir()
	secondPath := t.TempDir()
	_, err := st.Update(ctx, func(registry *model.Registry) error {
		for index, projectPath := range []string{firstPath, secondPath} {
			registry.Projects = append(registry.Projects, model.NormalizeProject(model.Project{
				ID: "shared-port-" + strconv.Itoa(index), Name: "Shared", Path: projectPath, Source: "codex",
				StartCommand: "npm run dev", LaunchSource: "manual", LaunchPorts: []int{5173},
			}, time.Now()))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, ports.NewService(st, fixedListeners{{Port: 5173, PID: 43199}}))
	views, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.Run.State != "conflict" {
			t.Fatalf("ambiguous launch-port listener must not be claimed as external: %#v", view)
		}
	}
}

func TestStartExplainsExternalProjectInsteadOfGenericPortConflict(t *testing.T) {
	ctx := context.Background()
	st := store.New(t.TempDir())
	projectPath := t.TempDir()
	_, err := st.Update(ctx, func(registry *model.Registry) error {
		registry.Projects = append(registry.Projects, model.NormalizeProject(model.Project{
			ID: "external-demo", Name: "External demo", Path: projectPath, Source: "codex",
			StartCommand: "make run", Ports: []int{4310},
		}, time.Now()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, ports.NewService(st, fixedListeners{{Port: 4310, PID: 86427}}))
	_, err = service.Start(ctx, "external-demo")
	if err == nil || !strings.Contains(err.Error(), "外部运行") {
		t.Fatalf("expected external-running explanation, got %v", err)
	}
}

func TestStopExternalUsesRegisteredCommandWithoutKillingListenerPID(t *testing.T) {
	ctx := context.Background()
	st := store.New(t.TempDir())
	projectPath := t.TempDir()
	marker := filepath.Join(projectPath, "stopped.marker")
	_, err := st.Update(ctx, func(registry *model.Registry) error {
		registry.Projects = append(registry.Projects, model.NormalizeProject(model.Project{
			ID: "external-demo", Name: "External demo", Path: projectPath, Source: "codex",
			StartCommand: "make run", StopCommand: "printf stopped > " + strconv.Quote(marker),
			LaunchSource: "manual", Ports: []int{4310},
		}, time.Now()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	scanner := &oneShotListeners{listener: model.PortListener{Port: 4310, PID: 99999}}
	service := NewService(st, ports.NewService(st, scanner))
	stopped, err := service.Stop(ctx, "external-demo")
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != "stopped" {
		t.Fatalf("expected stopped state, got %#v", stopped)
	}
	if data, readErr := os.ReadFile(marker); readErr != nil || string(data) != "stopped" {
		t.Fatalf("registered stop command did not run safely: data=%q err=%v", data, readErr)
	}
}

func TestSyncAutoDetectsPackageLifecycle(t *testing.T) {
	ctx := context.Background()
	st := store.New(t.TempDir())
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "package.json"), []byte(`{
		"scripts": {"dev": "vite --host 127.0.0.1 --port 6123"}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "pnpm-lock.yaml"), []byte("lockfileVersion: '9.0'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(st, ports.NewService(st, noListeners{}))
	result, err := service.SyncPath(ctx, SyncInput{Path: projectPath, Name: "Vite app", Source: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	project := result.Project
	if project.StartCommand != "pnpm run dev" || project.LaunchSource != "package" {
		t.Fatalf("package lifecycle was not detected: %#v", project)
	}
	if len(project.LaunchPorts) != 1 || project.LaunchPorts[0] != 6123 {
		t.Fatalf("expected detected port 6123, got %#v", project.LaunchPorts)
	}
}

func TestSyncAutoDetectsComposeLifecycle(t *testing.T) {
	ctx := context.Background()
	st := store.New(t.TempDir())
	projectPath := t.TempDir()
	compose := "services:\n  app:\n    image: demo\n    ports:\n      - \"127.0.0.1:${APP_PORT:-4175}:4175\"\n"
	if err := os.WriteFile(filepath.Join(projectPath, "compose.yml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(st, ports.NewService(st, noListeners{}))
	result, err := service.SyncPath(ctx, SyncInput{Path: projectPath, Name: "Compose app", Source: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	project := result.Project
	if project.StartCommand != "docker compose -f compose.yml up --build" ||
		project.StopCommand != "docker compose -f compose.yml stop" ||
		project.LaunchSource != "compose" {
		t.Fatalf("compose lifecycle was not detected: %#v", project)
	}
	if len(project.LaunchPorts) != 1 || project.LaunchPorts[0] != 4175 {
		t.Fatalf("expected detected compose port, got %#v", project.LaunchPorts)
	}
}

func TestSyncDoesNotAutoConfigureArchivedProject(t *testing.T) {
	ctx := context.Background()
	st := store.New(t.TempDir())
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(st, ports.NewService(st, noListeners{}))
	result, err := service.SyncPath(ctx, SyncInput{
		Path: projectPath, Name: "Archived", Source: "tri-agent",
		ProjectCard: "0-项目目录/项目档案/Archived/Archived.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.StartCommand != "" || result.Project.LaunchSource != "archived" {
		t.Fatalf("archived project should remain registry-only: %#v", result.Project)
	}
}

func TestDeleteProjectCanRemoveExactRegisteredDirectoryAfterNameConfirmation(t *testing.T) {
	ctx := context.Background()
	st := store.New(t.TempDir())
	service := NewService(st, ports.NewService(st, noListeners{}))
	projectPath := filepath.Join(t.TempDir(), "owner", "delete-me")
	if err := os.MkdirAll(projectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := service.SyncPath(ctx, SyncInput{Path: projectPath, Name: "Delete me", Source: "manual", Revive: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteProject(ctx, result.Project.ID, true, "wrong"); err == nil {
		t.Fatal("destructive delete accepted the wrong confirmation")
	}
	if _, err := os.Stat(result.Project.Path); err != nil {
		t.Fatalf("wrong confirmation changed project directory: %v", err)
	}
	deleted, err := service.DeleteProject(ctx, result.Project.ID, true, "Delete me")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted.FilesDeleted || !deleted.RegistrationDeleted {
		t.Fatalf("unexpected delete result: %#v", deleted)
	}
	if _, err := os.Stat(result.Project.Path); !os.IsNotExist(err) {
		t.Fatalf("project directory still exists: %v", err)
	}
}
