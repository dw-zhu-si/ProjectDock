package projects

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"projectdock/internal/model"
	"projectdock/internal/ports"
	"projectdock/internal/store"
)

type Service struct {
	store *store.Store
	ports *ports.Service
	now   func() time.Time

	lifecycle sync.Mutex
	mu        sync.RWMutex
	runs      map[string]*managedRun
}

type managedRun struct {
	command *exec.Cmd
	token   string
	status  model.RunStatus
}

type ProjectView struct {
	model.Project
	Run               model.RunStatus `json:"run"`
	PathAvailable     bool            `json:"pathAvailable"`
	ConfiguredToStart bool            `json:"configuredToStart"`
	ReadyToStart      bool            `json:"readyToStart"`
	CanStop           bool            `json:"canStop"`
	RegistrationState string          `json:"registrationState"`
	RunCapability     string          `json:"runCapability"`
}

// VisibleInControlPanel reports whether a project belongs in the start/stop UI.
// Registration-only, archived, and unavailable paths stay in the local registry
// for deduplication and audit purposes, but they are not actionable projects.
func (view ProjectView) VisibleInControlPanel() bool {
	if !view.PathAvailable || view.LaunchSource == "archived" {
		return false
	}
	return view.ConfiguredToStart || view.CanStop
}

func VisibleInControlPanel(views []ProjectView) []ProjectView {
	visible := make([]ProjectView, 0, len(views))
	for _, view := range views {
		if view.VisibleInControlPanel() {
			visible = append(visible, view)
		}
	}
	return visible
}

func NewService(store *store.Store, portService *ports.Service) *Service {
	return &Service{
		store: store,
		ports: portService,
		now:   time.Now,
		runs:  map[string]*managedRun{},
	}
}

func (s *Service) List(ctx context.Context) ([]ProjectView, error) {
	listeners, err := s.ports.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.ListWithListeners(ctx, listeners)
}

func (s *Service) ListWithListeners(ctx context.Context, listeners []model.PortListener) ([]ProjectView, error) {
	registry, err := s.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	portClaims := declaredPortClaims(registry.Projects)
	views := make([]ProjectView, 0, len(registry.Projects))
	for _, project := range registry.Projects {
		info, statErr := os.Stat(project.Path)
		pathAvailable := statErr == nil && info.IsDir()
		configured := strings.TrimSpace(project.StartCommand) != ""
		run := s.observedStatus(project, listeners, portClaims)
		runCapability := "registered"
		registrationState := "registered"
		if !pathAvailable {
			registrationState = "unavailable"
			runCapability = "unavailable"
		} else if project.LaunchSource == "archived" {
			runCapability = "archived"
		} else if run.State == "external" {
			runCapability = "external"
		} else if run.State == "conflict" {
			runCapability = "conflict"
		} else if configured {
			runCapability = "runnable"
		}
		managedRunning := run.State == "running" || run.State == "starting" || run.State == "stopping"
		views = append(views, ProjectView{
			Project:           project,
			Run:               run,
			PathAvailable:     pathAvailable,
			ConfiguredToStart: configured,
			ReadyToStart:      pathAvailable && runCapability == "runnable" && run.State == "stopped",
			CanStop:           managedRunning || (run.State == "external" && strings.TrimSpace(project.StopCommand) != ""),
			RegistrationState: registrationState,
			RunCapability:     runCapability,
		})
	}
	sort.Slice(views, func(i, j int) bool {
		return strings.ToLower(views[i].Name) < strings.ToLower(views[j].Name)
	})
	return views, nil
}

func (s *Service) Get(ctx context.Context, id string) (model.Project, error) {
	registry, err := s.store.Load(ctx)
	if err != nil {
		return model.Project{}, err
	}
	for _, project := range registry.Projects {
		if project.ID == id {
			return project, nil
		}
	}
	return model.Project{}, errors.New("项目不存在")
}

