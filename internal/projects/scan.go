package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxScanDepth   = 5
	maxScanResults = 250
)

type ScanCandidate struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	Managed      bool   `json:"managed"`
	Manageable   bool   `json:"manageable"`
	StartCommand string `json:"startCommand,omitempty"`
}

type ScanReport struct {
	Root       string          `json:"root"`
	Candidates []ScanCandidate `json:"candidates"`
	Truncated  bool            `json:"truncated"`
}

func (s *Service) Scan(ctx context.Context, rawRoot string) (ScanReport, error) {
	root, err := canonicalDirectory(rawRoot)
	if err != nil {
		return ScanReport{}, err
	}
	registry, err := s.store.Load(ctx)
	if err != nil {
		return ScanReport{}, err
	}
	managedPaths := make(map[string]bool, len(registry.Projects))
	for _, project := range registry.Projects {
		managedPaths[filepath.Clean(project.Path)] = true
	}
	report := ScanReport{Root: root, Candidates: []ScanCandidate{}}
	var visit func(string, int) error
	visit = func(directory string, depth int) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if len(report.Candidates) >= maxScanResults {
			report.Truncated = true
			return nil
		}
		kind, candidate := projectMarker(directory)
		if candidate {
			profile, manageable := detectLaunchProfile(directory, "")
			manageable = manageable && strings.TrimSpace(profile.StartCommand) != ""
			report.Candidates = append(report.Candidates, ScanCandidate{
				Name: filepath.Base(directory), Path: directory, Kind: kind,
				Managed: managedPaths[filepath.Clean(directory)], Manageable: manageable,
				StartCommand: profile.StartCommand,
			})
			if manageable {
				return nil
			}
		}
		if depth >= maxScanDepth {
			return nil
		}
		entries, readErr := os.ReadDir(directory)
		if readErr != nil {
			if depth == 0 {
				return readErr
			}
			return nil
		}
		for _, entry := range entries {
			if !entry.IsDir() || ignoredScanDirectory(entry.Name()) {
				continue
			}
			child := filepath.Join(directory, entry.Name())
			info, statErr := os.Lstat(child)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if err := visit(child, depth+1); err != nil {
				return err
			}
			if report.Truncated {
				return nil
			}
		}
		return nil
	}
	if err := visit(root, 0); err != nil {
		return ScanReport{}, err
	}
	sort.Slice(report.Candidates, func(i, j int) bool {
		if report.Candidates[i].Manageable != report.Candidates[j].Manageable {
			return report.Candidates[i].Manageable
		}
		return strings.ToLower(report.Candidates[i].Path) < strings.ToLower(report.Candidates[j].Path)
	})
	return report, nil
}

func projectMarker(directory string) (string, bool) {
	markers := []struct {
		name string
		kind string
	}{
		{".projectdock.json", "ProjectDock"}, {"projectdock.json", "ProjectDock"},
		{"package.json", "Node.js"}, {"go.mod", "Go"}, {"Package.swift", "Swift"},
		{"pyproject.toml", "Python"}, {"Cargo.toml", "Rust"},
		{"compose.yml", "Docker Compose"}, {"compose.yaml", "Docker Compose"},
		{"docker-compose.yml", "Docker Compose"}, {"Makefile", "Make"},
		{".git", "Git"},
	}
	for _, marker := range markers {
		if _, err := os.Lstat(filepath.Join(directory, marker.name)); err == nil {
			return marker.kind, true
		}
	}
	entries, err := os.ReadDir(directory)
	if err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".xcodeproj") && entry.IsDir() {
				return "Xcode", true
			}
		}
	}
	return "", false
}

func ignoredScanDirectory(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "node_modules", "vendor", "dist", "build", "out", "target", "deriveddata", "pods", "library", "__pycache__", ".venv", "venv":
		return true
	default:
		return false
	}
}

var errUnsafeProjectRemoval = errors.New("项目目录未通过安全删除检查")
