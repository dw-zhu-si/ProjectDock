package widget

import (
	"sort"
	"time"

	"projectdock/internal/model"
	"projectdock/internal/ports"
	"projectdock/internal/projects"
)

const SchemaVersion = 2

type ProjectSummary struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Source    string `json:"source"`
	PortCount int    `json:"portCount"`
}

type AllocationSummary struct {
	Port        int    `json:"port"`
	ProjectName string `json:"projectName"`
	State       string `json:"state"`
}

type Snapshot struct {
	SchemaVersion             int                 `json:"schemaVersion"`
	UpdatedAt                 time.Time           `json:"updatedAt"`
	ProjectCount              int                 `json:"projectCount"`
	RunningCount              int                 `json:"runningCount"`
	ListeningPortCount        int                 `json:"listeningPortCount"`
	AllocatedPortCount        int                 `json:"allocatedPortCount"`
	ActiveAllocationCount     int                 `json:"activeAllocationCount"`
	TemporaryReservationCount int                 `json:"temporaryReservationCount"`
	RegisteredOnlyCount       int                 `json:"registeredOnlyCount"`
	Projects                  []ProjectSummary    `json:"projects"`
	Allocations               []AllocationSummary `json:"allocations"`
}

func Build(
	projectViews []projects.ProjectView,
	listeners []model.PortListener,
	allocations []ports.PortAllocation,
	reservations []model.PortReservation,
	now time.Time,
) Snapshot {
	result := Snapshot{
		SchemaVersion:             SchemaVersion,
		UpdatedAt:                 now,
		ProjectCount:              len(projectViews),
		AllocatedPortCount:        len(allocations),
		TemporaryReservationCount: len(reservations),
		Projects:                  []ProjectSummary{},
		Allocations:               []AllocationSummary{},
	}

	uniquePorts := map[int]struct{}{}
	for _, listener := range listeners {
		uniquePorts[listener.Port] = struct{}{}
	}
	result.ListeningPortCount = len(uniquePorts)

	type rankedSummary struct {
		rank    int
		summary ProjectSummary
	}
	ranked := make([]rankedSummary, 0, len(projectViews))
	for _, view := range projectViews {
		state, rank := projectState(view)
		if state == "running" {
			result.RunningCount++
		}
		if state == "registered" {
			result.RegisteredOnlyCount++
		}
		ranked = append(ranked, rankedSummary{
			rank: rank,
			summary: ProjectSummary{
				Name:      view.Name,
				State:     state,
				Source:    view.Source,
				PortCount: len(view.Ports),
			},
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].rank == ranked[j].rank {
			return ranked[i].summary.Name < ranked[j].summary.Name
		}
		return ranked[i].rank < ranked[j].rank
	})
	for _, item := range ranked {
		result.Projects = append(result.Projects, item.summary)
		if len(result.Projects) == 6 {
			break
		}
	}
	for _, allocation := range allocations {
		if allocation.State == "active" {
			result.ActiveAllocationCount++
		}
		result.Allocations = append(result.Allocations, AllocationSummary{
			Port: allocation.Port, ProjectName: allocation.ProjectName, State: allocation.State,
		})
		if len(result.Allocations) == 8 {
			break
		}
	}
	return result
}

func projectState(view projects.ProjectView) (string, int) {
	switch view.Run.State {
	case "running", "starting", "stopping", "external":
		return "running", 0
	case "conflict":
		return "conflict", 1
	}
	if !view.PathAvailable {
		return "unavailable", 2
	}
	if !view.ConfiguredToStart {
		return "registered", 4
	}
	return "ready", 3
}