func (s *Service) Upsert(ctx context.Context, project model.Project) (model.Project, error) {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	now := s.now()
	project = model.NormalizeProject(project, now)
	if project.StartCommand != "" && project.LaunchSource == "" {
		project.LaunchSource = "manual"
	}
	if err := project.Validate(true); err != nil {
		return model.Project{}, err
	}
	if status := s.Status(project.ID); status.State == "running" || status.State == "starting" {
		return model.Project{}, errors.New("运行中的项目不能修改")
	}
	if err := s.ports.ValidateAssignments(ctx, project.ID, project.Ports); err != nil {
		return model.Project{}, err
	}

	_, err := s.store.Update(ctx, func(registry *model.Registry) error {
		registry.IgnoredPaths = removeIgnoredPath(registry.IgnoredPaths, project.Path)
		for i := range registry.Projects {
			if registry.Projects[i].ID == project.ID {
				project.CreatedAt = registry.Projects[i].CreatedAt
				registry.Projects[i] = project
				return nil
			}
		}
		registry.Projects = append(registry.Projects, project)
		return nil
	})
	if err != nil {
		return model.Project{}, err
	}
	updated, err := s.refreshLaunchProfilesUnlocked(ctx, project.ID)
	if err != nil {
		return model.Project{}, err
	}
	for _, saved := range updated.Projects {
		if saved.ID == project.ID {
			project = saved
			break
		}
	}
	s.audit(ctx, "project.upsert", "success", project.ID, 0, map[string]any{
		"name": project.Name, "source": project.Source,
	})
	return project, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.DeleteProject(ctx, id, false, "")
	return err
}

type DeleteResult struct {
	RegistrationDeleted bool   `json:"registrationDeleted"`
	FilesDeleted        bool   `json:"filesDeleted"`
	Path                string `json:"path"`
}

