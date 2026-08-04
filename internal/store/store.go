package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"projectdock/internal/model"
)

const maxAuditEvents = 500

type Store struct {
	dir          string
	registryPath string
	lockPath     string
	mu           sync.Mutex
}

func New(dir string) *Store {
	return &Store{
		dir:          dir,
		registryPath: filepath.Join(dir, "registry.json"),
		lockPath:     filepath.Join(dir, "registry.lock"),
	}
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) Ensure() error {
	if err := os.MkdirAll(filepath.Join(s.dir, "logs"), 0o700); err != nil {
		return fmt.Errorf("创建 ProjectDock 数据目录: %w", err)
	}
	return nil
}

func (s *Store) Load(ctx context.Context) (model.Registry, error) {
	var registry model.Registry
	err := s.withLock(ctx, syscall.LOCK_SH, func() error {
		var err error
		registry, err = s.loadUnlocked()
		return err
	})
	return registry, err
}

func (s *Store) Update(ctx context.Context, fn func(*model.Registry) error) (model.Registry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var updated model.Registry
	err := s.withLock(ctx, syscall.LOCK_EX, func() error {
		registry, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if err := fn(&registry); err != nil {
			return err
		}
		registry.Version = model.RegistryVersion
		registry.Reservations = activeReservations(registry.Reservations, time.Now())
		if len(registry.Audit) > maxAuditEvents {
			registry.Audit = registry.Audit[len(registry.Audit)-maxAuditEvents:]
		}
		if err := s.writeUnlocked(registry); err != nil {
			return err
		}
		updated = registry
		return nil
	})
	return updated, err
}

func (s *Store) AppendAudit(ctx context.Context, event model.AuditEvent) error {
	_, err := s.Update(ctx, func(registry *model.Registry) error {
		registry.Audit = append(registry.Audit, event)
		return nil
	})
	return err
}

