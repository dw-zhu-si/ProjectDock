package projects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"projectdock/internal/model"
)

var ErrProjectIgnored = errors.New("项目路径已被用户删除并加入自动同步忽略列表")

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

type SyncInput struct {
	Path         string `json:"path"`
	Name         string `json:"name,omitempty"`
	Source       string `json:"source"`
	DiscoveredBy string `json:"discoveredBy,omitempty"`
	ProjectCard  string `json:"projectCard,omitempty"`
	Revive       bool   `json:"revive,omitempty"`
}

type SyncResult struct {
	Action  string        `json:"action"`
	Project model.Project `json:"project"`
}

type RegistrySyncItem struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	ID      string `json:"id,omitempty"`
}

type RegistrySyncReport struct {
	File    string             `json:"file"`
	Added   int                `json:"added"`
	Updated int                `json:"updated"`
	Ignored int                `json:"ignored"`
	Skipped int                `json:"skipped"`
	Items   []RegistrySyncItem `json:"items"`
}

type knownRegistry struct {
	Version  int    `json:"version"`
	Vault    string `json:"vault"`
	Projects []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Card string `json:"card"`
	} `json:"projects"`
}

func (s *Service) SyncPath(ctx context.Context, input SyncInput) (SyncResult, error) {
	return s.syncPath(ctx, input, true, true)
}

func (s *Service) syncPath(ctx context.Context, input SyncInput, writeAudit, refreshProfiles bool) (SyncResult, error) {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()

	path, err := canonicalDirectory(input.Path)
	if err != nil {
		return SyncResult{}, err
	}
	input.Path = path
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = filepath.Base(path)
	}
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	if input.Source == "" {
		input.Source = "other"
	}
	input.DiscoveredBy = strings.ToLower(strings.TrimSpace(input.DiscoveredBy))
	if input.DiscoveredBy == "" {
		input.DiscoveredBy = input.Source
	}
	if err := validateSyncSource(input.Source); err != nil {
		return SyncResult{}, err
	}

	now := s.now()
	result := SyncResult{}
	_, err = s.store.Update(ctx, func(registry *model.Registry) error {
		if input.Revive {
			registry.IgnoredPaths = removeIgnoredPath(registry.IgnoredPaths, path)
		} else if ignoredPath(registry.IgnoredPaths, path) {
			return ErrProjectIgnored
		}

		for index := range registry.Projects {
			existing := registry.Projects[index]
			if samePath(existing.Path, path) {
				if existing.SyncMode == "auto" && input.Name != "" {
					existing.Name = input.Name
				}
				if existing.Source == "tri-agent" || existing.Source == "other" {
					existing.Source = input.Source
				}
				if input.ProjectCard != "" {
					existing.ProjectCard = strings.TrimSpace(input.ProjectCard)
				}
				existing.DiscoveredBy = input.DiscoveredBy
				existing.LastSeenAt = now
				existing.UpdatedAt = now
				if input.Revive {
					existing.SyncMode = "manual"
				}
				existing = model.NormalizeProject(existing, now)
				if err := existing.Validate(true); err != nil {
					return err
				}
				registry.Projects[index] = existing
				result = SyncResult{Action: "updated", Project: registry.Projects[index]}
				return nil
			}
		}

		syncMode := "auto"
		if input.Revive {
			syncMode = "manual"
		}
		project := model.NormalizeProject(model.Project{
			ID:           stableProjectID(path),
			Name:         input.Name,
			Path:         path,
			Source:       input.Source,
			SyncMode:     syncMode,
			DiscoveredBy: input.DiscoveredBy,
			ProjectCard:  strings.TrimSpace(input.ProjectCard),
			Ports:        []int{},
			LastSeenAt:   now,
		}, now)
		if err := project.Validate(true); err != nil {
			return err
		}
		for _, existing := range registry.Projects {
			if existing.ID == project.ID && !samePath(existing.Path, project.Path) {
				project.ID = stableProjectID(path + "#collision")
				break
			}
		}
		registry.Projects = append(registry.Projects, project)
		result = SyncResult{Action: "added", Project: registry.Projects[len(registry.Projects)-1]}
		return nil
	})
	if err != nil {
		return SyncResult{}, err
	}
	if refreshProfiles {
		refreshed, refreshErr := s.refreshLaunchProfilesUnlocked(ctx, result.Project.ID)
		if refreshErr != nil {
			return SyncResult{}, refreshErr
		}
		for _, project := range refreshed.Projects {
			if project.ID == result.Project.ID {
				result.Project = project
				break
			}
		}
	}
	if writeAudit {
		s.audit(ctx, "project.sync", "success", result.Project.ID, 0, map[string]any{
			"action": result.Action, "source": input.Source, "path": result.Project.Path, "revived": input.Revive,
		})
	}
	return result, nil
}