func (s *Service) DeleteProject(ctx context.Context, id string, removeFiles bool, confirmation string) (DeleteResult, error) {
	views, err := s.List(ctx)
	if err != nil {
		return DeleteResult{}, err
	}
	var target *ProjectView
	for index := range views {
		if views[index].ID == id {
			target = &views[index]
			break
		}
	}
	if target == nil {
		return DeleteResult{}, errors.New("项目不存在")
	}
	if target.Run.State != "stopped" && target.Run.State != "exited" && target.Run.State != "failed" {
		return DeleteResult{}, errors.New("请先停止项目再删除")
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if status := s.Status(id); status.State == "running" || status.State == "starting" {
		return DeleteResult{}, errors.New("请先停止项目再删除登记")
	}
	project := target.Project
	if removeFiles {
		if confirmation != project.Name {
			return DeleteResult{}, errors.New("彻底卸载前必须完整输入项目名称确认")
		}
		resolved, safetyErr := s.validateRemovalPath(project.Path)
		if safetyErr != nil {
			return DeleteResult{}, safetyErr
		}
		if err := os.RemoveAll(resolved); err != nil {
			return DeleteResult{}, fmt.Errorf("删除项目目录失败: %w", err)
		}
		if _, err := os.Lstat(resolved); !errors.Is(err, os.ErrNotExist) {
			return DeleteResult{}, errors.New("项目目录删除后仍然存在，登记信息已保留")
		}
	}
	_, err = s.store.Update(ctx, func(registry *model.Registry) error {
		found := false
		var deleted model.Project
		filtered := registry.Projects[:0]
		for _, project := range registry.Projects {
			if project.ID == id {
				found = true
				deleted = project
				continue
			}
			filtered = append(filtered, project)
		}
		if !found {
			return errors.New("项目不存在")
		}
		registry.Projects = filtered
		reservations := registry.Reservations[:0]
		for _, reservation := range registry.Reservations {
			if reservation.ProjectID == id {
				continue
			}
			reservations = append(reservations, reservation)
		}
		registry.Reservations = reservations
		registry.IgnoredPaths = removeIgnoredPath(registry.IgnoredPaths, deleted.Path)
		registry.IgnoredPaths = append(registry.IgnoredPaths, model.IgnoredPath{
			Path: deleted.Path, ProjectID: deleted.ID, DeletedAt: s.now(),
		})
		return nil
	})
	if err != nil {
		return DeleteResult{}, err
	}
	s.audit(ctx, "project.delete", "success", id, 0, map[string]any{"filesDeleted": removeFiles, "ignoredForAutoSync": true})
	return DeleteResult{RegistrationDeleted: true, FilesDeleted: removeFiles, Path: project.Path}, nil
}

func (s *Service) validateRemovalPath(raw string) (string, error) {
	cleaned, err := filepath.Abs(strings.TrimSpace(raw))
	if err != nil || cleaned == string(filepath.Separator) {
		return "", errors.New("项目目录未通过安全删除检查")
	}
	info, err := os.Lstat(cleaned)
	if err != nil {
		return "", fmt.Errorf("项目目录不可访问: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("彻底卸载不允许删除符号链接或非目录目标")
	}
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(cleaned) {
		return "", errors.New("项目登记路径与实际目录不完全一致，已拒绝删除")
	}
	volume := filepath.VolumeName(resolved)
	remainder := strings.TrimPrefix(resolved, volume)
	components := strings.FieldsFunc(remainder, func(r rune) bool { return r == filepath.Separator })
	if len(components) < 3 {
		return "", errors.New("项目路径层级过浅，已拒绝彻底卸载")
	}
	if home, homeErr := os.UserHomeDir(); homeErr == nil && filepath.Clean(home) == resolved {
		return "", errors.New("不能删除用户主目录")
	}
	dataDir, dataErr := filepath.Abs(s.store.Dir())
	if dataErr == nil && pathContains(resolved, dataDir) {
		return "", errors.New("项目目录包含 ProjectDock 数据目录，已拒绝删除")
	}
	return resolved, nil
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (s *Service) Start(ctx context.Context, id string) (model.RunStatus, error) {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	project, err := s.Get(ctx, id)
	if err != nil {
		return model.RunStatus{}, err
	}
	if err := project.Validate(true); err != nil {
		return model.RunStatus{}, err
	}
	if strings.TrimSpace(project.StartCommand) == "" {
		return model.RunStatus{}, errors.New("该项目已登记，但启动方式尚未接入；请先配置启动命令")
	}
	s.mu.Lock()
	if run, exists := s.runs[id]; exists && (run.status.State == "running" || run.status.State == "starting" || run.status.State == "stopping") {
		status := run.status
		s.mu.Unlock()
		return status, errors.New("项目已经由 ProjectDock 管理运行")
	}
	s.mu.Unlock()

	observed, observeErr := s.observeProject(ctx, project)
	if observeErr != nil {
		return model.RunStatus{}, observeErr
	}
	if observed.State == "external" {
		return observed, errors.New("项目已在 ProjectDock 外部运行；请先使用已登记的停止命令停止，或直接沿用当前服务")
	}
	if observed.State == "conflict" {
		return observed, errors.New("项目检测端口被其他已登记项目占用，请先处理端口冲突")
	}

	for _, portNumber := range project.Ports {
		availability, checkErr := s.ports.Check(ctx, portNumber, project.ID)
		if checkErr != nil {
			return model.RunStatus{}, checkErr
		}
		if !availability.Available {
			return model.RunStatus{}, fmt.Errorf("端口 %d 不可用: %s", portNumber, availability.Reason)
		}
		if availability.Allocation == nil {
			return model.RunStatus{}, fmt.Errorf("端口 %d 尚未持久分配给当前项目", portNumber)
		}
	}
	allocated := make(map[int]bool, len(project.Ports))
	for _, portNumber := range project.Ports {
		allocated[portNumber] = true
	}
	for _, portNumber := range project.LaunchPorts {
		if allocated[portNumber] {
			continue
		}
		availability, checkErr := s.ports.Check(ctx, portNumber, project.ID)
		if checkErr != nil {
			return model.RunStatus{}, checkErr
		}
		if !availability.Available {
			return model.RunStatus{}, fmt.Errorf("检测端口 %d 不可用: %s", portNumber, availability.Reason)
		}
	}

	logPath := filepath.Join(s.store.Dir(), "logs", fmt.Sprintf("%s-%s.log", project.ID, s.now().Format("20060102-150405")))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return model.RunStatus{}, fmt.Errorf("创建项目日志: %w", err)
	}

	command := exec.Command("/bin/zsh", "-lc", project.StartCommand)
	if _, statErr := os.Stat("/bin/zsh"); statErr != nil {
		command = exec.Command("/bin/sh", "-lc", project.StartCommand)
	}
	runtimeDirectory, err := project.RuntimeDirectory(true)
	if err != nil {
		logFile.Close()
		return model.RunStatus{}, err
	}
	command.Dir = runtimeDirectory
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Env = append(os.Environ(),
		"PROJECTDOCK_PROJECT_ID="+project.ID,
		"PROJECTDOCK_MANAGED=1",
	)
	token, err := randomToken()
	if err != nil {
		logFile.Close()
		return model.RunStatus{}, err
	}
	if err := command.Start(); err != nil {
		logFile.Close()
		return model.RunStatus{}, fmt.Errorf("启动项目: %w", err)
	}
	status := model.RunStatus{
		ProjectID: project.ID,
		State:     "running",
		PID:       command.Process.Pid,
		StartedAt: s.now(),
		LogPath:   logPath,
		Message:   "由 ProjectDock 管理",
	}
	run := &managedRun{command: command, token: token, status: status}
	s.mu.Lock()
	s.runs[project.ID] = run
	s.mu.Unlock()
	s.audit(ctx, "project.start", "success", project.ID, 0, map[string]any{"pid": status.PID})

	go s.wait(project, run, logFile)
	return status, nil
}

func (s *Service) wait(project model.Project, run *managedRun, logFile *os.File) {
	err := run.command.Wait()
	_ = logFile.Close()
	exitCode := 0
	message := "项目已退出"
	if err != nil {
		message = err.Error()
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}
	now := s.now()
	s.mu.Lock()
	current, exists := s.runs[project.ID]
	if exists && current.token == run.token {
		current.status.State = "stopped"
		current.status.StoppedAt = now
		current.status.ExitCode = &exitCode
		current.status.Message = message
	}
	s.mu.Unlock()
	s.audit(context.Background(), "project.exit", "success", project.ID, 0, map[string]any{"exitCode": exitCode})
}

func (s *Service) Stop(ctx context.Context, id string) (model.RunStatus, error) {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	run, exists := s.runs[id]
	if !exists || run.status.State != "running" {
		s.mu.Unlock()
		return s.stopExternal(ctx, id)
	}
	pid := run.status.PID
	run.status.State = "stopping"
	run.status.Message = "正在发送 SIGTERM"
	s.mu.Unlock()

	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		s.mu.Lock()
		run.status.State = "running"
		run.status.Message = "停止信号发送失败"
		s.mu.Unlock()
		return run.status, fmt.Errorf("停止项目: %w", err)
	}
	deadline := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return s.Status(id), ctx.Err()
		case <-ticker.C:
			status := s.Status(id)
			if status.State == "stopped" {
				s.audit(ctx, "project.stop", "success", id, 0, map[string]any{"pid": pid, "signal": "SIGTERM"})
				return status, nil
			}
		case <-deadline.C:
			s.mu.RLock()
			current, stillManaged := s.runs[id]
			tokenMatches := stillManaged && current.token == run.token && current.status.PID == pid
			s.mu.RUnlock()
			if !tokenMatches {
				return s.Status(id), errors.New("运行令牌已变化，拒绝强制停止")
			}
			if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				return s.Status(id), fmt.Errorf("强制停止项目: %w", err)
			}
			s.audit(ctx, "project.stop", "success", id, 0, map[string]any{"pid": pid, "signal": "SIGKILL"})
			return s.Status(id), nil
		}
	}
}

