package projects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"projectdock/internal/model"
)

type launchProfile struct {
	WorkingDirectory string
	StartCommand     string
	StopCommand      string
	Source           string
	Ports            []int
}

type lifecycleManifest struct {
	WorkingDirectory string `json:"workingDirectory"`
	StartCommand     string `json:"startCommand"`
	StopCommand      string `json:"stopCommand"`
	Ports            []int  `json:"ports"`
}

var (
	makeTargetPattern  = regexp.MustCompile(`(?m)^([A-Za-z0-9_.-]+)\s*:`)
	commandPortPattern = regexp.MustCompile(`(?:^|\s)--port(?:=|\s+)([0-9]{1,5})(?:\s|$)`)
	composePortPattern = regexp.MustCompile(`(?m)(?:127\.0\.0\.1:|localhost:|\[::1\]:)?(?:\$\{[A-Za-z_][A-Za-z0-9_]*:-)?([0-9]{2,5})\}?:[0-9]{1,5}(?:/(?:tcp|udp))?`)
	composeUpPattern   = regexp.MustCompile(`(?s)^(.*(?:docker compose|docker-compose)(?:\s+-f\s+\S+)?)\s+up(?:\s+.*)?$`)
)

var automaticLaunchSources = map[string]bool{
	"projectdock-manifest": true,
	"script":               true,
	"make":                 true,
	"compose":              true,
	"package":              true,
	"archived":             true,
}

func refreshDetectedLaunchProfiles(projects []model.Project) {
	registeredPaths := make(map[string]string, len(projects))
	for _, project := range projects {
		registeredPaths[filepath.Clean(project.Path)] = project.ID
	}
	for index := range projects {
		project := projects[index]
		if info, err := os.Stat(project.Path); err != nil || !info.IsDir() {
			// Removable volumes and archived folders can be temporarily offline.
			// Preserve the last known launch profile until the path is available.
			continue
		}
		if project.LaunchSource == "manual" {
			if project.StopCommand == "" {
				if matches := composeUpPattern.FindStringSubmatch(project.StartCommand); len(matches) == 2 {
					project.StopCommand = strings.TrimSpace(matches[1]) + " stop"
				} else if runtimeDirectory, err := project.RuntimeDirectory(false); err == nil {
					if profile, found := detectInDirectory(runtimeDirectory); found && profile.StopCommand != "" {
						project.StopCommand = profile.StopCommand
					}
				}
			}
			projects[index] = project
			continue
		}
		if project.StartCommand != "" && !automaticLaunchSources[project.LaunchSource] {
			project.LaunchSource = "manual"
			projects[index] = project
			continue
		}

		profile, found := detectLaunchProfile(project.Path, project.ProjectCard)
		if found && profile.WorkingDirectory != "" {
			candidatePath := filepath.Clean(filepath.Join(project.Path, profile.WorkingDirectory))
			if owner, exists := registeredPaths[candidatePath]; exists && owner != project.ID {
				found = false
			}
		}
		if !found {
			if automaticLaunchSources[project.LaunchSource] {
				project.WorkingDirectory = ""
				project.StartCommand = ""
				project.StopCommand = ""
				project.LaunchSource = ""
				project.LaunchPorts = nil
			}
			projects[index] = project
			continue
		}
		project.WorkingDirectory = profile.WorkingDirectory
		project.StartCommand = profile.StartCommand
		project.StopCommand = profile.StopCommand
		project.LaunchSource = profile.Source
		project.LaunchPorts = profile.Ports
		projects[index] = project
	}
}

func detectLaunchProfile(projectPath, projectCard string) (launchProfile, bool) {
	if archivedProjectCard(projectCard) {
		return launchProfile{Source: "archived"}, true
	}
	if profile, found := detectInDirectory(projectPath); found {
		return profile, true
	}

	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return launchProfile{}, false
	}
	directoryCount := 0
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && !ignoredDetectionDirectory(entry.Name()) {
			directoryCount++
		}
	}
	if directoryCount > 40 {
		return launchProfile{}, false
	}
	candidates := []launchProfile{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || ignoredDetectionDirectory(entry.Name()) {
			continue
		}
		childPath := filepath.Join(projectPath, entry.Name())
		if profile, found := detectInDirectory(childPath); found {
			profile.WorkingDirectory = entry.Name()
			candidates = append(candidates, profile)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return launchProfile{}, false
}

func detectInDirectory(directory string) (launchProfile, bool) {
	for _, name := range []string{".projectdock.json", "projectdock.json"} {
		if profile, ok := detectManifest(directory, name); ok {
			return profile, true
		}
	}
	for _, name := range []string{"start.sh", "run.sh"} {
		path := filepath.Join(directory, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			stopCommand := ""
			if stopInfo, stopErr := os.Stat(filepath.Join(directory, "stop.sh")); stopErr == nil && !stopInfo.IsDir() {
				stopCommand = "./stop.sh"
			}
			return launchProfile{
				StartCommand: "./" + name,
				StopCommand:  stopCommand,
				Source:       "script",
			}, true
		}
	}
	if profile, ok := detectMakefile(directory); ok {
		return profile, true
	}
	for _, name := range []string{"compose.yml", "compose.yaml", "docker-compose.yml", "docker-compose.yaml"} {
		if profile, ok := detectCompose(directory, name); ok {
			return profile, true
		}
	}
	if profile, ok := detectPackage(directory); ok {
		return profile, true
	}
	return launchProfile{}, false
}

func detectManifest(directory, name string) (launchProfile, bool) {
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		return launchProfile{}, false
	}
	var manifest lifecycleManifest
	if err := json.Unmarshal(data, &manifest); err != nil || strings.TrimSpace(manifest.StartCommand) == "" {
		return launchProfile{}, false
	}
	workingDirectory := strings.TrimSpace(manifest.WorkingDirectory)
	if workingDirectory != "" {
		if filepath.IsAbs(workingDirectory) {
			return launchProfile{}, false
		}
		cleaned := filepath.Clean(workingDirectory)
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return launchProfile{}, false
		}
		workingDirectory = cleaned
	}
	return launchProfile{
		WorkingDirectory: workingDirectory,
		StartCommand:     strings.TrimSpace(manifest.StartCommand),
		StopCommand:      strings.TrimSpace(manifest.StopCommand),
		Source:           "projectdock-manifest",
		Ports:            validUniquePorts(manifest.Ports),
	}, true
}