func (s *Service) SyncRegistry(ctx context.Context, registryPath string) (RegistrySyncReport, error) {
	path, err := filepath.Abs(strings.TrimSpace(registryPath))
	if err != nil {
		return RegistrySyncReport{}, fmt.Errorf("解析项目注册表路径: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return RegistrySyncReport{}, fmt.Errorf("打开项目注册表: %w", err)
	}
	defer file.Close()

	var registry knownRegistry
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return RegistrySyncReport{}, fmt.Errorf("读取项目注册表: %w", err)
	}
	report := RegistrySyncReport{File: path, Items: []RegistrySyncItem{}}
	for _, item := range registry.Projects {
		entry := RegistrySyncItem{Name: strings.TrimSpace(item.Name), Path: strings.TrimSpace(item.Path)}
		result, syncErr := s.syncPath(ctx, SyncInput{
			Path: item.Path, Name: item.Name, Source: "tri-agent",
			DiscoveredBy: "tri-agent-registry", ProjectCard: item.Card,
		}, false, false)
		switch {
		case errors.Is(syncErr, ErrProjectIgnored):
			entry.Status = "ignored"
			entry.Message = syncErr.Error()
			report.Ignored++
		case syncErr != nil:
			entry.Status = "skipped"
			entry.Message = syncErr.Error()
			report.Skipped++
		default:
			entry.Status = result.Action
			entry.ID = result.Project.ID
			if result.Action == "added" {
				report.Added++
			} else {
				report.Updated++
			}
		}
		report.Items = append(report.Items, entry)
	}
	s.lifecycle.Lock()
	_, refreshErr := s.refreshLaunchProfilesUnlocked(ctx)
	s.lifecycle.Unlock()
	if refreshErr != nil {
		return RegistrySyncReport{}, refreshErr
	}
	s.audit(ctx, "registry.sync", "success", "", 0, map[string]any{
		"file": path, "added": report.Added, "updated": report.Updated,
		"ignored": report.Ignored, "skipped": report.Skipped,
	})
	return report, nil
}

func (s *Service) refreshLaunchProfilesUnlocked(ctx context.Context, projectIDs ...string) (model.Registry, error) {
	registry, err := s.store.Load(ctx)
	if err != nil {
		return model.Registry{}, err
	}
	selected := make(map[string]bool, len(projectIDs))
	for _, id := range projectIDs {
		selected[id] = true
	}
	detected := make([]model.Project, 0, len(registry.Projects))
	for _, project := range registry.Projects {
		if len(selected) == 0 || selected[project.ID] {
			detected = append(detected, project)
		}
	}
	refreshDetectedLaunchProfiles(detected)
	detectedByID := make(map[string]model.Project, len(detected))
	now := s.now()
	for index := range detected {
		detected[index] = model.NormalizeProject(detected[index], now)
		// The registry intentionally retains projects on temporarily unavailable
		// volumes. Refreshing another project must not fail because one of those
		// paths is offline.
		if err := detected[index].Validate(false); err != nil {
			return model.Registry{}, err
		}
		detectedByID[detected[index].ID] = detected[index]
	}
	return s.store.Update(ctx, func(current *model.Registry) error {
		for index := range current.Projects {
			profile, exists := detectedByID[current.Projects[index].ID]
			if !exists {
				continue
			}
			if current.Projects[index].WorkingDirectory == profile.WorkingDirectory &&
				current.Projects[index].StartCommand == profile.StartCommand &&
				current.Projects[index].StopCommand == profile.StopCommand &&
				current.Projects[index].LaunchSource == profile.LaunchSource &&
				slices.Equal(current.Projects[index].LaunchPorts, profile.LaunchPorts) {
				continue
			}
			current.Projects[index].WorkingDirectory = profile.WorkingDirectory
			current.Projects[index].StartCommand = profile.StartCommand
			current.Projects[index].StopCommand = profile.StopCommand
			current.Projects[index].LaunchSource = profile.LaunchSource
			current.Projects[index].LaunchPorts = profile.LaunchPorts
			current.Projects[index].UpdatedAt = now
		}
		return nil
	})
}

func canonicalDirectory(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("项目路径不能为空")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("解析项目路径: %w", err)
	}
	absolute = filepath.Clean(absolute)
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = resolved
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("项目路径不可访问: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("项目路径不是目录")
	}
	return absolute, nil
}

func validateSyncSource(source string) error {
	switch source {
	case "codex", "trae", "claude", "manual", "tri-agent", "other":
		return nil
	default:
		return errors.New("同步来源必须是 codex、trae、claude、manual、tri-agent 或 other")
	}
}

func stableProjectID(path string) string {
	base := strings.ToLower(filepath.Base(path))
	base = strings.Trim(slugPattern.ReplaceAllString(base, "-"), "-")
	if len(base) > 30 {
		base = base[:30]
	}
	sum := sha256.Sum256([]byte(path))
	suffix := hex.EncodeToString(sum[:])[:10]
	if base == "" {
		base = "project"
	}
	id := base + "-" + suffix
	if len(id) > 48 {
		id = id[:48]
	}
	return id
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func ignoredPath(ignored []model.IgnoredPath, path string) bool {
	for _, item := range ignored {
		if samePath(item.Path, path) {
			return true
		}
	}
	return false
}

func removeIgnoredPath(ignored []model.IgnoredPath, path string) []model.IgnoredPath {
	filtered := make([]model.IgnoredPath, 0, len(ignored))
	for _, item := range ignored {
		if samePath(item.Path, path) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
