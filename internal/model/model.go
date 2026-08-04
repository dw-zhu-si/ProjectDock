package model

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const RegistryVersion = 5

var projectIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,47}$`)

type Registry struct {
	Version      int               `json:"version"`
	Projects     []Project         `json:"projects"`
	Reservations []PortReservation `json:"reservations"`
	Audit        []AuditEvent      `json:"audit"`
	IgnoredPaths []IgnoredPath     `json:"ignoredPaths"`
}

func NewRegistry() Registry {
	return Registry{
		Version:      RegistryVersion,
		Projects:     []Project{},
		Reservations: []PortReservation{},
		Audit:        []AuditEvent{},
		IgnoredPaths: []IgnoredPath{},
	}
}

type IgnoredPath struct {
	Path      string    `json:"path"`
	ProjectID string    `json:"projectId"`
	DeletedAt time.Time `json:"deletedAt"`
}

type Project struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	Source           string    `json:"source"`
	SyncMode         string    `json:"syncMode"`
	DiscoveredBy     string    `json:"discoveredBy,omitempty"`
	ProjectCard      string    `json:"projectCard,omitempty"`
	WorkingDirectory string    `json:"workingDirectory,omitempty"`
	StartCommand     string    `json:"startCommand"`
	StopCommand      string    `json:"stopCommand,omitempty"`
	LaunchSource     string    `json:"launchSource,omitempty"`
	Ports            []int     `json:"ports"`
	LaunchPorts      []int     `json:"launchPorts,omitempty"`
	HealthURL        string    `json:"healthUrl,omitempty"`
	LastSeenAt       time.Time `json:"lastSeenAt,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (p Project) Validate(requireExistingPath bool) error {
	if !projectIDPattern.MatchString(p.ID) {
		return errors.New("项目 ID 只能包含小写字母、数字、下划线或连字符，长度为 2-48")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("项目名称不能为空")
	}
	if !filepath.IsAbs(p.Path) {
		return errors.New("项目路径必须是绝对路径")
	}
	if requireExistingPath {
		info, err := os.Stat(p.Path)
		if err != nil {
			return fmt.Errorf("项目路径不可访问: %w", err)
		}
		if !info.IsDir() {
			return errors.New("项目路径不是目录")
		}
	}
	switch p.Source {
	case "codex", "trae", "claude", "manual", "tri-agent", "other":
	default:
		return errors.New("来源工具必须是 codex、trae、claude、manual、tri-agent 或 other")
	}
	switch p.SyncMode {
	case "auto", "manual":
	default:
		return errors.New("同步方式必须是 auto 或 manual")
	}
	if _, err := p.RuntimeDirectory(requireExistingPath); err != nil {
		return err
	}
	seen := map[int]bool{}
	for _, port := range p.Ports {
		if err := ValidatePort(port); err != nil {
			return fmt.Errorf("计划端口无效: %w", err)
		}
		if seen[port] {
			return fmt.Errorf("计划端口 %d 重复", port)
		}
		seen[port] = true
	}
	seenLaunch := map[int]bool{}
	for _, port := range p.LaunchPorts {
		if err := ValidatePort(port); err != nil {
			return fmt.Errorf("检测端口无效: %w", err)
		}
		if seenLaunch[port] {
			return fmt.Errorf("检测端口 %d 重复", port)
		}
		seenLaunch[port] = true
	}
	if p.HealthURL != "" {
		if err := ValidateLoopbackURL(p.HealthURL); err != nil {
			return fmt.Errorf("健康地址无效: %w", err)
		}
	}
	return nil
}

func (p Project) RuntimeDirectory(requireExistingPath bool) (string, error) {
	workingDirectory := strings.TrimSpace(p.WorkingDirectory)
	if workingDirectory == "" || workingDirectory == "." {
		return p.Path, nil
	}
	if filepath.IsAbs(workingDirectory) {
		return "", errors.New("项目工作目录必须是项目根目录内的相对路径")
	}
	cleaned := filepath.Clean(workingDirectory)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("项目工作目录不能离开项目根目录")
	}
	runtimeDirectory := filepath.Join(p.Path, cleaned)
	relative, err := filepath.Rel(p.Path, runtimeDirectory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("项目工作目录不能离开项目根目录")
	}
	if requireExistingPath {
		info, statErr := os.Stat(runtimeDirectory)
		if statErr != nil {
			return "", fmt.Errorf("项目工作目录不可访问: %w", statErr)
		}
		if !info.IsDir() {
			return "", errors.New("项目工作目录不是目录")
		}
	}
	return runtimeDirectory, nil
}

