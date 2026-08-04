package widget

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"projectdock/internal/model"
	"projectdock/internal/ports"
	"projectdock/internal/projects"
)

func TestBuildCountsUniquePortsAndRedactsProjectDetails(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	views := []projects.ProjectView{
		{
			Project: model.Project{
				ID:           "secret-id",
				Name:         "Alpha",
				Path:         "/private/secret/project",
				Source:       "codex",
				StartCommand: "SECRET_COMMAND",
				HealthURL:    "http://127.0.0.1:9999/secret",
				Ports:        []int{3000},
			},
			Run:               model.RunStatus{State: "running", PID: 4242, LogPath: "/private/secret.log"},
			PathAvailable:     true,
			ConfiguredToStart: true,
			ReadyToStart:      true,
		},
		{
			Project:       model.Project{Name: "Beta", Source: "trae"},
			Run:           model.RunStatus{State: "stopped"},
			PathAvailable: true,
			ReadyToStart:  false,
		},
	}
	listeners := []model.PortListener{
		{Port: 3000, PID: 1},
		{Port: 3000, PID: 1},
		{Port: 43110, PID: 2},
	}
	allocations := []ports.PortAllocation{
		{Port: 3000, ProjectID: "secret-id", ProjectName: "Alpha", State: "active"},
	}
	snapshot := Build(views, listeners, allocations, []model.PortReservation{{Port: 5173}}, now)
	if snapshot.ProjectCount != 2 || snapshot.RunningCount != 1 || snapshot.RegisteredOnlyCount != 1 {
		t.Fatalf("unexpected project counts: %#v", snapshot)
	}
	if snapshot.ListeningPortCount != 2 || snapshot.AllocatedPortCount != 1 ||
		snapshot.ActiveAllocationCount != 1 || snapshot.TemporaryReservationCount != 1 {
		t.Fatalf("unexpected port counts: %#v", snapshot)
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, secret := range []string{
		"secret-id", "/private/secret/project", "SECRET_COMMAND",
		"http://127.0.0.1:9999/secret", "4242", "/private/secret.log",
	} {
		if strings.Contains(text, secret) {
			t.Fatalf("snapshot leaked %q: %s", secret, text)
		}
	}
}

func TestBuildLimitsAndRanksProjectSummaries(t *testing.T) {
	views := []projects.ProjectView{
		{Project: model.Project{Name: "Ready", Source: "manual"}, Run: model.RunStatus{State: "stopped"}, PathAvailable: true, ConfiguredToStart: true, ReadyToStart: true},
		{Project: model.Project{Name: "Missing", Source: "claude"}, Run: model.RunStatus{State: "stopped"}},
		{Project: model.Project{Name: "Needs config", Source: "trae"}, Run: model.RunStatus{State: "stopped"}, PathAvailable: true},
		{Project: model.Project{Name: "Running", Source: "codex"}, Run: model.RunStatus{State: "starting"}, PathAvailable: true, ConfiguredToStart: true, ReadyToStart: true},
	}
	snapshot := Build(views, nil, nil, nil, time.Now())
	if len(snapshot.Projects) != 4 {
		t.Fatalf("expected four summaries, got %#v", snapshot.Projects)
	}
	got := []string{snapshot.Projects[0].State, snapshot.Projects[1].State, snapshot.Projects[2].State, snapshot.Projects[3].State}
	want := []string{"running", "unavailable", "ready", "registered"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected rank order: got %#v want %#v", got, want)
		}
	}
}