func (s *Service) stopExternal(ctx context.Context, id string) (model.RunStatus, error) {
	project, err := s.Get(ctx, id)
	if err != nil {
		return model.RunStatus{}, err
	}
	observed, err := s.observeProject(ctx, project)
	if err != nil {
		return model.RunStatus{}, err
	}
	if observed.State != "external" {
		return observed, errors.New("项目当前没有可由 ProjectDock 停止的运行服务")
	}
	if strings.TrimSpace(project.StopCommand) == "" {
		return observed, errors.New("项目正在外部运行，但没有登记安全停止命令；ProjectDock 不会终止未知 PID")
	}
	runtimeDirectory, err := project.RuntimeDirectory(true)
	if err != nil {
		return observed, err
	}
	command := exec.CommandContext(ctx, "/bin/zsh", "-lc", project.StopCommand)
	if _, statErr := os.Stat("/bin/zsh"); statErr != nil {
		command = exec.CommandContext(ctx, "/bin/sh", "-lc", project.StopCommand)
	}
	command.Dir = runtimeDirectory
	command.Env = append(os.Environ(),
		"PROJECTDOCK_PROJECT_ID="+project.ID,
		"PROJECTDOCK_EXTERNAL_STOP=1",
	)
	output, commandErr := command.CombinedOutput()
	if commandErr != nil {
		s.audit(ctx, "project.external-stop", "failed", project.ID, 0, map[string]any{
			"error": commandErr.Error(),
		})
		return observed, fmt.Errorf("执行已登记停止命令失败: %w: %s", commandErr, strings.TrimSpace(string(output)))
	}

	deadline := time.NewTimer(10 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		status, statusErr := s.observeProject(ctx, project)
		if statusErr != nil {
			return observed, statusErr
		}
		if status.State != "external" {
			status.State = "stopped"
			status.StoppedAt = s.now()
			status.Message = "已通过项目登记的停止命令停止"
			s.audit(ctx, "project.external-stop", "success", project.ID, 0, map[string]any{
				"commandRegistered": true,
			})
			return status, nil
		}
		select {
		case <-ctx.Done():
			return observed, ctx.Err()
		case <-ticker.C:
		case <-deadline.C:
			return observed, errors.New("停止命令已执行，但项目声明端口仍在监听；未对未知 PID 发送信号")
		}
	}
}