func (s *Store) loadUnlocked() (model.Registry, error) {
	if err := s.Ensure(); err != nil {
		return model.Registry{}, err
	}
	file, err := os.Open(s.registryPath)
	if errors.Is(err, os.ErrNotExist) {
		return model.NewRegistry(), nil
	}
	if err != nil {
		return model.Registry{}, fmt.Errorf("打开注册表: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 4<<20))
	decoder.DisallowUnknownFields()
	var registry model.Registry
	if err := decoder.Decode(&registry); err != nil {
		return model.Registry{}, fmt.Errorf("读取注册表: %w", err)
	}
	if registry.Version == 1 {
		registry.Version = 2
		for index := range registry.Projects {
			if registry.Projects[index].SyncMode == "" {
				registry.Projects[index].SyncMode = "manual"
			}
			if registry.Projects[index].DiscoveredBy == "" {
				registry.Projects[index].DiscoveredBy = registry.Projects[index].Source
			}
		}
	}
	if registry.Version == 2 {
		registry.Version = 3
		allocated := map[string]bool{}
		for _, project := range registry.Projects {
			for _, port := range project.Ports {
				allocated[fmt.Sprintf("%s:%d", project.ID, port)] = true
			}
		}
		filtered := make([]model.PortReservation, 0, len(registry.Reservations))
		for _, reservation := range registry.Reservations {
			if allocated[fmt.Sprintf("%s:%d", reservation.ProjectID, reservation.Port)] {
				continue
			}
			filtered = append(filtered, reservation)
		}
		registry.Reservations = filtered
	}
	if registry.Version == 3 {
		registry.Version = 4
	}
	if registry.Version == 4 {
		registry.Projects, registry.Reservations = deduplicateProjects(registry.Projects, registry.Reservations)
		registry.Version = 5
	}
	if registry.Version != model.RegistryVersion {
		return model.Registry{}, fmt.Errorf("不支持的注册表版本 %d", registry.Version)
	}
	for index := range registry.Projects {
		if registry.Projects[index].StartCommand != "" && registry.Projects[index].LaunchSource == "" {
			registry.Projects[index].LaunchSource = "manual"
		}
	}
	if registry.Projects == nil {
		registry.Projects = []model.Project{}
	}
	if registry.Reservations == nil {
		registry.Reservations = []model.PortReservation{}
	}
	if registry.Audit == nil {
		registry.Audit = []model.AuditEvent{}
	}
	if registry.IgnoredPaths == nil {
		registry.IgnoredPaths = []model.IgnoredPath{}
	}
	registry.Reservations = activeReservations(registry.Reservations, time.Now())
	return registry, nil
}

func deduplicateProjects(projects []model.Project, reservations []model.PortReservation) ([]model.Project, []model.PortReservation) {
	merged := make([]model.Project, 0, len(projects))
	indexByPath := make(map[string]int, len(projects))
	projectIDRemap := make(map[string]string)
	for _, project := range projects {
		pathKey := filepath.Clean(project.Path)
		existingIndex, duplicate := indexByPath[pathKey]
		if !duplicate {
			indexByPath[pathKey] = len(merged)
			merged = append(merged, project)
			continue
		}

		existing := merged[existingIndex]
		winner, loser := existing, project
		if projectMergeScore(project) > projectMergeScore(existing) {
			winner, loser = project, existing
		}
		winner.Ports = uniquePorts(winner.Ports, loser.Ports)
		winner.LaunchPorts = uniquePorts(winner.LaunchPorts, loser.LaunchPorts)
		if winner.StartCommand == "" {
			winner.StartCommand = loser.StartCommand
			winner.WorkingDirectory = loser.WorkingDirectory
			winner.LaunchSource = loser.LaunchSource
		}
		if winner.StopCommand == "" {
			winner.StopCommand = loser.StopCommand
		}
		if winner.HealthURL == "" {
			winner.HealthURL = loser.HealthURL
		}
		if winner.ProjectCard == "" {
			winner.ProjectCard = loser.ProjectCard
		}
		if winner.CreatedAt.IsZero() || (!loser.CreatedAt.IsZero() && loser.CreatedAt.Before(winner.CreatedAt)) {
			winner.CreatedAt = loser.CreatedAt
		}
		if loser.LastSeenAt.After(winner.LastSeenAt) {
			winner.LastSeenAt = loser.LastSeenAt
		}
		if loser.UpdatedAt.After(winner.UpdatedAt) {
			winner.UpdatedAt = loser.UpdatedAt
		}
		projectIDRemap[loser.ID] = winner.ID
		merged[existingIndex] = winner
	}
	for index := range reservations {
		if winnerID, exists := projectIDRemap[reservations[index].ProjectID]; exists {
			reservations[index].ProjectID = winnerID
		}
	}
	return merged, reservations
}

func projectMergeScore(project model.Project) int {
	score := 0
	if project.LaunchSource == "manual" && project.StartCommand != "" {
		score += 100
	}
	if project.StartCommand != "" {
		score += 50
	}
	if len(project.Ports) > 0 {
		score += 20
	}
	if project.SyncMode == "manual" {
		score += 10
	}
	if project.ProjectCard != "" {
		score += 5
	}
	return score
}

func uniquePorts(primary, secondary []int) []int {
	seen := make(map[int]bool, len(primary)+len(secondary))
	ports := make([]int, 0, len(primary)+len(secondary))
	for _, candidates := range [][]int{primary, secondary} {
		for _, port := range candidates {
			if seen[port] {
				continue
			}
			seen[port] = true
			ports = append(ports, port)
		}
	}
	return ports
}

func (s *Store) writeUnlocked(registry model.Registry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("编码注册表: %w", err)
	}
	temp, err := os.CreateTemp(s.dir, "registry-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时注册表: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("设置注册表权限: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("写入临时注册表: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("同步临时注册表: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭临时注册表: %w", err)
	}
	if err := os.Rename(tempName, s.registryPath); err != nil {
		return fmt.Errorf("替换注册表: %w", err)
	}
	return nil
}

func (s *Store) withLock(ctx context.Context, mode int, fn func() error) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("打开注册表锁: %w", err)
	}
	defer lockFile.Close()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = syscall.Flock(int(lockFile.Fd()), mode|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("锁定注册表: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	return fn()
}

func activeReservations(all []model.PortReservation, now time.Time) []model.PortReservation {
	active := make([]model.PortReservation, 0, len(all))
	for _, reservation := range all {
		if reservation.Active(now) {
			active = append(active, reservation)
		}
	}
	return active
}
