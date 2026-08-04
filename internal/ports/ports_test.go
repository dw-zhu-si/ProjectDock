package ports

import (
	"context"
	"testing"
	"time"

	"projectdock/internal/model"
	"projectdock/internal/store"
)

func TestParseLsof(t *testing.T) {
	input := []byte("p101\ncnode\nnTCP 127.0.0.1:5173\np202\ncpostgres\nnTCP *:5432\n")
	got, err := ParseLsof(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 listeners, got %d", len(got))
	}
	if got[0].Port != 5173 || got[0].PID != 101 || got[0].Process != "node" {
		t.Fatalf("unexpected first listener: %#v", got[0])
	}
	if got[1].Port != 5432 || got[1].PID != 202 {
		t.Fatalf("unexpected second listener: %#v", got[1])
	}
}

type fakeScanner struct {
	listeners []model.PortListener
}

func (f fakeScanner) List(context.Context) ([]model.PortListener, error) {
	return f.listeners, nil
}

func TestReservationConflict(t *testing.T) {
	data := t.TempDir()
	st := store.New(data)
	service := NewService(st, fakeScanner{})
	// Use a live-relative timestamp because Store intentionally expires
	// reservations against wall-clock time while loading the registry.
	now := time.Now().UTC().Truncate(time.Second)
	service.now = func() time.Time { return now }

	_, err := service.Reserve(context.Background(), model.PortReservation{
		Port:      5173,
		ProjectID: "alpha",
		Owner:     "codex",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Reserve(context.Background(), model.PortReservation{
		Port:      5173,
		ProjectID: "beta",
		Owner:     "claude",
		ExpiresAt: now.Add(time.Hour),
	})
	if err == nil {
		t.Fatal("expected reservation conflict")
	}
}

func TestRealListenerWins(t *testing.T) {
	st := store.New(t.TempDir())
	service := NewService(st, fakeScanner{listeners: []model.PortListener{{
		Port: 8080, PID: 42, Process: "demo",
	}}})
	availability, err := service.Check(context.Background(), 8080, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if availability.Available || availability.Listener == nil {
		t.Fatalf("expected occupied listener, got %#v", availability)
	}
}

func TestPersistentAllocationBlocksOtherProjectWhileIdle(t *testing.T) {
	st := store.New(t.TempDir())
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	_, err := st.Update(context.Background(), func(registry *model.Registry) error {
		registry.Projects = append(registry.Projects,
			model.Project{ID: "alpha", Name: "Alpha", Ports: []int{}, Source: "codex", SyncMode: "manual"},
			model.Project{ID: "beta", Name: "Beta", Ports: []int{}, Source: "trae", SyncMode: "manual"},
		)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, fakeScanner{})
	service.now = func() time.Time { return now }
	allocation, err := service.Allocate(context.Background(), 5173, "alpha", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if allocation.State != "idle" {
		t.Fatalf("expected idle allocation, got %#v", allocation)
	}
	own, err := service.Check(context.Background(), 5173, "alpha")
	if err != nil || !own.Available || own.Allocation == nil {
		t.Fatalf("expected own allocation to be usable: %#v, %v", own, err)
	}
	other, err := service.Check(context.Background(), 5173, "beta")
	if err != nil || other.Available || other.Allocation == nil {
		t.Fatalf("expected allocation to block beta: %#v, %v", other, err)
	}
	if _, err := service.Reserve(context.Background(), model.PortReservation{
		Port: 5173, ProjectID: "beta", Owner: "trae", ExpiresAt: now.Add(time.Hour),
	}); err == nil {
		t.Fatal("expected durable allocation to block temporary reservation")
	}
}

func TestPoolSuggestsOnlyUnusedPortsAndUnassigns(t *testing.T) {
	st := store.New(t.TempDir())
	_, err := st.Update(context.Background(), func(registry *model.Registry) error {
		registry.Projects = append(registry.Projects,
			model.Project{ID: "alpha", Name: "Alpha", Ports: []int{3000}, Source: "codex", SyncMode: "manual"},
		)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(st, fakeScanner{listeners: []model.PortListener{{Port: 3001, PID: 1, Process: "node"}}})
	pool, err := service.Pool(context.Background(), 3000, 3003, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pool.Suggestions) != 2 || pool.Suggestions[0] != 3002 || pool.Suggestions[1] != 3003 {
		t.Fatalf("unexpected suggestions: %#v", pool.Suggestions)
	}
	if err := service.Unassign(context.Background(), 3000, "alpha"); err != nil {
		t.Fatal(err)
	}
	availability, err := service.Check(context.Background(), 3000, "beta")
	if err != nil || !availability.Available {
		t.Fatalf("expected unassigned port to be free: %#v, %v", availability, err)
	}
}