func (s *Service) Status(id string) model.RunStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if run, exists := s.runs[id]; exists {
		return run.status
	}
	return model.RunStatus{ProjectID: id, State: "stopped", Message: "当前实例未管理该项目进程"}
}

func (s *Service) observeProject(ctx context.Context, project model.Project) (model.RunStatus, error) {
	registry, err := s.store.Load(ctx)
	if err != nil {
		return model.RunStatus{}, err
	}
	listeners, err := s.ports.List(ctx)
	if err != nil {
		return model.RunStatus{}, err
	}
	return s.observedStatus(project, listeners, declaredPortClaims(registry.Projects)), nil
}

type portClaim struct {
	projectID string
	allocated bool
}

func (s *Service) observedStatus(project model.Project, listeners []model.PortListener, portClaims map[int][]portClaim) model.RunStatus {
	managed := s.Status(project.ID)
	if managed.State == "running" || managed.State == "starting" || managed.State == "stopping" {
		return managed
	}
	allocated := make(map[int]bool, len(project.Ports))
	for _, portNumber := range project.Ports {
		allocated[portNumber] = true
	}
	declared := make(map[int]bool, len(project.Ports)+len(project.LaunchPorts))
	for _, portNumber := range project.Ports {
		declared[portNumber] = true
	}
	for _, portNumber := range project.LaunchPorts {
		declared[portNumber] = true
	}
	for _, listener := range listeners {
		if !declared[listener.Port] {
			continue
		}
		claims := portClaims[listener.Port]
		allocatedOwner := ""
		for _, claim := range claims {
			if claim.allocated {
				allocatedOwner = claim.projectID
				break
			}
		}
		if allocatedOwner != "" && allocatedOwner != project.ID && !allocated[listener.Port] {
			return model.RunStatus{
				ProjectID: project.ID,
				State:     "conflict",
				PID:       listener.PID,
				Message:   fmt.Sprintf("检测端口 %d 已持久分配给项目 %s", listener.Port, allocatedOwner),
			}
		}
		if !allocated[listener.Port] && len(claims) > 1 {
			return model.RunStatus{
				ProjectID: project.ID,
				State:     "conflict",
				PID:       listener.PID,
				Message:   fmt.Sprintf("检测端口 %d 被 %d 个项目共同声明，需持久分配后确认归属", listener.Port, len(claims)),
			}
		}
		return model.RunStatus{
			ProjectID: project.ID,
			State:     "external",
			PID:       listener.PID,
			Message:   fmt.Sprintf("端口 %d 正由 ProjectDock 外部进程监听", listener.Port),
		}
	}
	return managed
}

func declaredPortClaims(projects []model.Project) map[int][]portClaim {
	claims := map[int][]portClaim{}
	for _, project := range projects {
		for _, portNumber := range project.Ports {
			claims[portNumber] = append(claims[portNumber], portClaim{projectID: project.ID, allocated: true})
		}
		allocated := map[int]bool{}
		for _, portNumber := range project.Ports {
			allocated[portNumber] = true
		}
		for _, portNumber := range project.LaunchPorts {
			if !allocated[portNumber] {
				claims[portNumber] = append(claims[portNumber], portClaim{projectID: project.ID})
			}
		}
	}
	return claims
}

func (s *Service) ReadLog(id string, maxLines int) ([]string, error) {
	if maxLines < 1 || maxLines > 1000 {
		maxLines = 200
	}
	status := s.Status(id)
	if status.LogPath == "" {
		return []string{}, nil
	}
	file, err := os.Open(status.LogPath)
	if err != nil {
		return nil, fmt.Errorf("打开日志: %w", err)
	}
	defer file.Close()
	return tailLines(file, maxLines)
}

func tailLines(reader io.Reader, limit int) ([]string, error) {
	lines := make([]string, 0, limit)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			lines = lines[len(lines)-limit:]
		}
	}
	return lines, scanner.Err()
}

func (s *Service) audit(ctx context.Context, action, status, projectID string, port int, detail map[string]any) {
	_ = s.store.AppendAudit(ctx, model.AuditEvent{
		ID:        eventID(),
		Timestamp: s.now(),
		Action:    action,
		Status:    status,
		ProjectID: projectID,
		Port:      port,
		Detail:    detail,
	})
}

func eventID() string {
	token, err := randomToken()
	if err != nil {
		return fmt.Sprintf("event-%d", time.Now().UnixNano())
	}
	return token[:16]
}

func randomToken() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("生成运行令牌: %w", err)
	}
	return hex.EncodeToString(value), nil
}