type PortReservation struct {
	Port      int       `json:"port"`
	ProjectID string    `json:"projectId"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (r PortReservation) Active(now time.Time) bool {
	return r.ExpiresAt.After(now)
}

func (r PortReservation) Validate() error {
	if err := ValidatePort(r.Port); err != nil {
		return err
	}
	if !projectIDPattern.MatchString(r.ProjectID) {
		return errors.New("预留项目 ID 无效")
	}
	if err := ValidatePortOwner(r.Owner); err != nil {
		return err
	}
	if r.ExpiresAt.IsZero() || !r.ExpiresAt.After(r.CreatedAt) {
		return errors.New("预留过期时间无效")
	}
	return nil
}

func ValidatePortOwner(owner string) error {
	switch owner {
	case "codex", "trae", "claude", "manual", "projectdock":
		return nil
	default:
		return errors.New("端口使用方必须是 codex、trae、claude、manual 或 projectdock")
	}
}

type AuditEvent struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Action    string         `json:"action"`
	Status    string         `json:"status"`
	ProjectID string         `json:"projectId,omitempty"`
	Port      int            `json:"port,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
}

type PortListener struct {
	Port     int    `json:"port"`
	PID      int    `json:"pid"`
	Process  string `json:"process"`
	Address  string `json:"address"`
	Protocol string `json:"protocol"`
}

type RunStatus struct {
	ProjectID string    `json:"projectId"`
	State     string    `json:"state"`
	PID       int       `json:"pid,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	StoppedAt time.Time `json:"stoppedAt,omitempty"`
	ExitCode  *int      `json:"exitCode,omitempty"`
	LogPath   string    `json:"logPath,omitempty"`
	Message   string    `json:"message,omitempty"`
}

func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("端口必须在 1-65535 之间")
	}
	return nil
}

func ValidateLoopbackListen(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("监听地址格式无效: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return errors.New("只允许监听回环地址")
	}
	portNumber, err := net.LookupPort("tcp", port)
	if err != nil {
		return errors.New("监听端口无效")
	}
	return ValidatePort(portNumber)
}

func ValidateLoopbackURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("URL 解析失败")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("只允许 http 或 https")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return errors.New("只允许调用本机回环地址")
	}
	if parsed.User != nil {
		return errors.New("URL 中不能包含账号信息")
	}
	return nil
}

func NormalizeProject(project Project, now time.Time) Project {
	project.ID = strings.TrimSpace(strings.ToLower(project.ID))
	project.Name = strings.TrimSpace(project.Name)
	project.Path = filepath.Clean(strings.TrimSpace(project.Path))
	project.Source = strings.TrimSpace(strings.ToLower(project.Source))
	project.SyncMode = strings.TrimSpace(strings.ToLower(project.SyncMode))
	project.DiscoveredBy = strings.TrimSpace(strings.ToLower(project.DiscoveredBy))
	project.ProjectCard = strings.TrimSpace(project.ProjectCard)
	project.WorkingDirectory = strings.TrimSpace(project.WorkingDirectory)
	if project.WorkingDirectory == "." {
		project.WorkingDirectory = ""
	} else if project.WorkingDirectory != "" {
		project.WorkingDirectory = filepath.Clean(project.WorkingDirectory)
	}
	project.StartCommand = strings.TrimSpace(project.StartCommand)
	project.StopCommand = strings.TrimSpace(project.StopCommand)
	project.LaunchSource = strings.TrimSpace(strings.ToLower(project.LaunchSource))
	project.HealthURL = strings.TrimSpace(project.HealthURL)
	slices.Sort(project.Ports)
	slices.Sort(project.LaunchPorts)
	if project.Source == "" {
		project.Source = "manual"
	}
	if project.SyncMode == "" {
		project.SyncMode = "manual"
	}
	if project.DiscoveredBy == "" {
		project.DiscoveredBy = project.Source
	}
	if project.CreatedAt.IsZero() {
		project.CreatedAt = now
	}
	if project.LastSeenAt.IsZero() {
		project.LastSeenAt = now
	}
	project.UpdatedAt = now
	return project
}