func detectMakefile(directory string) (launchProfile, bool) {
	var data []byte
	var err error
	for _, name := range []string{"Makefile", "makefile"} {
		data, err = os.ReadFile(filepath.Join(directory, name))
		if err == nil {
			break
		}
	}
	if err != nil {
		return launchProfile{}, false
	}
	targets := map[string]bool{}
	for _, match := range makeTargetPattern.FindAllStringSubmatch(string(data), -1) {
		targets[match[1]] = true
	}
	start := ""
	for _, target := range []string{"run", "dev", "start"} {
		if targets[target] {
			start = "make " + target
			break
		}
	}
	if start == "" {
		return launchProfile{}, false
	}
	stop := ""
	if targets["stop"] {
		stop = "make stop"
	}
	return launchProfile{StartCommand: start, StopCommand: stop, Source: "make"}, true
}

func detectCompose(directory, name string) (launchProfile, bool) {
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil || !strings.Contains(string(data), "services:") {
		return launchProfile{}, false
	}
	return launchProfile{
		StartCommand: "docker compose -f " + name + " up --build",
		StopCommand:  "docker compose -f " + name + " stop",
		Source:       "compose",
		Ports:        portsFromCompose(string(data)),
	}, true
}

func detectPackage(directory string) (launchProfile, bool) {
	data, err := os.ReadFile(filepath.Join(directory, "package.json"))
	if err != nil {
		return launchProfile{}, false
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return launchProfile{}, false
	}
	script := ""
	for _, candidate := range []string{"dev", "start", "serve"} {
		if strings.TrimSpace(manifest.Scripts[candidate]) != "" {
			script = candidate
			break
		}
	}
	if script == "" {
		return launchProfile{}, false
	}
	manager := "npm"
	switch {
	case fileExists(filepath.Join(directory, "pnpm-lock.yaml")):
		manager = "pnpm"
	case fileExists(filepath.Join(directory, "yarn.lock")):
		manager = "yarn"
	case fileExists(filepath.Join(directory, "bun.lockb")) || fileExists(filepath.Join(directory, "bun.lock")):
		manager = "bun"
	}
	command := manager + " run " + script
	ports := portsFromPackageCommand(manifest.Scripts[script])
	return launchProfile{StartCommand: command, Source: "package", Ports: ports}, true
}

func archivedProjectCard(card string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(card))
	return strings.Contains(normalized, "/项目档案/") ||
		strings.HasPrefix(normalized, "项目档案/") ||
		strings.Contains(normalized, "/已完成/") ||
		strings.HasPrefix(normalized, "已完成/")
}

func ignoredDetectionDirectory(name string) bool {
	switch strings.ToLower(name) {
	case "node_modules", "vendor", "dist", "build", "coverage", "work", "outputs", "archive", "archives":
		return true
	default:
		return false
	}
}

func portsFromPackageCommand(command string) []int {
	ports := []int{}
	for _, match := range commandPortPattern.FindAllStringSubmatch(command, -1) {
		if port, err := strconv.Atoi(match[1]); err == nil {
			ports = append(ports, port)
		}
	}
	if len(ports) == 0 {
		lower := strings.ToLower(command)
		switch {
		case strings.Contains(lower, "vite"):
			ports = append(ports, 5173)
		case strings.Contains(lower, "next"):
			ports = append(ports, 3000)
		}
	}
	return validUniquePorts(ports)
}

func portsFromCompose(compose string) []int {
	ports := []int{}
	for _, match := range composePortPattern.FindAllStringSubmatch(compose, -1) {
		if port, err := strconv.Atoi(match[1]); err == nil {
			ports = append(ports, port)
		}
	}
	return validUniquePorts(ports)
}

func validUniquePorts(ports []int) []int {
	seen := map[int]bool{}
	valid := make([]int, 0, len(ports))
	for _, port := range ports {
		if model.ValidatePort(port) != nil || seen[port] {
			continue
		}
		seen[port] = true
		valid = append(valid, port)
	}
	sort.Ints(valid)
	return valid
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
